package transcript

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

func TestFromHistoryGroupsTurnsByUserMessage(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	turns := FromHistory([]agent.Event{
		{Type: agent.UserMessage, Text: "first", Time: at},
		{Type: agent.TextDone, Segment: "a", Text: "done", Time: at.Add(time.Minute)},
		{Type: agent.UserMessage, Text: "second", Time: at.Add(2 * time.Minute)},
		{Type: agent.ToolStart, Segment: "t", Tool: "bash", Preview: "ls"},
		{Type: agent.ToolEnd, Segment: "t", Tool: "bash", Failed: true},
	})
	if len(turns) != 2 || turns[0].Prompt != "first" || turns[1].Prompt != "second" {
		t.Fatalf("turns = %+v", turns)
	}
	if turns[0].Worked() != time.Minute || turns[0].Status != Completed {
		t.Fatalf("first turn = %+v", turns[0])
	}
	if s := turns[1].Segments[0]; s.Tool != "bash" || s.Status != Failed {
		t.Fatalf("tool segment = %+v", s)
	}
}

func TestReportedReadsOnlyTheFinalReply(t *testing.T) {
	turn := New("t", "finish", nil, time.Now())
	turn.Apply(agent.Event{Type: agent.ToolEnd, Segment: "x", Tool: "bash", Text: readyMarker})
	if turn.Reported(readyMarker) {
		t.Fatal("tool output counted as readiness")
	}
	turn.Apply(agent.Event{Type: agent.TextDone, Segment: "y", Text: "All set.\n\n**" + readyMarker + "**\nSummary follows."})
	if !turn.Reported(readyMarker) {
		t.Fatal("final reply readiness not read")
	}
}

func TestBoundOutputKeepsTheEnd(t *testing.T) {
	long := strings.Repeat("é", MaxToolOutput)
	got := BoundOutput(long)
	if !strings.HasPrefix(got, "…\n") || len(got) > MaxToolOutput+len("…\n") || !strings.HasSuffix(got, "é") {
		t.Fatalf("bounded output %q…", got[:10])
	}
}

// roundTrip sends p as JSON, as the protocol does.
func roundTrip(t *testing.T, p Patch) Patch {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got Patch
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestDiffStreamsAppendedTextAndRebuildsTheSender(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cur := []*Turn{New("1", "hello", []string{"/tmp/a.png"}, now)}
	var sent, received []*Turn
	sync := func() Patch {
		t.Helper()
		p, changed := Diff(sent, cur)
		if !changed {
			return Patch{}
		}
		received = Apply(received, roundTrip(t, p))
		sent = Clone(cur)
		if !reflect.DeepEqual(received, cur) {
			t.Fatalf("received %+v, want %+v", received, cur)
		}
		return p
	}
	sync()
	cur[0].Apply(agent.Event{Type: agent.TextDelta, Segment: "s", Text: "Hel"})
	sync()
	cur[0].Apply(agent.Event{Type: agent.TextDelta, Segment: "s", Text: "lo there"})
	p := sync()
	if len(p.Turns) != 1 || p.Turns[0].Header != nil || len(p.Turns[0].Segments) != 1 ||
		!p.Turns[0].Segments[0].Append || p.Turns[0].Segments[0].Text != "lo there" {
		t.Fatalf("streaming patch = %+v", p)
	}
	if _, changed := Diff(sent, cur); changed {
		t.Fatal("unchanged turns produced a patch")
	}
	cur[0].Apply(agent.Event{Type: agent.TextDone, Segment: "s", Text: "Hello there"})
	cur[0].Finish(Completed, now.Add(time.Second))
	cur = append(cur, New("2", "next", nil, now.Add(2*time.Second)))
	sync()
	// A saved history arriving before live turns resets the receiver.
	cur = append(FromHistory([]agent.Event{{Type: agent.UserMessage, Text: "old", Time: now.Add(-time.Hour)}}), cur...)
	if p := sync(); !p.Reset {
		t.Fatalf("history patch = %+v, want reset", p)
	}
}

const readyMarker = "AGIO_READY"
