package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func listen(t *testing.T) *Server {
	t.Helper()
	s, err := Listen(Options{Name: "canvas", Instructions: "Draw.",
		Tools: []Tool{{Name: "echo", InputSchema: map[string]any{"type": "object"}}},
		Call: func(_ context.Context, name string, arguments json.RawMessage) (string, bool) {
			var a struct{ Text string }
			json.Unmarshal(arguments, &a)
			return name + ": " + a.Text, a.Text == "fail"
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// raw posts body to the server, with its token when auth says.
func raw(t *testing.T, s *Server, body string, auth bool) (int, string) {
	t.Helper()
	m := s.Agent()
	req, _ := http.NewRequest(http.MethodPost, m.URL, bytes.NewReader([]byte(body)))
	if auth {
		req.Header.Set("Authorization", m.Headers["Authorization"])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(data)
}

func TestServesTools(t *testing.T) {
	s := listen(t)
	if err := s.Agent().Check(); err != nil {
		t.Fatal(err)
	}
	code, body := raw(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`, true)
	if code != 200 || !strings.Contains(body, `"protocolVersion":"2025-03-26"`) || !strings.Contains(body, `"instructions":"Draw."`) {
		t.Fatalf("initialize: %d %s", code, body)
	}
	if code, _ := raw(t, s, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, true); code != http.StatusAccepted {
		t.Fatalf("notification: %d", code)
	}
	if _, body := raw(t, s, `[{"jsonrpc":"2.0","id":2,"method":"tools/list"},{"jsonrpc":"2.0","id":3,"method":"nope"}]`, true); !strings.Contains(body, `"name":"echo"`) || !strings.Contains(body, `-32601`) {
		t.Fatalf("batch: %s", body)
	}
	text, failed, err := CallTool(context.Background(), s.Agent(), "echo", map[string]any{"text": "hi"})
	if err != nil || failed || text != "echo: hi" {
		t.Fatalf("call = %q %v %v", text, failed, err)
	}
	if _, failed, _ := CallTool(context.Background(), s.Agent(), "echo", map[string]any{"text": "fail"}); !failed {
		t.Fatal("a failed call did not say so")
	}
	if _, _, err := CallTool(context.Background(), s.Agent(), "other", nil); err == nil {
		t.Fatal("called an unknown tool")
	}
}

func TestRefusesRequestsWithoutTheToken(t *testing.T) {
	s := listen(t)
	if code, _ := raw(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, false); code != http.StatusUnauthorized {
		t.Fatalf("code = %d", code)
	}
	m := s.Agent()
	req, _ := http.NewRequest(http.MethodPost, m.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Authorization", m.Headers["Authorization"])
	req.Header.Set("Origin", "https://example.com")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("a page's request got %d", res.StatusCode)
	}
}
