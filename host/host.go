// Package host runs conversations with coding agents in this process. It
// finds the agent CLIs on the login PATH, starts a conversation's agent with
// its first message and folds what the agent reports into turns. A Host is
// an agio.Backend: show it directly in a single-process application, or
// serve it to other processes with package wire.
package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/quota"
)

// publishInterval coalesces change reports, so a streamed reply reaches
// watchers about 30 times a second.
const publishInterval = 33 * time.Millisecond

// closeGrace is how long a closing agent may take before its process group
// is terminated.
const closeGrace = 3 * time.Second

// Options configure a Host.
type Options struct {
	// Env is the agents' environment; nil reads the login shell's.
	Env []string
	// Tools are the CLIs looked for, in the order models list; nil is
	// agent.Tools.
	Tools []agent.Tool
	// Disabled are tools turned off from the start, DisabledModels model
	// keys (agent.Key).
	Disabled       []agent.Tool
	DisabledModels []string
	// ReadQuota reads an account's limits, such as quota.Reader; nil reads
	// none.
	ReadQuota func(ctx context.Context, env []string, p quota.Provider) (quota.Usage, error)
	// Drivers replace looking for a tool's CLI, such as for tests or a
	// driver of the application's own.
	Drivers map[agent.Tool]agent.Driver
	// Dir is where a conversation created without a directory works;
	// empty refuses such conversations.
	Dir string
	// Home is where catalogs are read and paths shortened to ~; empty is
	// the user's home directory.
	Home string
	// Now is the clock, time.Now when nil.
	Now func() time.Time
}

// Host runs conversations in this process. All its state is guarded by mu;
// what agents report is queued and applied in order by one goroutine, so
// their callbacks never wait for the host.
type Host struct {
	opts Options

	mu          sync.Mutex
	discovering int
	tools       map[agent.Tool]*toolInfo
	disabled    map[agent.Tool]bool
	models      map[string]bool // turned off
	// env is the login environment once read.
	env           []string
	quotas        map[quota.Provider]*quotaState
	conversations []*conversation
	closed        bool

	queueMu sync.Mutex
	queue   []func()
	wake    chan struct{}

	watchMu   sync.Mutex
	watchers  map[int]func()
	nextWatch int
	kick      chan struct{}
	done      chan struct{}
}

// New starts a host and looks for its agent CLIs in the background.
func New(o Options) *Host {
	if o.Tools == nil {
		o.Tools = agent.Tools
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	h := &Host{opts: o, tools: map[agent.Tool]*toolInfo{}, disabled: map[agent.Tool]bool{},
		models: map[string]bool{}, quotas: map[quota.Provider]*quotaState{}, wake: make(chan struct{}, 1),
		watchers: map[int]func(){}, kick: make(chan struct{}, 1), done: make(chan struct{})}
	for _, tool := range o.Disabled {
		h.disabled[tool] = true
	}
	for _, key := range o.DisabledModels {
		h.models[key] = true
	}
	go h.loop()
	go h.publish()
	h.Discover()
	h.watchQuotas()
	return h
}

// post queues fn to run under the lock after what is already queued. It
// never blocks, so agents' callbacks may call it from anywhere.
func (h *Host) post(fn func()) {
	h.queueMu.Lock()
	h.queue = append(h.queue, fn)
	h.queueMu.Unlock()
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *Host) loop() {
	for {
		select {
		case <-h.wake:
		case <-h.done:
			return
		}
		for {
			h.queueMu.Lock()
			batch := h.queue
			h.queue = nil
			h.queueMu.Unlock()
			if len(batch) == 0 {
				break
			}
			h.mu.Lock()
			for _, fn := range batch {
				fn()
			}
			h.mu.Unlock()
			h.changed()
		}
	}
}

// do runs fn under the lock and reports the change.
func (h *Host) do(fn func() error) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return errClosed
	}
	err := fn()
	h.mu.Unlock()
	h.changed()
	return err
}

// changed tells watchers, coalescing reports close together.
func (h *Host) changed() {
	select {
	case h.kick <- struct{}{}:
	default:
	}
}

