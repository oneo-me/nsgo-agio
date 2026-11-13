package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// requestTimeout bounds requests other than prompts, which last a run.
const requestTimeout = 60 * time.Second

// protocolVersion is the ACP major version Agio speaks.
const protocolVersion = 1

// conn is a JSON-RPC 2.0 connection to an agent over its stdio.
type conn struct {
	proc    *agent.Process
	mu      sync.Mutex
	next    int64
	pending map[int64]chan result
	closed  bool
	caps    capabilities
	// onUpdate receives session/update notifications and onRequest the
	// agent's requests, both on the reader goroutine.
	onUpdate  func(json.RawMessage)
	onRequest func(method string, params json.RawMessage) (any, *rpcError)
}

type result struct {
	data json.RawMessage
	err  error
}

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC error; Data often explains Message.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	var data struct {
		Details string `json:"details"`
	}
	if json.Unmarshal(e.Data, &data) == nil && data.Details != "" {
		return e.Message + ": " + data.Details
	}
	return e.Message
}

var errMethodNotFound = &rpcError{Code: -32601, Message: "Method not found"}

// codeAuthRequired is ACP's error for an agent that needs a sign-in.
const codeAuthRequired = -32000

// authRequired reports whether err asks for a sign-in.
func authRequired(err error) bool {
	var e *rpcError
	return errors.As(err, &e) && e.Code == codeAuthRequired
}

// capabilities are the parts of the agent's initialize response Agio uses.
type capabilities struct {
	LoadSession        bool `json:"loadSession"`
	PromptCapabilities struct {
		Image bool `json:"image"`
	} `json:"promptCapabilities"`
	SessionCapabilities struct {
		Resume json.RawMessage `json:"resume"`
	} `json:"sessionCapabilities"`
}

// canResume reports whether the agent reopens sessions without replaying them.
func (c capabilities) canResume() bool {
	return len(c.SessionCapabilities.Resume) > 0 && string(c.SessionCapabilities.Resume) != "null"
}

// dial starts cmd and completes the initialize handshake. The client offers
// neither file system nor terminal methods, so the agent works on its own
// files and processes, as other agents do.
func dial(cmd *exec.Cmd, name string, setup func(*conn), onExit func(error)) (*conn, error) {
	c := &conn{pending: map[int64]chan result{}}
	if setup != nil {
		setup(c)
	}
	ready := make(chan struct{})
	proc, err := agent.StartProcess(cmd, func(line []byte) {
		<-ready
		c.receive(line)
	}, func(err error) {
		<-ready
		c.fail()
		if onExit != nil {
			onExit(err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	c.proc = proc
	close(ready)
	var init struct {
		ProtocolVersion   int          `json:"protocolVersion"`
		AgentCapabilities capabilities `json:"agentCapabilities"`
	}
	err = c.callInto(context.Background(), "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
		"clientInfo": map[string]any{"name": agent.Client.Name, "title": agent.Client.Title, "version": agent.Client.Version},
	}, &init)
	if err == nil && init.ProtocolVersion != protocolVersion {
		err = fmt.Errorf("%s speaks ACP version %d, %s %d", name, init.ProtocolVersion, agent.Client.Title, protocolVersion)
	}
	if err != nil {
		go proc.Close(3 * time.Second)
		return nil, err
	}
	c.caps = init.AgentCapabilities
	return c, nil
}

func (c *conn) receive(line []byte) {
	var m message
	if json.Unmarshal(line, &m) != nil {
		return
	}
	switch {
	case m.Method == "":
		c.answer(m)
	case len(m.ID) == 0:
		if m.Method == "session/update" && c.onUpdate != nil {
			c.onUpdate(m.Params)
		}
	default:
		var res any
		rpcErr := errMethodNotFound
		if c.onRequest != nil {
			res, rpcErr = c.onRequest(m.Method, m.Params)
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": m.ID}
		if rpcErr != nil {
			reply["error"] = rpcErr
		} else {
			reply["result"] = res
		}
		// The reader never blocks on a full stdin pipe.
		go c.proc.Send(reply)
	}
}

func (c *conn) answer(m message) {
	var id int64
	if json.Unmarshal(m.ID, &id) != nil {
		return
	}
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch == nil {
		return
	}
	if m.Error != nil {
		ch <- result{err: m.Error}
		return
	}
	ch <- result{data: m.Result}
}

func (c *conn) fail() {
	c.mu.Lock()
	c.closed = true
	pending := c.pending
	c.pending = map[int64]chan result{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- result{err: agent.ErrClosed}
	}
}

// request sends a request and returns the channel its result arrives on.
func (c *conn) request(method string, params any) (<-chan result, func(), error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, nil, agent.ErrClosed
	}
	c.next++
	id := c.next
	ch := make(chan result, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}
	if err := c.proc.Send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		forget()
		return nil, nil, err
	}
	return ch, forget, nil
}

// call sends a request and waits for its result for at most requestTimeout.
func (c *conn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ch, forget, err := c.request(method, params)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("%s: %w", method, r.err)
		}
		return r.data, nil
	case <-ctx.Done():
		forget()
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

func (c *conn) callInto(ctx context.Context, method string, params, out any) error {
	data, err := c.call(ctx, method, params)
	if err != nil || out == nil || len(data) == 0 || string(data) == "null" {
		return err
	}
	return json.Unmarshal(data, out)
}

func (c *conn) notify(method string, params any) error {
	return c.proc.Send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}
