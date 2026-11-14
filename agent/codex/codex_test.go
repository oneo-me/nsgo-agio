package codex

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

// fakeCodex answers the app-server requests Agio sends, appending them to
// requests.txt beside the script. A "Slow" prompt starts a turn that only an
// interrupt completes.
const fakeCodex = `#!/bin/sh
[ "$1" = "app-server" ] || exit 2
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$(dirname "$0")/requests.txt"
  id=$(printf '%s' "$line" | sed -n 's/^{"id":\([0-9]*\),.*/\1/p')
  case "$line" in
    *'"method":"initialize"'*) printf '{"id":%s,"result":{"userAgent":"fake"}}\n' "$id" ;;
    *'"method":"initialized"'*) ;;
    *'"method":"model/list"'*)
      printf '{"id":%s,"result":{"data":[{"model":"gpt-6-luna","displayName":"gpt-6-luna","hidden":false,"isDefault":true,"defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"none"},{"reasoningEffort":"low"},{"reasoningEffort":"medium"}],"inputModalities":["text","image"],"serviceTiers":[{"id":"priority","name":"Fast","description":"2x speed"}]},{"model":"hidden-one","hidden":true}],"nextCursor":null}}\n' "$id" ;;
    *'"method":"thread/start"'*|*'"method":"thread/resume"'*)
      printf '{"method":"thread/started","params":{"thread":{"id":"t1"}}}\n'
      printf '{"id":%s,"result":{"thread":{"id":"t1"},"model":"gpt-6-luna","reasoningEffort":"low"}}\n' "$id" ;;
    *'"method":"thread/name/set"'*|*'"method":"skills/extraRoots/set"'*) printf '{"id":%s,"result":{}}\n' "$id" ;;
    *'"method":"turn/start"'*'"text":"Slow'*)
      printf '{"id":%s,"result":{"turn":{"id":"u2","status":"inProgress"}}}\n' "$id"
      printf '{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u2"}}}\n' ;;
    *'"method":"turn/start"'*)
      printf '{"id":%s,"result":{"turn":{"id":"u1","status":"inProgress"}}}\n' "$id"
      printf '{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1"}}}\n'
      printf '{"method":"item/started","params":{"item":{"type":"userMessage","id":"i0","content":[{"type":"text","text":"go"}]}}}\n'
      printf '{"method":"item/started","params":{"item":{"type":"commandExecution","id":"c1","command":"/bin/zsh -lc '"'"'echo hi'"'"'","commandActions":[{"type":"unknown","command":"echo hi"}],"status":"inProgress"}}}\n'
      printf '{"method":"item/commandExecution/outputDelta","params":{"itemId":"c1","delta":"hi\\n"}}\n'
      printf '{"method":"item/completed","params":{"item":{"type":"commandExecution","id":"c1","command":"echo hi","commandActions":[{"type":"unknown","command":"echo hi"}],"status":"completed","aggregatedOutput":"hi\\n","exitCode":0}}}\n'
      printf '{"method":"item/reasoning/summaryTextDelta","params":{"itemId":"r1","delta":"Plan","summaryIndex":0}}\n'
      printf '{"method":"item/reasoning/summaryTextDelta","params":{"itemId":"r1","delta":"Check","summaryIndex":1}}\n'
      printf '{"method":"item/agentMessage/delta","params":{"itemId":"m1","delta":"do"}}\n'
      printf '{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"m1","text":"done"}}}\n'
      printf '{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"last":{"totalTokens":140},"modelContextWindow":258400}}}\n'
      printf '{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"u1","status":"completed"}}}\n' ;;
    *'"method":"turn/interrupt"'*)
      printf '{"id":%s,"result":{}}\n' "$id"
      printf '{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"u2","status":"interrupted"}}}\n' ;;
    *'"method":"thread/turns/list"'*)
      printf '{"id":%s,"result":{"data":[{"id":"u1","status":"completed","startedAt":100,"completedAt":160,"items":[{"type":"userMessage","id":"i0","content":[{"type":"text","text":"go"}]},{"type":"fileChange","id":"f1","status":"completed","changes":[{"path":"a.go","diff":"+x"}]},{"type":"agentMessage","id":"m1","text":"done"}]},{"id":"u2","status":"interrupted","startedAt":200,"items":[{"type":"userMessage","id":"i1","content":[{"type":"text","text":"Slow"}]}]}],"nextCursor":null}}\n' "$id" ;;
    *) [ -n "$id" ] && printf '{"id":%s,"error":{"message":"unsupported"}}\n' "$id" ;;
  esac
done
`

func fakeDriver(t *testing.T) *Driver {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix shell")
	}
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte(fakeCodex), 0755); err != nil {
		t.Fatal(err)
	}
	return &Driver{Path: path, Env: os.Environ()}
}

