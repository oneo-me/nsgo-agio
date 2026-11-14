package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// TestMain runs the test binary as a fake ACP agent when asked to.
func TestMain(m *testing.M) {
	if os.Getenv("AGIO_FAKE_ACP") != "" {
		fakeAgent()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type fakeMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// fakeAgent speaks enough ACP for the tests, logging the methods it
// receives to AGIO_FAKE_ACP. Model "a" offers reasoning levels, "b" one.
func fakeAgent() {
	logFile, _ := os.OpenFile(os.Getenv("AGIO_FAKE_ACP"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if len(os.Args) > 1 {
		fmt.Fprintln(logFile, "args", strings.Join(os.Args[1:], " "))
	}
	out := json.NewEncoder(os.Stdout)
	send := func(v any) { out.Encode(v) }
	reply := func(id json.RawMessage, result any) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	notify := func(u map[string]any) {
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": u}})
	}
	values := map[string]string{"mode": "default", "model": "a", "thought": "enabled"}
	options := func() []map[string]any {
		levels := []map[string]any{{"value": "disabled"}, {"value": "low"}, {"value": "high"}, {"value": "enabled"}}
		if values["model"] == "b" {
			levels = []map[string]any{{"value": "medium"}}
		}
		return []map[string]any{
			{"id": "mode", "category": "mode", "type": "select", "currentValue": values["mode"],
				"options": []map[string]any{{"value": "default"}, {"value": "bypassPermissions"}}},
			{"id": "model", "category": "model", "type": "select", "currentValue": values["model"],
				"options": []map[string]any{{"group": "g", "name": "Group", "options": []map[string]any{{"value": "a", "name": "Model A"}, {"value": "b", "name": "Model B", "description": "Fast"}}}}},
			{"id": "thought", "category": "thought_level", "type": "select", "currentValue": values["thought"], "options": levels},
		}
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	var slow json.RawMessage
	for in.Scan() {
		var m fakeMessage
		if json.Unmarshal(in.Bytes(), &m) != nil {
			continue
		}
		fmt.Fprintln(logFile, m.Method, string(m.Params))
		var p struct {
			ConfigID string `json:"configId"`
			Value    string `json:"value"`
			Prompt   []struct {
				Text string `json:"text"`
			} `json:"prompt"`
		}
		json.Unmarshal(m.Params, &p)
		switch m.Method {
		case "initialize":
			reply(m.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": true, "promptCapabilities": map[string]any{"image": true}}})
		case "session/new":
			reply(m.ID, map[string]any{"sessionId": "s1", "configOptions": options()})
		case "session/set_config_option":
			key := p.ConfigID
			if key == "thought" || values["model"] != p.Value && key == "model" {
				values["thought"] = "enabled"
			}
			values[key] = p.Value
			reply(m.ID, map[string]any{"configOptions": options()})
		case "session/load":
			notify(map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]any{"type": "text", "text": "hi"}})
			notify(map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]any{"type": "text", "text": " there"}})
			notify(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "do"}})
			notify(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "ne"}})
			notify(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t0", "title": "List", "kind": "execute",
				"rawInput": map[string]any{"command": "ls"}, "status": "completed", "content": []map[string]any{{"type": "content", "content": map[string]any{"type": "text", "text": "a.go"}}}})
			reply(m.ID, map[string]any{"configOptions": options()})
		case "session/prompt":
			if len(p.Prompt) > 0 && p.Prompt[0].Text == "Slow" {
				slow = m.ID
				continue
			}
			send(map[string]any{"jsonrpc": "2.0", "id": 100, "method": "session/request_permission", "params": map[string]any{"sessionId": "s1",
				"toolCall": map[string]any{"toolCallId": "t1"},
				"options":  []map[string]any{{"optionId": "no", "kind": "reject_once"}, {"optionId": "once", "kind": "allow_once"}, {"optionId": "always", "kind": "allow_always"}}}})
			chosen := ""
			for in.Scan() {
				var r fakeMessage
				json.Unmarshal(in.Bytes(), &r)
				if string(r.ID) == "100" && r.Method == "" {
					var res struct {
						Outcome struct {
							OptionID string `json:"optionId"`
						} `json:"outcome"`
					}
					json.Unmarshal(r.Result, &res)
					chosen = res.Outcome.OptionID
					break
				}
			}
			notify(map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "Plan"}})
			notify(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t1", "title": "Read a.go", "kind": "read",
				"locations": []map[string]any{{"path": "/w/a.go"}}, "status": "pending"})
			notify(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "completed",
				"content": []map[string]any{{"type": "content", "content": map[string]any{"type": "text", "text": chosen}}}})
			notify(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Do"}})
			notify(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "ne"}})
			notify(map[string]any{"sessionUpdate": "usage_update", "used": 120, "size": 200000})
			notify(map[string]any{"sessionUpdate": "session_info_update", "title": "Fix it"})
			reply(m.ID, map[string]any{"stopReason": "end_turn"})
		case "session/cancel":
			if slow != nil {
				reply(slow, map[string]any{"stopReason": "cancelled"})
				slow = nil
			}
		default:
			if m.ID != nil && m.Method != "" {
				send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": "Method not found"}})
			}
		}
	}
}

