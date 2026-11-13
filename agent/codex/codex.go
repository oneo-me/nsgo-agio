// Package codex runs OpenAI's Codex CLI through its app-server JSON-RPC
// protocol over stdio. Every thread runs with full access, as Agio's other
// agents do; Codex's own sandbox is not used.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

// RequestTimeout bounds a request's response. Turn starts respond on
// acceptance, not completion.
const RequestTimeout = 60 * time.Second

// ErrNotFound reports that codex is not on the login PATH.
var ErrNotFound = errors.New("codex was not found on PATH")

// Locate finds codex on env's PATH.
func Locate(env []string) (*Driver, error) {
	path := agent.LookPath("codex", env)
	if path == "" {
		return nil, ErrNotFound
	}
	return &Driver{Path: path, Env: env}, nil
}

// Driver runs Codex's app server as an agent.Driver.
type Driver struct {
	Path string
	Env  []string
}

func (d *Driver) Tool() agent.Tool { return agent.Codex }

func (d *Driver) command(dir string) *exec.Cmd {
	cmd := exec.Command(d.Path, "app-server", "--stdio")
	cmd.Dir, cmd.Env = dir, d.Env
	return cmd
}

// Version runs `codex --version`.
func (d *Driver) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.Path, "--version")
	childproc.HideConsole(cmd)
	cmd.Env = d.Env
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", fmt.Errorf("unrecognized codex version %q", out)
	}
	return fields[len(fields)-1], nil
}

// client is one app-server process with JSON-RPC correlation. Messages
// carrying a method are notifications, or requests when they also carry an
// ID; the rest answer Agio's own requests.
type client struct {
	proc    *agent.Process
	mu      sync.Mutex
	next    int64
	pending map[int64]chan rpcResult
	closed  bool
}

type rpcResult struct {
	data json.RawMessage
	err  error
}

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// start launches the app server and completes the initialize handshake.
// onMessage receives notifications and server requests on the reader.
func start(cmd *exec.Cmd, onMessage func(message), onExit func(error)) (*client, error) {
	c := &client{pending: map[int64]chan rpcResult{}}
	proc, err := agent.StartProcess(cmd, func(line []byte) {
		var m message
		if json.Unmarshal(line, &m) != nil {
			return
		}
		if m.Method == "" {
			c.answer(m)
		} else if onMessage != nil {
			onMessage(m)
		}
	}, func(err error) {
		c.fail()
		if onExit != nil {
			onExit(err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}
	c.proc = proc
	ctx := context.Background()
	if _, err := c.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": agent.Client.Name, "title": agent.Client.Title, "version": agent.Client.Version},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		go proc.Close(3 * time.Second)
		return nil, err
	}
	if err := proc.Send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		go proc.Close(3 * time.Second)
		return nil, err
	}
	return c, nil
}

func (c *client) answer(m message) {
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
		ch <- rpcResult{err: errors.New(firstNonEmpty(m.Error.Message, "codex rejected the request"))}
		return
	}
	ch <- rpcResult{data: m.Result}
}

func (c *client) fail() {
	c.mu.Lock()
	c.closed = true
	pending := c.pending
	c.pending = map[int64]chan rpcResult{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- rpcResult{err: agent.ErrClosed}
	}
}

func (c *client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, agent.ErrClosed
	}
	c.next++
	id := c.next
	ch := make(chan rpcResult, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.proc.Send(map[string]any{"method": method, "id": id, "params": params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

func (c *client) callInto(ctx context.Context, method string, params, out any) error {
	data, err := c.call(ctx, method, params)
	if err != nil || out == nil || len(data) == 0 {
		return err
	}
	return json.Unmarshal(data, out)
}

// reply answers a server request. Approvals are accepted, matching full
// access; anything needing a person is declined because Agio has no UI
// for it.
func (c *client) reply(m message) {
	decision := ""
	switch m.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		decision = "accept"
	case "applyPatchApproval", "execCommandApproval":
		decision = "approved"
	}
	if decision != "" {
		c.proc.Send(map[string]any{"id": m.ID, "result": map[string]any{"decision": decision}})
		return
	}
	c.proc.Send(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": agent.Client.Title + " cannot answer " + m.Method}})
}

// modelInfo is one model/list entry.
type modelInfo struct {
	Model                     string `json:"model"`
	DisplayName               string `json:"displayName"`
	Description               string `json:"description"`
	Hidden                    bool   `json:"hidden"`
	IsDefault                 bool   `json:"isDefault"`
	DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
	SupportedReasoningEfforts []struct {
		ReasoningEffort string `json:"reasoningEffort"`
	} `json:"supportedReasoningEfforts"`
	InputModalities []string `json:"inputModalities"`
	ServiceTiers    []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"serviceTiers"`
	AdditionalSpeedTiers []string `json:"additionalSpeedTiers"`
}

func (m modelInfo) agent() agent.Model {
	name := strings.TrimSpace(m.DisplayName)
	if name == "" {
		name = m.Model
	}
	out := agent.Model{Tool: agent.Codex, ID: m.Model, Name: displayName(name), Provider: "OpenAI",
		Detail: m.Model, DefaultEffort: m.DefaultReasoningEffort, Default: m.IsDefault}
	for _, e := range m.SupportedReasoningEfforts {
		if !agent.ThinkingOff(e.ReasoningEffort) {
			out.Efforts = append(out.Efforts, e.ReasoningEffort)
		}
	}
	for _, input := range m.InputModalities {
		out.Images = out.Images || input == "image"
	}
	for _, t := range m.ServiceTiers {
		if t.ID != "" && t.ID != "default" {
			out.Tiers = append(out.Tiers, agent.Tier{ID: t.ID, Name: firstNonEmpty(t.Name, t.ID), Description: t.Description})
		}
	}
	if len(out.Tiers) == 0 {
		for _, id := range m.AdditionalSpeedTiers {
			out.Tiers = append(out.Tiers, agent.Tier{ID: id, Name: strings.ToUpper(id[:1]) + id[1:]})
		}
	}
	return out
}

// displayName spells GPT in capitals and capitalizes hyphenated words, as in
// GPT-6.1-Sol.
func displayName(name string) string {
	parts := strings.Split(name, "-")
	for i, p := range parts {
		switch {
		case strings.EqualFold(p, "gpt"):
			parts[i] = "GPT"
		case p != "":
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "-")
}

// Catalog lists models through a short-lived app server.
func (d *Driver) Catalog(ctx context.Context, dir string) (agent.Catalog, error) {
	c, err := start(d.command(dir), nil, nil)
	if err != nil {
		return agent.Catalog{}, err
	}
	defer c.proc.Close(3 * time.Second)
	var catalog agent.Catalog
	cursor := ""
	for page := 0; page < 32; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var list struct {
			Data       []modelInfo `json:"data"`
			NextCursor *string     `json:"nextCursor"`
		}
		if err := c.callInto(ctx, "model/list", params, &list); err != nil {
			return agent.Catalog{}, err
		}
		for _, m := range list.Data {
			if m.Model == "" || m.Hidden {
				continue
			}
			model := m.agent()
			catalog.Models = append(catalog.Models, model)
			if model.Default {
				catalog.Model, catalog.Effort = model.ID, model.DefaultEffort
			}
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			break
		}
		cursor = *list.NextCursor
	}
	if catalog.Model == "" && len(catalog.Models) > 0 {
		catalog.Model, catalog.Effort = catalog.Models[0].ID, catalog.Models[0].DefaultEffort
	}
	return catalog, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
