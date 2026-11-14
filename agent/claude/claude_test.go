package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// fakeClaude answers initialization and streams one run per user message. A
// "Slow" message starts a run that only an interrupt settles. "Background"
// leaves a background task that resumes the conversation when it ends, and
// "Linger" one that only stop_task ends. Arguments are appended to args.txt
// and stopped tasks to stopped.txt beside the script.
const fakeClaude = `#!/bin/sh
echo "$*" >> "$(dirname "$0")/args.txt"
while IFS= read -r line; do
  case "$line" in
    *'"subtype":"initialize"'*)
      printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"agio-catalog","response":{"models":[{"value":"default","resolvedModel":"claude-opus-5-5","displayName":"Default (recommended)","description":"Opus 5.5 · Best for everyday tasks","supportedEffortLevels":["low","medium","high"]},{"value":"opus","resolvedModel":"claude-opus-5-5","displayName":"Opus 5.5","supportedEffortLevels":["none","low","medium","high"]},{"value":"haiku","resolvedModel":"claude-haiku-4-5","displayName":"Haiku 4.5"}]}}}' ;;
    *'"subtype":"interrupt"'*)
      printf '%s\n' '{"type":"result","subtype":"error_during_execution","is_error":true}' ;;
    *'"subtype":"stop_task"'*)
      echo "$line" >> "$(dirname "$0")/stopped.txt"
      printf '%s\n' '{"type":"system","subtype":"background_tasks_changed","tasks":[]}' ;;
    *'"text":"Background'*|*'"text":"Linger'*)
      printf '%s\n' '{"type":"stream_event","event":{"type":"message_start","message":{"id":"b1"}},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"bt1","task_type":"local_bash"}]}'
      printf '%s\n' '{"type":"assistant","message":{"id":"b1","content":[{"type":"text","text":"started"}]},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"started"}'
      case "$line" in *Background*)
        sleep 0.2
        printf '%s\n' '{"type":"system","subtype":"background_tasks_changed","tasks":[]}'
        printf '%s\n' '{"type":"system","subtype":"task_notification","task_id":"bt1","status":"completed"}'
        printf '%s\n' '{"type":"stream_event","event":{"type":"message_start","message":{"id":"b2"}},"parent_tool_use_id":null}'
        printf '%s\n' '{"type":"assistant","message":{"id":"b2","content":[{"type":"text","text":"checked"}]},"parent_tool_use_id":null}'
        printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"checked"}' ;;
      esac ;;
    *'"text":"Slow'*)
      printf '%s\n' '{"type":"stream_event","event":{"type":"message_start","message":{"id":"m9"}},"parent_tool_use_id":null}' ;;
    *'"type":"user"'*)
      printf '%s\n' '{"type":"stream_event","event":{"type":"message_start","message":{"id":"m1"}},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Plan"}},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"assistant","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"Plan"}]},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"assistant","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"echo hi"}}],"usage":{"input_tokens":10,"cache_read_input_tokens":90,"output_tokens":20}},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"assistant","message":{"id":"s1","content":[{"type":"text","text":"subagent"}]},"parent_tool_use_id":"tu1"}'
      printf '%s\n' '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"hi","is_error":false}]},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"message_start","message":{"id":"m2"}},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"do"}},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"assistant","message":{"id":"m2","model":"claude-opus-5-5","content":[{"type":"text","text":"done"}]},"parent_tool_use_id":null}'
      printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"done","modelUsage":{"claude-opus-5-5":{"contextWindow":200000}}}' ;;
  esac
done
`

func fakeDriver(t *testing.T) (*Driver, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix shell")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte(fakeClaude), 0755); err != nil {
		t.Fatal(err)
	}
	return &Driver{Path: path, Env: append(os.Environ(), "CLAUDE_CONFIG_DIR="+filepath.Join(dir, "config"))}, dir
}

