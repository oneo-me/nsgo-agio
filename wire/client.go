package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// Client mirrors a served backend. It is an agio.Backend: its requests go
// to the server, and its state follows the server's publications.
type Client struct {
	peer *Peer

	mu        sync.Mutex
	state     agio.State
	turns     map[string][]*transcript.Turn
	watchers  map[int]func()
	nextWatch int
}

// Dial connects to a server listening on addr over TCP.
func Dial(addr, token string) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	c, err := Connect(conn, token)
	if err != nil {
		conn.Close()
	}
	return c, err
}

// Connect opens a session with the server on conn.
func Connect(conn io.ReadWriteCloser, token string) (*Client, error) {
	c := &Client{turns: map[string][]*transcript.Turn{}, watchers: map[int]func(){}, state: agio.State{Discovering: true}}
	c.peer = NewPeer(conn, c)
	var w Welcome
	if err := c.call(MethodHello, Hello{Version: Version, Token: token}, &w); err != nil {
		c.peer.Close()
		return nil, err
	}
	if w.Version != Version {
		c.peer.Close()
		return nil, fmt.Errorf("the server speaks protocol %d, this client %d", w.Version, Version)
	}
	go func() {
		<-c.peer.Done()
		c.mu.Lock()
		c.state.Err = "disconnected"
		if err := c.peer.Err(); err != nil && !errors.Is(err, ErrClosed) {
			c.state.Err += ": " + err.Error()
		}
		c.mu.Unlock()
		c.notify()
	}()
	return c, nil
}

// Done closes once the connection has ended.
func (c *Client) Done() <-chan struct{} { return c.peer.Done() }

func (c *Client) call(method string, params, result any) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return c.peer.Call(ctx, method, params, result)
}

// State implements agio.Backend.
func (c *Client) State() agio.State {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state
	s.Conversations = make([]*agio.Conversation, len(c.state.Conversations))
	for i, conv := range c.state.Conversations {
		copy := *conv
		copy.Turns = transcript.Clone(c.turns[conv.ID])
		s.Conversations[i] = &copy
	}
	return s
}

// Watch implements agio.Backend.
func (c *Client) Watch(fn func()) (stop func()) {
	c.mu.Lock()
	id := c.nextWatch
	c.nextWatch++
	c.watchers[id] = fn
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.watchers, id)
		c.mu.Unlock()
	}
}

func (c *Client) notify() {
	c.mu.Lock()
	fns := make([]func(), 0, len(c.watchers))
	for _, fn := range c.watchers {
		fns = append(fns, fn)
	}
	c.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// Create implements agio.Backend. The answer comes before the
// publication showing the conversation, which it waits for.
func (c *Client) Create(o agio.Create) (string, error) {
	shown := make(chan struct{}, 1)
	stop := c.Watch(func() {
		select {
		case shown <- struct{}{}:
		default:
		}
	})
	defer stop()
	var ref Ref
	if err := c.call(MethodCreate, o, &ref); err != nil {
		return "", err
	}
	timeout := time.After(callTimeout)
	for {
		c.mu.Lock()
		ok := c.state.Conversation(ref.ID) != nil
		c.mu.Unlock()
		if ok {
			return ref.ID, nil
		}
		select {
		case <-shown:
		case <-c.peer.Done():
			return "", ErrClosed
		case <-timeout:
			// Removed before it was published; there is nothing to wait for.
			return ref.ID, nil
		}
	}
}

// Send implements agio.Backend.
func (c *Client) Send(id string, m agio.Message) error {
	return c.call(MethodSend, Send{ID: id, Message: m}, nil)
}

// Stop implements agio.Backend.
func (c *Client) Stop(id string) error { return c.call(MethodStop, Ref{ID: id}, nil) }

// Choose implements agio.Backend.
func (c *Client) Choose(id string, ch agio.Choice) error {
	return c.call(MethodChoose, Choose{ID: id, Choice: ch}, nil)
}

// Remove implements agio.Backend.
func (c *Client) Remove(id string) error { return c.call(MethodRemove, Ref{ID: id}, nil) }

// Discover implements agio.Backend.
func (c *Client) Discover() { go c.call(MethodDiscover, nil, nil) }

// Close implements agio.Backend: it disconnects, leaving the server's
// agents running.
func (c *Client) Close() error { return c.peer.Close() }

// Request implements Handler; servers send none.
func (c *Client) Request(method string, _ json.RawMessage, reply func(any, error)) {
	reply(nil, errors.New("unknown request "+method))
}

// Event implements Handler: it applies a state publication.
func (c *Client) Event(method string, params json.RawMessage) {
	if method != EventState {
		return
	}
	var st State
	if json.Unmarshal(params, &st) != nil {
		return
	}
	c.mu.Lock()
	if st.Reset {
		c.turns = map[string][]*transcript.Turn{}
	}
	if st.Head != nil {
		c.state.Discovering, c.state.Tools = st.Head.Discovering, st.Head.Tools
	}
	if st.Conversations != nil {
		c.state.Conversations = *st.Conversations
		live := map[string]bool{}
		for _, conv := range c.state.Conversations {
			live[conv.ID] = true
		}
		for id := range c.turns {
			if !live[id] {
				delete(c.turns, id)
			}
		}
	}
	for id, p := range st.Turns {
		c.turns[id] = transcript.Apply(c.turns[id], p)
	}
	c.mu.Unlock()
	c.notify()
}

// Stream implements Handler; servers send none.
func (c *Client) Stream(uint64, []byte) {}
