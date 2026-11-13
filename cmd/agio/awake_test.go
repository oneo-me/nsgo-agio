package main

import "testing"

func TestAwakeFollowsSettingsAndWork(t *testing.T) {
	var held []bool
	released := 0
	w := &awake{hold: func(_ string, display bool) func() {
		held = append(held, display)
		return func() { released++ }
	}}
	w.update(settings{KeepAwake: true}, false)
	if len(held) != 0 {
		t.Fatalf("held %v without work", held)
	}
	w.update(settings{KeepAwake: true}, true)
	w.update(settings{KeepAwake: true}, true)
	if len(held) != 1 || held[0] {
		t.Fatalf("held %v, want one system-only hold", held)
	}
	// Asking to keep the display on replaces the hold, even without KeepAwake.
	w.follow(settings{KeepDisplayAwake: true})
	if len(held) != 2 || !held[1] || released != 1 {
		t.Fatalf("held %v released %d, want a display hold replacing the first", held, released)
	}
	w.update(settings{KeepDisplayAwake: true}, false)
	if released != 2 || w.release != nil {
		t.Fatalf("released %d after work ended, want 2", released)
	}
}
