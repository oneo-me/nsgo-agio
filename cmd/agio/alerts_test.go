package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
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
	if len(notes) != 2 || notes[0] != (taskNotification{"a", "Fix", "The agent completed", true}) ||
		notes[1] != (taskNotification{"b", "Untitled task", "The agent failed", true}) {
		t.Fatalf("notifications %v", notes)
	}
	// Agents that stay idle alert once.
	al.look(s, []*agio.Conversation{{ID: "a"}}, "en")
	if len(sounds) != 2 {
		t.Fatalf("alerted again: %v", sounds)
	}
}

func TestNotificationsShowAsTheAppAndOpenTheirTask(t *testing.T) {
	var selected *core.Signal[string]
	al := &alerts{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		selected = core.NewSignal(cx, "")
		al.attach(cx, selected, func(id string) bool { return id == "a" })
		return core.Text{Content: "agio"}
	})
	showTaskNotification(al.scope.Load(), taskNotification{ID: "a", Title: "Fix", Body: "The agent completed", Silent: true})
	app.WaitUntil(5*time.Second, func() bool { return len(app.Notifications) == 1 })
	if n := app.Notifications[0]; n != (core.Notification{ID: "a", Title: "Fix", Body: "The agent completed", Silent: true}) {
		t.Fatalf("notification %+v", n)
	}
	app.PressNotification("a")
	app.WaitUntil(5*time.Second, func() bool { return selected.GetUntracked() == "a" })
}