func TestCatalogFoldsTheDefaultAlias(t *testing.T) {
	d, _ := fakeDriver(t)
	catalog, err := d.Catalog(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 2 || catalog.Model != "opus" || catalog.Effort != "high" {
		t.Fatalf("catalog: %+v", catalog)
	}
	opus, haiku := catalog.Models[0], catalog.Models[1]
	if opus.Name != "Claude Opus 5.5" || opus.Detail != "claude-opus-5-5" || !opus.Default || opus.Key() != "claude:opus" || opus.HasEffort("none") {
		t.Fatalf("opus: %+v", opus)
	}
	if haiku.Name != "Claude Haiku 4.5" || len(haiku.Efforts) != 0 || haiku.DefaultEffort != "" {
		t.Fatalf("haiku: %+v", haiku)
	}
	// A default alias resolving to an unlisted model keeps its own entry,
	// named after the model its description names.
	c := parseCatalog(t, `[{"value":"default","resolvedModel":"x-1","displayName":"Default (recommended)","description":"X One · Fast"}]`)
	if len(c.Models) != 1 || c.Models[0].Name != "Claude X One" || c.Model != "default" {
		t.Fatalf("unlisted default: %+v", c)
	}
}

func parseCatalog(t *testing.T, models string) agent.Catalog {
	t.Helper()
	var list []modelInfo
	if err := json.Unmarshal([]byte(models), &list); err != nil {
		t.Fatal(err)
	}
	return catalog(list)
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

func (r *recorder) settled(t *testing.T, n int) []agent.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		events := append([]agent.Event(nil), r.events...)
		r.mu.Unlock()
		count := 0
		for _, e := range events {
			if e.Type == agent.Settled {
				count++
			}
		}
		if count >= n {
			return events
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the run to settle")
	return nil
}

func TestSessionStreamsInterruptsAndRelaunchesForEffort(t *testing.T) {
	d, dir := fakeDriver(t)
	var r recorder
	skill := t.TempDir()
	s, info, err := d.Start(agent.StartOptions{Dir: t.TempDir(), SystemPrompt: "Be brief", Skills: []string{skill},
		Settings: agent.Settings{Model: "opus", Effort: "high"}}, r.add, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(time.Second)
	if len(info.Cursor) != 36 {
		t.Fatalf("session id: %q", info.Cursor)
	}
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	events := r.settled(t, 1)
	var text, thinking string
	var tool, result, usage agent.Event
	for _, e := range events {
		switch e.Type {
		case agent.TextDelta:
			text += e.Text
		case agent.ThinkingDelta:
			thinking += e.Text
		case agent.ToolStart:
			tool = e
		case agent.ToolEnd:
			result = e
		case agent.TextDone:
			if e.Text == "subagent" {
				t.Fatal("subagent text reached the conversation")
			}
		case agent.Usage:
			if e.Tokens > 0 {
				usage.Tokens = e.Tokens
			}
			if e.Window > 0 {
				usage.Window = e.Window
			}
		}
	}
	if events[0].Type != agent.Started || text != "do" || thinking != "Plan" {
		t.Fatalf("stream: %+v", events)
	}
	if tool.Segment != "tu1" || tool.Tool != "bash" || tool.Preview != "echo hi" || result.Text != "hi" || result.Failed {
		t.Fatalf("tool: %+v %+v", tool, result)
	}
	if usage.Tokens != 120 || usage.Window != 200000 {
		t.Fatalf("usage: %+v", usage)
	}

	if err := s.Prompt(context.Background(), agent.Prompt{Text: "Slow"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	last := r.events[len(r.events)-1]
	for _, e := range r.events {
		if e.Type == agent.Error {
			t.Fatalf("an interrupt reported an error: %+v", e)
		}
	}
	r.mu.Unlock()
	if last.Type != agent.Settled || !last.Aborted {
		t.Fatalf("abort did not wait for the result: %+v", last)
	}

	// A new reasoning level resumes the same session in a new process.
	s.Configure(context.Background(), agent.Settings{Model: "opus", Effort: "low"})
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	r.settled(t, 3)
	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	lines := strings.Split(strings.TrimSpace(string(args)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "--session-id "+info.Cursor) ||
		!strings.Contains(lines[1], "--resume "+info.Cursor) || !strings.Contains(lines[1], "--effort low") ||
		!strings.Contains(lines[1], "--dangerously-skip-permissions") ||
		!strings.Contains(lines[0], "--append-system-prompt Be brief") || !strings.Contains(lines[1], "--append-system-prompt Be brief") {
		t.Fatalf("launches: %q", lines)
	}
	// Both launches load the same plugin, which Close removes.
	plugin := s.(*session).plugin
	if target, _ := os.Readlink(filepath.Join(plugin, "skills", filepath.Base(skill))); target != skill ||
		!strings.Contains(lines[0], "--plugin-dir "+plugin) || !strings.Contains(lines[1], "--plugin-dir "+plugin) {
		t.Fatalf("plugin %s links %q, launches %q", plugin, target, lines)
	}
	s.Close(time.Second)
	if _, err := os.Stat(plugin); !os.IsNotExist(err) {
		t.Fatalf("plugin left behind: %v", err)
	}
}

func TestHistoryFollowsTheActiveBranch(t *testing.T) {
	d, dir := fakeDriver(t)
	project := filepath.Join(dir, "config", "projects", "-work-demo")
	os.MkdirAll(project, 0755)
	lines := []string{
		`{"type":"user","uuid":"u1","parentUuid":null,"timestamp":"2026-10-09T10:00:00Z","message":{"role":"user","content":"Fix it"}}`,
		`{"type":"attachment","uuid":"a1","parentUuid":"u1"}`,
		`{"type":"assistant","uuid":"x1","parentUuid":"a1","timestamp":"2026-10-09T10:00:05Z","message":{"content":[{"type":"text","text":"abandoned"}]}}`,
		`{"type":"assistant","uuid":"b1","parentUuid":"a1","timestamp":"2026-10-09T10:00:06Z","message":{"content":[{"type":"tool_use","id":"tu","name":"Read","input":{"file_path":"a.go"}}],"usage":{"input_tokens":5,"output_tokens":5}}}`,
		`{"type":"user","uuid":"b2","parentUuid":"b1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu","content":[{"type":"text","text":"package a"}]}]}}`,
		`{"type":"assistant","uuid":"b3","parentUuid":"b2","isSidechain":true,"message":{"content":[{"type":"text","text":"side"}]}}`,
		`{"type":"assistant","uuid":"b4","parentUuid":"b2","timestamp":"2026-10-09T10:00:09Z","message":{"content":[{"type":"text","text":"Fixed"}]}}`,
		`{"type":"user","uuid":"n1","parentUuid":"b4","origin":{"kind":"task-notification"},"message":{"role":"user","content":"<task-notification></task-notification>"}}`,
		`{"type":"user","uuid":"b5","parentUuid":"n1","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
	}
	os.WriteFile(filepath.Join(project, "sid.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0644)
	events, err := d.History("sid", "")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range events {
		got = append(got, strings.TrimSpace(e.Tool+" "+e.Text))
	}
	want := []string{"Fix it", "read", "", "package a", "Fixed", ""}
	if strings.Join(got, "|") != strings.Join(want, "|") || events[5].Type != agent.Cancelled || events[2].Tokens != 10 {
		t.Fatalf("history: %q %+v", got, events)
	}
}

func TestRetriedRequestsShowUntilAResponseStarts(t *testing.T) {
	var events []agent.Event
	s := &session{emitter: agent.NewEmitter(func(e agent.Event) { events = append(events, e) }), stream: streamState{blocks: map[string]int{}}}
	s.emitter.Open()
	for _, line := range []string{
		`{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"error_status":null,"error":"unknown"}`,
		`{"type":"system","subtype":"api_retry","attempt":2,"max_retries":10,"error_status":529,"error":"server_error"}`,
		`{"type":"stream_event","event":{"type":"message_start","message":{"id":"m1"}}}`,
	} {
		s.handle(nil, []byte(line))
	}
	var errs []string
	for _, e := range events {
		if e.Type == agent.Error {
			errs = append(errs, e.Text)
		}
	}
	want := []string{"API request failed (connection error), retrying 1/10", "API request failed (HTTP 529 server_error), retrying 2/10", ""}
	if strings.Join(errs, "|") != strings.Join(want, "|") {
		t.Fatalf("errors: %q", errs)
	}
}

func TestRunsLastUntilBackgroundTasksEnd(t *testing.T) {
	d, dir := fakeDriver(t)
	var r recorder
	s, _, err := d.Start(agent.StartOptions{Dir: t.TempDir()}, r.add, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(time.Second)
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "Background"}); err != nil {
		t.Fatal(err)
	}
	// The run that resumes after the task ends belongs to the same run, so
	// its reply is the final one.
	var texts []string
	for _, e := range r.settled(t, 1) {
		if e.Type == agent.TextDone {
			texts = append(texts, e.Text)
		}
	}
	if strings.Join(texts, "|") != "started|checked" {
		t.Fatalf("texts before settling: %q", texts)
	}

	// Aborting while only a background task runs stops it and settles
	// without waiting for a result that never comes.
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "Linger"}); err != nil {
		t.Fatal(err)
	}
	session := s.(*session)
	for deadline := time.Now().Add(5 * time.Second); ; {
		session.mu.Lock()
		waiting := session.waiting
		session.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the run did not wait for its background task")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	events := r.settled(t, 2)
	if last := events[len(events)-1]; last.Type != agent.Settled || !last.Aborted {
		t.Fatalf("abort: %+v", last)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		stopped, _ := os.ReadFile(filepath.Join(dir, "stopped.txt"))
		if strings.Contains(string(stopped), `"task_id":"bt1"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stopped: %q", stopped)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	r.settled(t, 3)
}