func (h *Host) publish() {
	for {
		select {
		case <-h.kick:
		case <-h.done:
			return
		}
		h.watchMu.Lock()
		fns := make([]func(), 0, len(h.watchers))
		for _, fn := range h.watchers {
			fns = append(fns, fn)
		}
		h.watchMu.Unlock()
		for _, fn := range fns {
			fn()
		}
		select {
		case <-time.After(publishInterval):
		case <-h.done:
			return
		}
	}
}

// Watch implements agio.Backend.
func (h *Host) Watch(fn func()) (stop func()) {
	h.watchMu.Lock()
	id := h.nextWatch
	h.nextWatch++
	h.watchers[id] = fn
	h.watchMu.Unlock()
	return func() {
		h.watchMu.Lock()
		delete(h.watchers, id)
		h.watchMu.Unlock()
	}
}

// State implements agio.Backend.
func (h *Host) State() agio.State {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := agio.State{Discovering: h.discovering > 0}
	for _, tool := range h.opts.Tools {
		t := agio.Tool{Tool: tool}
		if info := h.tools[tool]; info != nil {
			t = info.state(tool)
		}
		t.Disabled = h.disabled[tool]
		for key := range h.models {
			if kt, id := agent.ParseKey(key); kt == tool {
				t.DisabledModels = append(t.DisabledModels, id)
			}
		}
		slices.Sort(t.DisabledModels)
		s.Tools = append(s.Tools, t)
	}
	s.Quotas = h.quotaStates()
	for _, c := range h.conversations {
		s.Conversations = append(s.Conversations, h.snapshot(c))
	}
	return s
}

// Close implements agio.Backend: it closes every agent, waiting for them to
// exit or be terminated.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	var sessions []agent.Session
	for _, c := range h.conversations {
		if c.session != nil {
			sessions = append(sessions, c.session)
		}
	}
	h.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range sessions {
		wg.Go(func() { s.Close(closeGrace) })
	}
	wg.Wait()
	close(h.done)
	return nil
}

var (
	errClosed         = errors.New("the host is closed")
	errNoConversation = errors.New("no such conversation")
	errEmpty          = errors.New("empty message")
)

// Create implements agio.Backend.
func (h *Host) Create(o agio.Create) (string, error) {
	var id string
	err := h.do(func() error {
		if o.Dir == "" {
			o.Dir = h.opts.Dir
		}
		if o.Dir == "" {
			return errors.New("a conversation needs a directory")
		}
		if info, err := os.Stat(o.Dir); err != nil || !info.IsDir() {
			return errors.New("not a directory: " + o.Dir)
		}
		if o.Cursor != "" && o.Model == "" {
			return errors.New("resuming a session needs its model")
		}
		now := h.opts.Now()
		created, updated := o.Created, o.Updated
		if created.IsZero() {
			created = now
		}
		if updated.IsZero() {
			updated = created
		}
		c := &conversation{id: newID(), title: o.Title, dir: o.Dir, model: o.Model, effort: o.Effort, tier: o.Tier,
			cursor: o.Cursor, bound: o.Cursor != "", created: created, updated: updated, loaded: o.Cursor == ""}
		h.conversations = append([]*conversation{c}, h.conversations...)
		h.loadHistory(c)
		id = c.id
		return nil
	})
	return id, err
}

// Remove implements agio.Backend.
func (h *Host) Remove(id string) error {
	var session agent.Session
	err := h.do(func() error {
		i := slices.IndexFunc(h.conversations, func(c *conversation) bool { return c.id == id })
		if i < 0 {
			return errNoConversation
		}
		c := h.conversations[i]
		h.conversations = slices.Delete(h.conversations, i, i+1)
		session, c.session, c.removed = c.session, nil, true
		return nil
	})
	if session != nil {
		go session.Close(closeGrace)
	}
	return err
}

func (h *Host) find(id string) *conversation {
	for _, c := range h.conversations {
		if c.id == id {
			return c
		}
	}
	return nil
}

func newID() string {
	var b [12]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// titleFromPrompt uses the first nonempty line, shortened for a list.
func titleFromPrompt(prompt string) string {
	for line := range strings.Lines(prompt) {
		if line = strings.TrimSpace(line); line != "" {
			r := []rune(line)
			if len(r) > 60 {
				return strings.TrimSpace(string(r[:60])) + "…"
			}
			return line
		}
	}
	return ""
}

// background runs fn off the lock with a fresh context.
func background(fn func(ctx context.Context)) { go fn(context.Background()) }
