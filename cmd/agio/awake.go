package main

import "sync"

// awake holds a system idle-sleep prevention while the settings ask for it
// and an agent works, and releases it otherwise. Unless they also keep the
// display on, it may still turn off and lock the screen; closing a laptop's
// lid still sleeps it.
type awake struct {
	mu      sync.Mutex
	working bool
	display bool
	release func()
	// hold is keepAwake but in tests.
	hold func(reason string, display bool) func()
}

// update follows the settings and whether an agent works.
func (w *awake) update(s settings, working bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.working = working
	w.apply(s)
}

// follow applies changed settings to the work last seen.
func (w *awake) follow(s settings) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.apply(s)
}

func (w *awake) apply(s settings) {
	want := (s.KeepAwake || s.KeepDisplayAwake) && w.working
	display := want && s.KeepDisplayAwake
	if w.release != nil && (!want || display != w.display) {
		w.release()
		w.release = nil
	}
	if want && w.release == nil {
		hold := w.hold
		if hold == nil {
			hold = keepAwake
		}
		w.release = hold("Agio agents are working", display)
		w.display = display
	}
}

// stop releases the hold.
func (w *awake) stop() { w.update(settings{}, false) }
