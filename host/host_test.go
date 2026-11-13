package host

import (
	"strings"
	"testing"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/internal/fakeagent"
	"github.com/oneo-me/nsgo-agio/transcript"
)

func newHost(t *testing.T, d *fakeagent.Driver) *Host {
	t.Helper()
	h := New(Options{Env: []string{}, Tools: []agent.Tool{agent.Claude}, Drivers: map[agent.Tool]agent.Driver{agent.Claude: d}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	waitFor(t, h, func(s agio.State) bool { return !s.Discovering && s.Ready() })
	return h
}

// waitFor waits until the host's state satisfies ok.
func waitFor(t *testing.T, b agio.Backend, ok func(agio.State) bool) agio.State {
	t.Helper()
	changed := make(chan struct{}, 1)
	stop := b.Watch(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer stop()
	deadline := time.After(5 * time.Second)
	for {
		if s := b.State(); ok(s) {
			return s
		}
		select {
		case <-changed:
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatalf("state never matched: %+v", b.State())
		}
	}
}

func TestSendFoldsTheReply(t *testing.T) {
	d := &fakeagent.Driver{}
	h := newHost(t, d)
	dir := t.TempDir()
	id, err := h.Create(agio.Create{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	c := h.State().Conversation(id)
	if c.Model != "claude:opus" || c.Effort != "high" || c.Bound || c.ContextWindow != 1000 {
		t.Fatalf("new conversation = %+v", c)
	}
	if err := h.Send(id, agio.Message{Text: "hello\nworld"}); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, h, func(s agio.State) bool {
		c := s.Conversation(id)
		return len(c.Turns) == 1 && c.Turns[0].Status == transcript.Completed
	})
	c = s.Conversation(id)
	if final := c.Turns[0].Final(); final == nil || final.Text != "echo: hello\nworld" {
		t.Fatalf("turn = %+v", c.Turns[0])
	}
	if c.Title != "hello" || c.Cursor != "session-1" || !c.Bound || c.Running || c.ContextTokens != 42 {
		t.Fatalf("conversation = %+v", c)
	}
	if started := d.Started(); len(started) != 1 || started[0].Dir != dir || started[0].Settings != (agent.Settings{Model: "opus", Effort: "high"}) {
		t.Fatalf("started = %+v", started)
	}
}

func TestMessageInterruptsTheRun(t *testing.T) {
	h := newHost(t, &fakeagent.Driver{})
	id, _ := h.Create(agio.Create{Dir: t.TempDir()})
	h.Send(id, agio.Message{Text: "wait"})
	waitFor(t, h, func(s agio.State) bool {
		c := s.Conversation(id)
		return len(c.Turns) == 1 && len(c.Turns[0].Segments) == 1
	})
	h.Send(id, agio.Message{Text: "next"})
	s := waitFor(t, h, func(s agio.State) bool {
		c := s.Conversation(id)
		return len(c.Turns) == 2 && c.Turns[1].Status == transcript.Completed
	})
	turns := s.Conversation(id).Turns
	if turns[0].Status != transcript.Cancelled || turns[1].Final().Text != "echo: next" {
		t.Fatalf("turns = %+v, %+v", turns[0], turns[1])
	}
}

func TestStopCancelsTheRun(t *testing.T) {
	h := newHost(t, &fakeagent.Driver{})
	id, _ := h.Create(agio.Create{Dir: t.TempDir()})
	h.Send(id, agio.Message{Text: "wait"})
	waitFor(t, h, func(s agio.State) bool {
		c := s.Conversation(id)
		return len(c.Turns) == 1 && len(c.Turns[0].Segments) == 1
	})
	h.Stop(id)
	s := waitFor(t, h, func(s agio.State) bool { return !s.Conversation(id).Running })
	if st := s.Conversation(id).Turns[0].Status; st != transcript.Cancelled {
		t.Fatalf("status = %v", st)
	}
}

func TestChooseConfiguresTheAgent(t *testing.T) {
	d := &fakeagent.Driver{}
	h := newHost(t, d)
	id, _ := h.Create(agio.Create{Dir: t.TempDir()})
	haiku := "claude:haiku"
	if err := h.Choose(id, agio.Choice{Model: &haiku}); err != nil {
		t.Fatal(err)
	}
	if c := h.State().Conversation(id); c.Model != haiku || c.Effort != "" {
		t.Fatalf("after choosing = %+v", c)
	}
	h.Send(id, agio.Message{Text: "hi"})
	waitFor(t, h, func(s agio.State) bool { return s.Conversation(id).Cursor != "" })
	opus, low := "claude:opus", "low"
	h.Choose(id, agio.Choice{Model: &opus})
	h.Choose(id, agio.Choice{Effort: &low})
	want := agent.Settings{Model: "opus", Effort: "low"}
	waitFor(t, h, func(agio.State) bool { c := d.Configured(); return len(c) > 0 && c[len(c)-1] == want })
	other := "codex:gpt"
	if err := h.Choose(id, agio.Choice{Model: &other}); err == nil {
		t.Fatal("chose a model of another tool")
	}
}

func TestResumeReadsHistory(t *testing.T) {
	d := &fakeagent.Driver{Saved: []agent.Event{
		{Type: agent.UserMessage, Text: "before"},
		{Type: agent.TextDone, Segment: "a", Text: "earlier reply"},
		{Type: agent.Usage, Tokens: 7},
	}}
	h := newHost(t, d)
	id, err := h.Create(agio.Create{Dir: t.TempDir(), Cursor: "saved", Model: "claude:opus"})
	if err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, h, func(s agio.State) bool { c := s.Conversation(id); return !c.Loading && len(c.Turns) == 1 })
	c := s.Conversation(id)
	if c.Turns[0].Prompt != "before" || !strings.Contains(c.Turns[0].Final().Text, "earlier") || c.ContextTokens != 7 {
		t.Fatalf("history = %+v", c)
	}
}
