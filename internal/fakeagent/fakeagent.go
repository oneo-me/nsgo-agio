// Package fakeagent is a scripted agent.Driver for tests: it answers each
// prompt with its text echoed back, unless the prompt is "wait", which runs
// a tool until aborted.
package fakeagent

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// Driver is a fake Claude Code with the models opus and haiku.
type Driver struct {
	// Saved is what History returns.
	Saved []agent.Event

	mu      sync.Mutex
	started []agent.StartOptions
	config  []agent.Settings
}

// Started lists the options of each session started.
func (d *Driver) Started() []agent.StartOptions {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]agent.StartOptions(nil), d.started...)
}

// Configured lists the settings applied to live sessions.
func (d *Driver) Configured() []agent.Settings {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]agent.Settings(nil), d.config...)
}

func (d *Driver) Tool() agent.Tool { return agent.Claude }

func (d *Driver) Catalog(context.Context, string) (agent.Catalog, error) {
	return agent.Catalog{Model: "opus", Effort: "high", Models: []agent.Model{
		{Tool: agent.Claude, ID: "opus", Name: "Opus", Efforts: []string{"low", "high"}, ContextWindow: 1000},
		{Tool: agent.Claude, ID: "haiku", Name: "Haiku"},
	}}, nil
}

func (d *Driver) Start(o agent.StartOptions, onEvent func(agent.Event), onExit func(error)) (agent.Session, agent.Info, error) {
	d.mu.Lock()
	d.started = append(d.started, o)
	d.mu.Unlock()
	return &session{d: d, emit: onEvent, exit: onExit}, agent.Info{Cursor: "session-1", Settings: o.Settings}, nil
}

func (d *Driver) History(cursor, dir string) ([]agent.Event, error) { return d.Saved, nil }

type session struct {
	d    *Driver
	mu   sync.Mutex
	emit func(agent.Event)
	exit func(error)
	run  int
	busy bool
}

func (s *session) Prompt(_ context.Context, p agent.Prompt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run++
	seg := "t" + strconv.Itoa(s.run)
	s.emit(agent.Event{Type: agent.Started})
	if p.Text == "wait" {
		s.busy = true
		s.emit(agent.Event{Type: agent.ToolStart, Segment: seg, Tool: "bash", Preview: "sleep"})
		return nil
	}
	s.emit(agent.Event{Type: agent.TextDelta, Segment: seg, Text: "echo: "})
	s.emit(agent.Event{Type: agent.TextDone, Segment: seg, Text: "echo: " + p.Text})
	s.emit(agent.Event{Type: agent.Usage, Tokens: 42})
	s.emit(agent.Event{Type: agent.Settled})
	return nil
}

func (s *session) Abort(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		s.busy = false
		s.emit(agent.Event{Type: agent.Settled, Aborted: true})
	}
	return nil
}

func (s *session) Configure(_ context.Context, settings agent.Settings) error {
	s.d.mu.Lock()
	s.d.config = append(s.d.config, settings)
	s.d.mu.Unlock()
	return nil
}

func (s *session) Close(time.Duration) { s.exit(nil) }
