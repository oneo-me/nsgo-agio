package main

import (
	"testing"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/transcript"
)

func TestAlertsWhenAnAgentStops(t *testing.T) {
	type sound struct {
		id     string
		failed bool
	}
	var sounds []sound
	var notes []taskNotification
	al := &alerts{play: func(id string, failed bool) { sounds = append(sounds, sound{id, failed}) },
		notify: func(n taskNotification) { notes = append(notes, n) }}
	s := settings{TaskSound: "chime", TaskNotification: true}
	turn := func(status transcript.Status) []*transcript.Turn { return []*transcript.Turn{{Status: status}} }

	al.look(s, []*agio.Conversation{{ID: "a", Running: true}, {ID: "b", Running: true}, {ID: "c", Running: true}}, "en")
	al.look(s, []*agio.Conversation{
		{ID: "a", Title: "Fix", Turns: turn(transcript.Completed)},
		{ID: "b", Turns: turn(transcript.Failed)},
		// One the user stopped raises no alert.
		{ID: "c", Turns: turn(transcript.Cancelled)},
	}, "en")
	if len(sounds) != 2 || sounds[0] != (sound{"chime", false}) || sounds[1] != (sound{"chime", true}) {
		t.Fatalf("sounds %v", sounds)
	}
	if len(notes) != 2 || notes[0] != (taskNotification{"Fix", "The agent completed", true}) ||
		notes[1] != (taskNotification{"Untitled task", "The agent failed", true}) {
		t.Fatalf("notifications %v", notes)
	}
	// Agents that stay idle alert once.
	al.look(s, []*agio.Conversation{{ID: "a"}}, "en")
	if len(sounds) != 2 {
		t.Fatalf("alerted again: %v", sounds)
	}
}
