package main

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/transcript"
)

func TestActivityCountsTurnsStartedSince(t *testing.T) {
	since := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	turn := func(d time.Duration) *transcript.Turn { return &transcript.Turn{Started: since.Add(d)} }
	s := agio.State{Conversations: []*agio.Conversation{
		{Model: agent.Key(agent.Codex, "gpt"), Turns: []*transcript.Turn{turn(-time.Minute), turn(time.Second), turn(2 * time.Second)}},
		{Model: agent.Key(agent.Claude, "opus"), Turns: []*transcript.Turn{turn(-time.Hour)}},
	}}
	tools, working := activity(s, since)
	if want := []agent.Tool{agent.Codex, agent.Codex}; !slices.Equal(tools, want) || working {
		t.Fatalf("activity = %v, %v; want %v, false", tools, working, want)
	}
	s.Conversations[1].Running = true
	if tools, working = activity(s, since.Add(time.Hour)); len(tools) != 0 || !working {
		t.Fatalf("activity an hour on = %v, %v; want none, working", tools, working)
	}
}

func TestStatsAgentNamesThePlatform(t *testing.T) {
	if got := statsAgent(core.OSMacOS); got != "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Agio/1.0" {
		t.Fatalf("statsAgent(macos) = %q", got)
	}
}
