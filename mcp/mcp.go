// Package mcp serves an application's own tools to the agents it runs, as
// a Model Context Protocol server over streamable HTTP on the loopback
// interface. Requests need a bearer token the server makes, which only the
// agent.MCPServer it returns carries, so other local programs and web
// pages cannot call the tools. It answers each request with one JSON
// message and opens no event streams, as tools that answer once need none.
package mcp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/oneo-me/nsgo-agio/agent"
)

// Tool is a tool the server offers; InputSchema is the JSON schema of its
// arguments, an object.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Options configure a Server.
type Options struct {
	// Name names the server to agents, in agent.MCPServer's form, and
	// Version is its version.
	Name, Version string
	// Instructions tell agents how to use the tools.
	Instructions string
	Tools        []Tool
	// Call runs a tool with its arguments, a JSON object, on the request's
	// goroutine, and returns the text the agent reads back and whether the
	// call failed. ctx ends when the agent gives up on the request.
	Call func(ctx context.Context, name string, arguments json.RawMessage) (text string, failed bool)
}

// Server is a running MCP server.
type Server struct {
	opts  Options
	token string
	ln    net.Listener
	srv   *http.Server
}

// Listen starts a server on a free loopback port.
func Listen(o Options) (*Server, error) {
	if err := (agent.MCPServer{Name: o.Name, URL: "http://127.0.0.1"}).Check(); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	rand.Read(key)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{opts: o, token: hex.EncodeToString(key), ln: ln}
	s.srv = &http.Server{Handler: s}
	go s.srv.Serve(ln)
	return s, nil
}

// Agent is how an agent reaches the server, for host.Options or
// agio.Create.
func (s *Server) Agent() agent.MCPServer {
	return agent.MCPServer{Name: s.opts.Name, URL: "http://" + s.ln.Addr().String() + "/mcp",
		Headers: map[string]string{"Authorization": "Bearer " + s.token}}
}

// Close stops the server.
func (s *Server) Close() error { return s.srv.Close() }

// versions are the protocol versions the server speaks, latest first.
var versions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := []byte(r.Header.Get("Authorization"))
	if subtle.ConstantTimeCompare(auth, []byte("Bearer "+s.token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// A page in a browser could reach the port; agents send no Origin, or
	// a local one.
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || !loopback(u.Hostname()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPost:
	case http.MethodDelete:
		// Sessions are not kept, so ending one is done already.
		w.WriteHeader(http.StatusOK)
		return
	default:
		// No stream of messages from the server.
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var replies []*response
	batch := strings.HasPrefix(strings.TrimSpace(string(body)), "[")
	var messages []request
	if batch {
		err = json.Unmarshal(body, &messages)
	} else {
		var m request
		err = json.Unmarshal(body, &m)
		messages = []request{m}
	}
	if err != nil {
		writeJSON(w, &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
		return
	}
	for _, m := range messages {
		if reply := s.handle(r.Context(), m); reply != nil {
			replies = append(replies, reply)
		}
	}
	switch {
	case len(replies) == 0:
		// Notifications and responses only.
		w.WriteHeader(http.StatusAccepted)
	case batch:
		writeJSON(w, replies)
	default:
		writeJSON(w, replies[0])
	}
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handle answers a request, or nothing for a notification or a response.
func (s *Server) handle(ctx context.Context, m request) *response {
	if len(m.ID) == 0 || m.Method == "" {
		return nil
	}
	result, err := s.result(ctx, m)
	reply := &response{JSONRPC: "2.0", ID: m.ID, Result: result}
	if err != nil {
		var e *rpcError
		if !errors.As(err, &e) {
			e = &rpcError{Code: -32603, Message: err.Error()}
		}
		reply.Result, reply.Error = nil, e
	}
	return reply
}

func (e *rpcError) Error() string { return e.Message }

func (s *Server) result(ctx context.Context, m request) (any, error) {
	switch m.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(m.Params, &p)
		version := versions[0]
		if slices.Contains(versions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		result := map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{"name": s.opts.Name, "version": cmp(s.opts.Version, "1")}}
		if s.opts.Instructions != "" {
			result["instructions"] = s.opts.Instructions
		}
		return result, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := s.opts.Tools
		if tools == nil {
			tools = []Tool{}
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		if !slices.ContainsFunc(s.opts.Tools, func(t Tool) bool { return t.Name == p.Name }) {
			return nil, &rpcError{Code: -32602, Message: "unknown tool " + p.Name}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		text, failed := s.opts.Call(ctx, p.Name, p.Arguments)
		return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": failed}, nil
	}
	return nil, &rpcError{Code: -32601, Message: "method not found: " + m.Method}
}

func cmp(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
