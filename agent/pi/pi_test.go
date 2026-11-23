package pi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

func TestReadSessionFollowsActiveBranch(t *testing.T) {
	lines := []string{
		`{"type":"session","version":3,"id":"s","cwd":"/tmp"}`,
		`{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":"one","timestamp":1}}`,
		`{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"answer"}],"stopReason":"stop","usage":{"input":10,"output":2,"cacheRead":0,"cacheWrite":0,"totalTokens":0}}}`,
		`{"type":"message","id":"old","parentId":"u1","message":{"role":"user","content":"abandoned"}}`,
		`not json`,
		`{"type":"session_info","id":"n1","parentId":"a1","name":"Named"}`,
		`{"type":"message","id":"u2","parentId":"n1","message":{"role":"user","content":[{"type":"text","text":"two"}]}}`,
	}
	h, err := parseSession(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "Named" || len(h.Messages) != 3 {
		t.Fatalf("history: %+v", h)
	}
	if h.Messages[0].Content.Text() != "one" || h.Messages[1].Content.Text() != "answer" || h.Messages[2].Content.Text() != "two" {
		t.Fatalf("wrong branch: %+v", h.Messages)
	}
	if h.Messages[1].Usage.ContextTokens() != 12 {
		t.Fatal("component token fallback")
	}
}

func TestModelLevels(t *testing.T) {
	var m Model
	if err := json.Unmarshal([]byte(`{"id":"x","provider":"p","reasoning":true,"thinkingLevelMap":{"off":null,"minimal":null,"low":"low","high":"high","max":"max"}}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Levels(), ","); got != "low,medium,high,max" {
		t.Fatal(got)
	}
	xhigh := "xhigh"
	m.ThinkingLevelMap = map[string]*string{"xhigh": &xhigh}
	if got := strings.Join(m.Levels(), ","); got != "minimal,low,medium,high,xhigh" {
		t.Fatal(got)
	}
	m.ThinkingLevelMap = nil
	if got := strings.Join(m.Levels(), ","); got != "minimal,low,medium,high" {
		t.Fatal(got)
	}
	m.Reasoning = false
	if got := strings.Join(m.Levels(), ","); got != "" {
		t.Fatal(got)
	}
	if !Supported("1.1.0") || !Supported("2.0") || Supported("0.73.1") || Supported("") {
		t.Fatal("version gate")
	}
}

func TestDriverAppendsTheSystemPromptAndSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix shell")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "pi")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$(dirname "$0")/args.txt"
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  printf '{"id":"%s","type":"response","success":true,"data":{}}\n' "$id"
done
`
	os.WriteFile(fake, []byte(script), 0755)
	d := &Driver{Command: Command{Path: fake, Env: os.Environ()}}
	s, _, err := d.Start(agent.StartOptions{Dir: dir, SystemPrompt: "Be brief", Skills: []string{"/skills/a", "/skills/b"}}, func(agent.Event) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(time.Second)
	if args, _ := os.ReadFile(filepath.Join(dir, "args.txt")); !strings.Contains(string(args), "--append-system-prompt\nBe brief\n--skill\n/skills/a\n--skill\n/skills/b\n") {
		t.Fatalf("args: %q", args)
	}
}

func TestEnvReachesPiAndItsCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix shell")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "pi")
	script := `#!/bin/sh
sh -c 'echo "$AGIO_TEST_TASK $PATH"' > "$(dirname "$0")/env.txt"
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  printf '{"id":"%s","type":"response","success":true,"data":{}}\n' "$id"
done
`
	os.WriteFile(fake, []byte(script), 0755)
	d := &Driver{Command: Command{Path: fake, Env: os.Environ()}}
	env := map[string]string{"AGIO_TEST_TASK": "t1", "PATH": "$PATH:/opt/test/bin"}
	s, _, err := d.Start(agent.StartOptions{Dir: dir, Env: env}, func(agent.Event) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(time.Second)
	if data, _ := os.ReadFile(filepath.Join(dir, "env.txt")); string(data) != "t1 "+os.Getenv("PATH")+":/opt/test/bin\n" {
		t.Fatalf("env: %q", data)
	}
}

func TestMCPServersGoInATemporaryExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix shell")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "pi")
	// The fake keeps a copy, as the extension is removed with the session.
	script := `#!/bin/sh
printf '%s\n' "$@" > "$(dirname "$0")/args.txt"
while [ $# -gt 0 ]; do [ "$1" = -e ] && cp "$2" "$(dirname "$0")/extension.js"; shift; done
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  printf '{"id":"%s","type":"response","success":true,"data":{}}\n' "$id"
done
`
	os.WriteFile(fake, []byte(script), 0755)
	d := &Driver{Command: Command{Path: fake, Env: os.Environ()}}
	servers := []agent.MCPServer{{Name: "docs", Command: "docs-mcp", Args: []string{"--stdio"}}, {Name: "web", URL: "https://example.com/mcp"}}
	s, _, err := d.Start(agent.StartOptions{Dir: dir, MCPServers: servers}, func(agent.Event) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	_, file, ok := strings.Cut(string(args), "-e\n")
	file, _, _ = strings.Cut(file, "\n")
	source, _ := os.ReadFile(filepath.Join(dir, "extension.js"))
	if !ok || !strings.Contains(string(source), `{"docs":{"args":["--stdio"],"command":"docs-mcp"},"web":{"url":"https://example.com/mcp"}}`) ||
		!strings.Contains(string(source), "pi.registerMcpServer(name, config)") {
		t.Fatalf("args %q, extension %s", args, source)
	}
	s.Close(time.Second)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Dir(file)); os.IsNotExist(err) {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("extension left behind: %v", err)
		}
	}
}

// writeBunShim writes Bun's Windows shim for a node script: name.exe and the
// name.bunx metadata its target path is read from, encoded as Bun 1.4 does.
// A fake RPC peer exercises framing, correlation, events and shutdown.
func TestClientAgainstFakeProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix shell")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "pi")
	script := `#!/bin/sh
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  case "$line" in
    *get_state*) printf '{"type":"agent_start"}\n{"id":"%s","type":"response","command":"get_state","success":true,"data":{"sessionId":"abc","sessionFile":"/s.jsonl","thinkingLevel":"high"}}\n' "$id" ;;
    *bad*) printf '{"id":"%s","type":"response","command":"bad","success":false,"error":"nope"}\n' "$id" ;;
  esac
done
`
	os.WriteFile(fake, []byte(script), 0755)
	events := make(chan Event, 4)
	exited := make(chan error, 1)
	c, err := Start(Options{Command: Command{Path: fake, Env: os.Environ()}, Dir: dir}, func(e Event) { events <- e }, func(err error) { exited <- err })
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err := c.CallInto(context.Background(), map[string]any{"type": "get_state"}, &state); err != nil || state.SessionID != "abc" || state.ThinkingLevel != "high" {
		t.Fatalf("state %+v %v", state, err)
	}
	if e := <-events; e.Type != "agent_start" {
		t.Fatal("event order")
	}
	if _, err := c.Call(context.Background(), map[string]any{"type": "bad"}); err == nil || err.Error() != "nope" {
		t.Fatalf("error response: %v", err)
	}
	c.Close(5 * time.Second)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("no exit")
	}
	if _, err := c.Call(context.Background(), map[string]any{"type": "get_state"}); err != ErrClosed {
		t.Fatalf("closed call: %v", err)
	}
}
