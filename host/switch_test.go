package host

import (
	"slices"
	"strings"
	"testing"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/internal/fakeagent"
	"github.com/oneo-me/nsgo-agio/transcript"
)

func newSwitchingHost(t *testing.T, claude, codex *fakeagent.Driver) *Host {
	t.Helper()
	h := New(Options{Env: []string{}, Tools: []agent.Tool{agent.Claude, agent.Codex},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: claude, agent.Codex: codex}, Home: t.TempDir(), SystemPrompt: "Be brief."})
	t.Cleanup(func() { h.Close() })
	waitFor(t, h, func(s agio.State) bool { return !s.Discovering && len(s.Models()) == 4 })
	return h
}

func TestChooseSwitchesTheAgentBetweenRuns(t *testing.T) {
	claude, codex := &fakeagent.Driver{}, &fakeagent.Driver{As: agent.Codex}
	h := newSwitchingHost(t, claude, codex)
	dir := t.TempDir()
	id, _ := h.Create(agio.Create{Dir: dir, Model: "claude:opus"})
	idle := func(s agio.State) bool { c := s.Conversation(id); return !c.Running && !c.Starting }
	h.Send(id, agio.Message{Text: "hi"})
	waitFor(t, h, func(s agio.State) bool { return idle(s) && s.Conversation(id).Cursor != "" })
	h.Send(id, agio.Message{Text: "wait"})
	waitFor(t, h, func(s agio.State) bool { return s.Conversation(id).Running })
	other := "codex:haiku"
	if err := h.Choose(id, agio.Choice{Model: &other}); err == nil {
		t.Fatal("switched the agent while it ran")
	}
	h.Stop(id)
	waitFor(t, h, idle)
	if err := h.Choose(id, agio.Choice{Model: &other}); err != nil {
		t.Fatal(err)
	}
	c := h.State().Conversation(id)
	if c.Model != other || c.Cursor != "" || !c.Bound ||
		!slices.Equal(c.Earlier, []agio.Session{{Tool: agent.Claude, Cursor: "claude-session"}}) {
		t.Fatalf("after switching = %+v", c)
	}
	h.Send(id, agio.Message{Text: "next"})
	s := waitFor(t, h, func(s agio.State) bool { return idle(s) && s.Conversation(id).Cursor == "codex-session" })
	if c := s.Conversation(id); len(c.Turns) != 3 || c.Turns[2].Prompt != "next" || !strings.HasSuffix(c.Turns[2].Final().Text, "next") ||
		!slices.Equal(c.Earlier, []agio.Session{{Tool: agent.Claude, Cursor: "claude-session"}}) {
		t.Fatalf("after the new agent's run = %+v", c)
	}
	started := codex.Started()
	if len(started) != 1 || started[0].Dir != dir || started[0].Cursor != "" || started[0].SystemPrompt != "Be brief." ||
		started[0].Settings.Model != "haiku" {
		t.Fatalf("codex started = %+v", started)
	}
	prompts := codex.Prompted()
	if len(prompts) != 1 || !strings.Contains(prompts[0], "## User\n\nhi") || !strings.Contains(prompts[0], "echo: hi") ||
		!strings.Contains(prompts[0], "(stopped by the user)") || !strings.HasSuffix(prompts[0], "\n\nnext") {
		t.Fatalf("codex prompts = %q", prompts)
	}
	h.Send(id, agio.Message{Text: "again"})
	waitFor(t, h, func(s agio.State) bool { return idle(s) && len(s.Conversation(id).Turns) == 4 })
	if prompts := codex.Prompted(); len(prompts) != 2 || prompts[1] != "again" {
		t.Fatalf("codex prompts = %q", prompts)
	}
}

func TestResumeReadsEveryAgentsHistory(t *testing.T) {
	claude := &fakeagent.Driver{Saved: []agent.Event{
		{Type: agent.UserMessage, Text: "before"},
		{Type: agent.TextDone, Segment: "a", Text: "first reply"},
		{Type: agent.Usage, Tokens: 7},
	}}
	earlier := []*transcript.Turn{{Prompt: "before", Status: transcript.Completed}}
	codex := &fakeagent.Driver{As: agent.Codex, Saved: []agent.Event{
		{Type: agent.UserMessage, Text: handoff(earlier, "after")},
		{Type: agent.TextDone, Segment: "b", Text: "second reply"},
		{Type: agent.Usage, Tokens: 9},
	}}
	h := newSwitchingHost(t, claude, codex)
	id, err := h.Create(agio.Create{Dir: t.TempDir(), Model: "codex:opus", Cursor: "c2",
		Earlier: []agio.Session{{Tool: agent.Claude, Cursor: "c1"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, h, func(s agio.State) bool { c := s.Conversation(id); return !c.Loading && len(c.Turns) == 2 })
	c := s.Conversation(id)
	if !c.Bound || c.Turns[0].Prompt != "before" || c.Turns[1].Prompt != "after" || c.ContextTokens != 9 {
		t.Fatalf("history = %+v, turns %+v %+v", c, c.Turns[0], c.Turns[1])
	}
}

func TestAResumedSwitchHandsTheAgentTheEarlierTurns(t *testing.T) {
	claude := &fakeagent.Driver{Saved: []agent.Event{
		{Type: agent.UserMessage, Text: "before"},
		{Type: agent.TextDone, Segment: "a", Text: "first reply"},
	}}
	codex := &fakeagent.Driver{As: agent.Codex}
	h := newSwitchingHost(t, claude, codex)
	id, err := h.Create(agio.Create{Dir: t.TempDir(), Model: "codex:opus", Earlier: []agio.Session{{Tool: agent.Claude, Cursor: "c1"}}})
	if err != nil {
		t.Fatal(err)
	}
	h.Send(id, agio.Message{Text: "go on"})
	waitFor(t, h, func(s agio.State) bool { c := s.Conversation(id); return !c.Running && len(c.Turns) == 2 })
	if prompts := codex.Prompted(); len(prompts) != 1 || !strings.Contains(prompts[0], "first reply") ||
		!strings.HasSuffix(prompts[0], "go on") {
		t.Fatalf("codex prompts = %q", prompts)
	}
}

func TestHandoffKeepsTheLatestTurns(t *testing.T) {
	var turns []*transcript.Turn
	for range 3 {
		turns = append(turns, &transcript.Turn{Prompt: strings.Repeat("x", maxHandoff/2), Status: transcript.Completed, Started: time.Now()})
	}
	turns[2].Prompt = "latest"
	text := handoff(turns, "now")
	if n := strings.Count(text, "## User"); n != 2 || !strings.Contains(text, "latest") {
		t.Fatalf("handed %d turns", n)
	}
	if got := stripHandoff(text); got != "now" {
		t.Fatalf("stripped = %q", got)
	}
	if got := handoff(nil, "now"); got != "now" {
		t.Fatalf("nothing to hand = %q", got)
	}
}