func fakeDriver(t *testing.T) (*Driver, func() []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "methods.log")
	d := &Driver{Spec: Spec{Tool: agent.CodeBuddy, Provider: "Fake", Modes: []string{"fullAccess", "bypassPermissions"}},
		Path: exe, Env: append(os.Environ(), "AGIO_FAKE_ACP="+log)}
	methods := func() []string {
		data, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	return d, methods
}

func TestCatalogReadsEachModelsLevelsAndRestoresTheSelection(t *testing.T) {
	d, methods := fakeDriver(t)
	c, err := d.Catalog(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.Model != "a" || c.Effort != "high" || len(c.Models) != 2 {
		t.Fatalf("catalog: %+v", c)
	}
	a, b := c.Models[0], c.Models[1]
	if a.Key() != "codebuddy:a" || a.Name != "Model A" || !a.Default || !a.Images || !slices.Equal(a.Efforts, []string{"low", "high"}) {
		t.Fatalf("model a: %+v", a)
	}
	if b.Detail != "Fast" || b.Default || !slices.Equal(b.Efforts, []string{"medium"}) || b.DefaultEffort != "medium" {
		t.Fatalf("model b: %+v", b)
	}
	log := methods()
	if last := log[len(log)-1]; !strings.Contains(last, `"configId":"model","sessionId":"s1","value":"a"`) {
		t.Fatalf("selection not restored: %v", log)
	}
}

type recorder struct {
	mu     sync.Mutex
	events []agent.Event
}

func (r *recorder) add(e agent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) wait(t *testing.T, settled int) []agent.Event {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		n := 0
		for _, e := range r.events {
			if e.Type == agent.Settled {
				n++
			}
		}
		events := slices.Clone(r.events)
		r.mu.Unlock()
		if n >= settled {
			return events
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the run to settle")
	return nil
}

func TestSystemPromptGoesInMetaOrTheCLIsOption(t *testing.T) {
	d, methods := fakeDriver(t)
	s, _, err := d.Start(agent.StartOptions{Dir: t.TempDir(), SystemPrompt: "Be brief"}, func(agent.Event) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(time.Second)
	if log := strings.Join(methods(), "\n"); !strings.Contains(log, `"_meta":{"systemPrompt":{"append":"Be brief"}}`) {
		t.Fatalf("session/new without _meta: %s", log)
	}

	d, methods = fakeDriver(t)
	d.AppendSystemPrompt = "--append-system-prompt"
	s, _, err = d.Start(agent.StartOptions{Dir: t.TempDir(), SystemPrompt: "Be brief"}, func(agent.Event) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(time.Second)
	if log := methods(); log[0] != "args --append-system-prompt Be brief" || strings.Contains(strings.Join(log, "\n"), "_meta") {
		t.Fatalf("launch: %q", log)
	}
}

func TestSkillsGoInATemporaryPlugin(t *testing.T) {
	d, methods := fakeDriver(t)
	d.PluginDir = "--plugin-dir"
	skill := t.TempDir()
	os.WriteFile(filepath.Join(skill, agent.SkillFile), nil, 0o644)
	s, _, err := d.Start(agent.StartOptions{Dir: t.TempDir(), Skills: []string{skill}}, func(agent.Event) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plugin, ok := strings.CutPrefix(methods()[0], "args --plugin-dir=")
	if _, err := os.Stat(filepath.Join(plugin, "skills", filepath.Base(skill), agent.SkillFile)); !ok || err != nil {
		t.Fatalf("launch %q: %v", methods()[0], err)
	}
	s.Close(time.Second)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(plugin); os.IsNotExist(err) {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("plugin left behind: %v", err)
		}
	}

	// Without the CLI's option the agent gets none.
	d, methods = fakeDriver(t)
	s, _, err = d.Start(agent.StartOptions{Dir: t.TempDir(), Skills: []string{skill}}, func(agent.Event) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(time.Second)
	if log := methods(); strings.HasPrefix(log[0], "args") {
		t.Fatalf("launch: %q", log)
	}
}

func TestSessionStreamsApprovesAndCancels(t *testing.T) {
	d, methods := fakeDriver(t)
	var r recorder
	s, info, err := d.Start(agent.StartOptions{Dir: t.TempDir(), Settings: agent.Settings{Model: "b", Effort: "medium"}}, r.add, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(time.Second)
	if info.Cursor != "s1" || info.Settings != (agent.Settings{Model: "b", Effort: "medium"}) {
		t.Fatalf("info: %+v", info)
	}
	if log := strings.Join(methods(), "\n"); !strings.Contains(log, `"value":"bypassPermissions"`) {
		t.Fatalf("full access not selected: %s", log)
	}
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "go", Images: []agent.Image{{MimeType: "image/png", Data: "AA=="}}}); err != nil {
		t.Fatal(err)
	}
	events := r.wait(t, 1)
	var got []string
	for _, e := range events {
		got = append(got, fmt.Sprintf("%s %s %s %s %s %d/%d", e.Type, e.Segment, e.Tool, e.Preview, e.Text, e.Tokens, e.Window))
	}
	want := []string{
		"started     0/0",
		"thinking_delta thinking-1   Plan 0/0",
		"tool_start t1 read /w/a.go  0/0",
		"tool_end t1   always 0/0",
		"text_delta text-2   Do 0/0",
		"text_delta text-2   ne 0/0",
		"usage     120/200000",
		"title    Fix it 0/0",
		"settled     0/0",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("events:\n%s", strings.Join(got, "\n"))
	}
	if log := strings.Join(methods(), "\n"); !strings.Contains(log, `{"data":"AA==","mimeType":"image/png","type":"image"}`) {
		t.Fatalf("image not sent: %s", log)
	}

	if err := s.Configure(context.Background(), agent.Settings{Model: "a", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	if log := methods(); !strings.Contains(log[len(log)-1], `"configId":"thought","sessionId":"s1","value":"low"`) {
		t.Fatalf("level not applied: %v", log)
	}

	if err := s.Prompt(context.Background(), agent.Prompt{Text: "Slow"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	events = r.wait(t, 2)
	if last := events[len(events)-1]; last.Type != agent.Settled || !last.Aborted {
		t.Fatalf("abort: %+v", last)
	}
}

func TestResumeDiscardsReplayAndHistoryJoinsIt(t *testing.T) {
	d, methods := fakeDriver(t)
	var r recorder
	s, info, err := d.Start(agent.StartOptions{Dir: t.TempDir(), Cursor: "s1"}, r.add, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(time.Second)
	if info.Cursor != "s1" || !strings.HasPrefix(methods()[1], "session/load") {
		t.Fatalf("resume: %+v %v", info, methods())
	}
	r.mu.Lock()
	replayed := len(r.events)
	r.mu.Unlock()
	if replayed != 0 {
		t.Fatalf("replay reported: %+v", r.events)
	}

	events, err := d.History("s1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []agent.Event{
		{Type: agent.UserMessage, Text: "hi there"},
		{Type: agent.TextDone, Segment: "text-2", Text: "done"},
		{Type: agent.ToolStart, Segment: "t0", Tool: "bash", Preview: "ls"},
		{Type: agent.ToolEnd, Segment: "t0", Text: "a.go"},
	}
	if !slices.Equal(events, want) {
		t.Fatalf("history: %+v", events)
	}
}

func TestPermissionWithoutAllowOptionIsCancelled(t *testing.T) {
	res, rpcErr := (&session{}).request("session/request_permission", json.RawMessage(`{"options":[{"optionId":"no","kind":"reject_once"}]}`))
	data, _ := json.Marshal(res)
	if rpcErr != nil || string(data) != `{"outcome":{"outcome":"cancelled"}}` {
		t.Fatalf("answer: %s %v", data, rpcErr)
	}
	if _, rpcErr := (&session{}).request("fs/read_text_file", nil); rpcErr != errMethodNotFound {
		t.Fatalf("fs request: %v", rpcErr)
	}
}

func TestToolCallUpdatesKeepTheCallsKind(t *testing.T) {
	var st stream
	title := "Edit main.go"
	st.events(update{SessionUpdate: "tool_call", ToolCallID: "t", Kind: "edit", Title: &title, RawInput: json.RawMessage(`{"file_path":"main.go"}`)}, false)
	renamed := "Edited main.go"
	got := st.events(update{SessionUpdate: "tool_call_update", ToolCallID: "t", Title: &renamed, Status: "failed"}, false)
	want := []agent.Event{{Type: agent.ToolEnd, Segment: "t", Failed: true}}
	if !slices.Equal(got, want) {
		t.Fatalf("update: %+v", got)
	}
}

func TestToolOutputSkipsStreamedArgumentsAndReadsRawOutput(t *testing.T) {
	var st stream
	title := "Bash"
	got := st.events(update{SessionUpdate: "tool_call", ToolCallID: "t", Kind: "other", Title: &title, RawInput: json.RawMessage(`{}`)}, false)
	got = append(got, st.events(update{SessionUpdate: "tool_call", ToolCallID: "t", Kind: "execute", RawInput: json.RawMessage(`{"command":"ls","description":"List files"}`),
		Content: json.RawMessage(`[{"type":"content","content":{"type":"text","text":"List files"}}]`)}, false)...)
	got = append(got, st.events(update{SessionUpdate: "tool_call_update", ToolCallID: "t", Status: "completed", RawOutput: json.RawMessage(`{"type":"text","text":"a.txt"}`)}, false)...)
	want := []agent.Event{
		{Type: agent.ToolStart, Segment: "t", Tool: "Bash"},
		{Type: agent.ToolStart, Segment: "t", Tool: "bash", Preview: "ls"},
		{Type: agent.ToolEnd, Segment: "t", Text: "a.txt"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("events: %+v", got)
	}
}
