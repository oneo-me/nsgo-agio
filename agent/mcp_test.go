package agent

import "testing"

func TestMCPServerCheck(t *testing.T) {
	for _, m := range []MCPServer{
		{Name: "", Command: "x"},
		{Name: "a.b", Command: "x"},
		{Name: "a"},
		{Name: "a", Command: "x", URL: "https://example.com"},
		{Name: "a", URL: "https://example.com", Args: []string{"x"}},
		{Name: "a", Command: "x", Headers: map[string]string{"A": "1"}},
	} {
		if m.Check() == nil {
			t.Errorf("%+v passed", m)
		}
	}
	for _, m := range []MCPServer{{Name: "docs_2", Command: "x", Env: map[string]string{"A": "1"}}, {Name: "web-a", URL: "https://example.com"}} {
		if err := m.Check(); err != nil {
			t.Errorf("%+v: %v", m, err)
		}
	}
}