func TestCatalogReadsModelsAndTiers(t *testing.T) {
	catalog, err := fakeDriver(t).Catalog(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 1 || catalog.Model != "gpt-6-luna" || catalog.Effort != "medium" {
		t.Fatalf("catalog: %+v", catalog)
	}
	m := catalog.Models[0]
	if m.Name != "GPT-6-Luna" || m.Key() != "codex:gpt-6-luna" || !m.Images || len(m.Efforts) != 2 || m.HasEffort("none") {
		t.Fatalf("model: %+v", m)
	}
	if len(m.Tiers) != 1 || m.Tiers[0].ID != "priority" || !m.HasTier("") || m.HasTier("flex") {
		t.Fatalf("tiers: %+v", m.Tiers)
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
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		n := 0
		for _, e := range r.events {
			if e.Type == agent.Settled {
				n++
			}
		}
		events := append([]agent.Event(nil), r.events...)
		r.mu.Unlock()
		if n >= settled {
			return events
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the run to settle")
	return nil
}

func TestSessionStreamsInterruptsAndReadsHistory(t *testing.T) {
	d := fakeDriver(t)
	var r recorder
	s, info, err := d.Start(agent.StartOptions{Dir: t.TempDir(), Title: "Task", SystemPrompt: "Be brief", Skills: []string{"/skills/review"},
		Settings: agent.Settings{Model: "gpt-6-luna", Tier: "priority"}}, r.add, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(time.Second)
	requests, _ := os.ReadFile(filepath.Join(filepath.Dir(d.Path), "requests.txt"))
	roots, start := strings.Index(string(requests), `"extraRoots":["/skills/review"]`), strings.Index(string(requests), `"method":"thread/start"`)
	if !strings.Contains(string(requests), `"developerInstructions":"Be brief"`) || roots < 0 || roots > start {
		t.Fatalf("requests: %s", requests)
	}
	if info.Cursor != "t1" || info.Settings.Model != "gpt-6-luna" || info.Settings.Effort != "low" {
		t.Fatalf("info: %+v", info)
	}
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	events := r.wait(t, 1)
	find := func(typ agent.EventType, segment string) agent.Event {
		for _, e := range events {
			if e.Type == typ && e.Segment == segment {
				return e
			}
		}
		t.Fatalf("missing event %d %q in %+v", typ, segment, events)
		return agent.Event{}
	}
	if e := find(agent.ToolStart, "c1"); e.Tool != "bash" || e.Preview != "echo hi" {
		t.Fatalf("command: %+v", e)
	}
	if e := find(agent.ToolEnd, "c1"); e.Text != "hi\n" || e.Failed {
		t.Fatalf("command result: %+v", e)
	}
	if e := find(agent.TextDone, "m1"); e.Text != "done" {
		t.Fatalf("answer: %+v", e)
	}
	var thinking string
	for _, e := range events {
		if e.Type == agent.ThinkingDelta {
			thinking += e.Text
		}
	}
	if thinking != "Plan\n\nCheck" {
		t.Fatalf("reasoning parts: %q", thinking)
	}
	if e := find(agent.Usage, ""); e.Tokens != 140 || e.Window != 258400 {
		t.Fatalf("usage: %+v", e)
	}
	if events[0].Type != agent.Started || events[len(events)-1].Type != agent.Settled {
		t.Fatalf("run bounds: %+v", events)
	}

	// Abort returns only after the interrupted turn has settled.
	if err := s.Prompt(context.Background(), agent.Prompt{Text: "Slow"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	last := r.events[len(r.events)-1]
	r.mu.Unlock()
	if last.Type != agent.Settled || !last.Aborted {
		t.Fatalf("abort did not wait for the settled run: %+v", last)
	}

	history, err := d.History("t1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var kinds []agent.EventType
	for _, e := range history {
		kinds = append(kinds, e.Type)
	}
	want := []agent.EventType{agent.UserMessage, agent.ToolStart, agent.ToolEnd, agent.TextDone, agent.UserMessage, agent.Cancelled}
	if len(kinds) != len(want) {
		t.Fatalf("history: %+v", history)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("history: %+v", history)
		}
	}
	if history[0].Text != "go" || history[0].Time != time.Unix(100, 0) || history[1].Tool != "edit" || history[1].Preview != "a.go" {
		t.Fatalf("history details: %+v", history)
	}
}

func TestRetriedErrorsShowUntilOutputResumes(t *testing.T) {
	var events []agent.Event
	s := &session{emitter: agent.NewEmitter(func(e agent.Event) { events = append(events, e) }), outputs: map[string]string{}}
	s.emitter.Open()
	for _, line := range []string{
		`{"method":"turn/started","params":{"turn":{"id":"u1"}}}`,
		`{"method":"item/started","params":{"item":{"type":"userMessage","id":"i0"}}}`,
		`{"method":"error","params":{"error":{"message":"Reconnecting... 2/5","additionalDetails":"Connection refused (os error 61)"},"willRetry":true}}`,
		`{"method":"item/agentMessage/delta","params":{"itemId":"m1","delta":"hi"}}`,
		`{"method":"item/agentMessage/delta","params":{"itemId":"m1","delta":"!"}}`,
	} {
		var m message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		s.handle(m)
	}
	var errs []string
	for _, e := range events {
		if e.Type == agent.Error {
			errs = append(errs, e.Text)
		}
	}
	if len(errs) != 2 || errs[0] != "Reconnecting... 2/5: Connection refused (os error 61)" || errs[1] != "" {
		t.Fatalf("errors: %q", errs)
	}
}
