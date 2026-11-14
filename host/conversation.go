package host

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// conversation is the live state of one conversation, guarded by the
// host's lock.
type conversation struct {
	id, title, dir string
	// system is appended to its agent's system prompt.
	system string
	// model is a model key (agent.Key), empty to follow the default until
	// the first message binds it; effort and tier are its choices.
	model, effort, tier string
	cursor              string
	bound               bool
	created, updated    time.Time
	err                 string

	turns   []*transcript.Turn
	current *transcript.Turn
	loaded  bool
	loading bool

	session  agent.Session
	starting bool
	running  bool
	removed  bool
	// waiting holds prompts accepted before the agent was ready.
	waiting []func()
	// interrupting counts aborts awaiting their replacement prompt.
	interrupting int

	// wantSettings are the settings the live agent should have,
	// sentSettings those it was given; configuring is set while they are
	// applied.
	wantSettings, sentSettings agent.Settings
	configuring                bool

	contextTokens, contextWindow int64
}

// snapshot copies c for State.
func (h *Host) snapshot(c *conversation) *agio.Conversation {
	model, effort, tier := h.effective(c)
	window := c.contextWindow
	if m, ok := h.model(model); ok && window == 0 {
		window = m.ContextWindow
	}
	return &agio.Conversation{ID: c.id, Title: c.title, Dir: c.dir, Model: model, Effort: effort, Tier: tier,
		Cursor: c.cursor, Bound: c.bound, Starting: c.starting, Running: c.running, Loading: c.loading, Err: c.err,
		ContextTokens: c.contextTokens, ContextWindow: window, Created: c.created, Updated: c.updated,
		Turns: transcript.Clone(c.turns)}
}

// effective resolves the conversation's model, reasoning level and tier:
// its own choices where its model offers them, else the tools' defaults.
// A bound conversation keeps its model even while its tool is unavailable.
func (h *Host) effective(c *conversation) (model, effort, tier string) {
	model, effort, tier = c.model, c.effort, c.tier
	m, ok := h.model(model)
	if ok && !c.bound && h.models[model] {
		ok = false // turned off before it was bound
	}
	if !ok {
		if c.bound {
			return model, effort, tier
		}
		var fallback string
		model, fallback = h.defaultModel()
		if m, ok = h.model(model); !ok {
			return "", "", ""
		}
		if !m.HasEffort(effort) {
			effort = fallback
		}
	}
	if len(m.Efforts) == 0 {
		effort = ""
	} else if !m.HasEffort(effort) {
		tool, _ := agent.ParseKey(model)
		effort = m.DefaultEffort
		if t := h.tools[tool]; t != nil && m.HasEffort(t.catalog.Effort) {
			effort = t.catalog.Effort
		}
	}
	if !m.HasTier(tier) {
		tier = ""
	}
	return model, effort, tier
}

// settings are the tool-level settings of the conversation's choices.
func (h *Host) settings(c *conversation) agent.Settings {
	model, effort, tier := h.effective(c)
	_, id := agent.ParseKey(model)
	return agent.Settings{Model: id, Effort: effort, Tier: tier}
}

// loadHistory reads a resumed conversation's saved turns off the lock once
// its tool can read them.
func (h *Host) loadHistory(c *conversation) {
	if c.loaded || c.loading || c.cursor == "" {
		return
	}
	tool, _ := agent.ParseKey(c.model)
	d := h.historyDriver(tool)
	if d == nil {
		return // retried when the tool is discovered
	}
	c.loading = true
	cursor, dir := c.cursor, c.dir
	go func() {
		events, err := d.History(cursor, dir)
		h.post(func() {
			c.loading, c.loaded = false, true
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				c.err = err.Error()
				return
			}
			// Live turns started before loading finished stay after history.
			c.turns = append(transcript.FromHistory(events), c.turns...)
			for i := len(events) - 1; i >= 0; i-- {
				if e := events[i]; e.Type == agent.Usage && e.Tokens > 0 {
					c.contextTokens = e.Tokens
					break
				}
			}
		})
	}()
}

// Send implements agio.Backend.
func (h *Host) Send(id string, m agio.Message) error {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return errEmpty
	}
	return h.do(func() error {
		c := h.find(id)
		if c == nil {
			return errNoConversation
		}
		if !c.bound {
			model, effort, tier := h.effective(c)
			if model == "" {
				return errors.New("no coding agent is available")
			}
			c.model, c.effort, c.tier, c.bound = model, effort, tier, true
		}
		now := h.opts.Now()
		if c.title == "" {
			c.title = titleFromPrompt(text)
		}
		c.updated, c.err = now, ""
		h.toFront(c)
		t := transcript.New(newID(), text, m.Files, now)
		c.turns = append(c.turns, t)
		if c.running && c.session != nil {
			// A new message always interrupts the run instead of queueing.
			h.interrupt(c, t, text, m.Files)
			return nil
		}
		if prev := c.current; prev != nil {
			// The agent is still starting: the earlier prompt never reached it.
			prev.Finish(transcript.Cancelled, now)
			c.waiting = nil
		}
		c.current, c.running = t, true
		run := func() { h.prompt(c, text, m.Files) }
		if c.session != nil {
			run()
			return nil
		}
		c.waiting = append(c.waiting, run)
		h.start(c)
		return nil
	})
}

// toFront keeps conversations newest first.
func (h *Host) toFront(c *conversation) {
	for i, x := range h.conversations {
		if x == c {
			copy(h.conversations[1:i+1], h.conversations[:i])
			h.conversations[0] = c
			return
		}
	}
}

// start launches the conversation's tool in its directory.
func (h *Host) start(c *conversation) {
	if c.starting || c.session != nil {
		return
	}
	tool, _ := agent.ParseKey(c.model)
	d := h.driver(tool)
	if d == nil && h.disabled[tool] {
		h.failRun(c, tool.Name()+" is turned off. Turn it on to continue this conversation.")
		return
	}
	if d == nil {
		h.failRun(c, tool.Name()+" is not available. Install it, then look for agents again.")
		return
	}
	c.starting = true
	options := agent.StartOptions{Dir: c.dir, Cursor: c.cursor, Title: c.title, SystemPrompt: c.system, Settings: h.settings(c)}
	go func() {
		// Drivers may report events before Start returns, even from inside
		// it, so callbacks must not block on it. They are held until the
		// session they belong to is known, then follow the start result.
		var (
			mu      sync.Mutex
			session agent.Session
			started bool
			held    []func(agent.Session)
		)
		deliver := func(fn func(agent.Session)) {
			mu.Lock()
			defer mu.Unlock()
			if !started {
				held = append(held, fn)
				return
			}
			s := session
			h.post(func() { fn(s) })
		}
		session, info, err := d.Start(options,
			func(e agent.Event) { deliver(func(s agent.Session) { h.applyEvent(c, s, e) }) },
			func(err error) { deliver(func(s agent.Session) { h.agentExited(c, s, err) }) })
		mu.Lock()
		started = true
		h.post(func() {
			c.starting = false
			if err != nil {
				h.failRun(c, "Could not start the agent: "+err.Error())
				return
			}
			if c.removed || h.closed {
				go session.Close(closeGrace)
				return
			}
			c.session, c.sentSettings = session, options.Settings
			if info.Cursor != "" {
				c.cursor = info.Cursor
			}
			if info.Settings.Model != "" {
				c.model = agent.Key(tool, info.Settings.Model)
			}
			if info.Settings.Effort != "" {
				c.effort = info.Settings.Effort
			}
			if info.ContextWindow > 0 {
				c.contextWindow = info.ContextWindow
			}
			if info.ContextTokens > 0 {
				c.contextTokens = info.ContextTokens
			}
			waiting := c.waiting
			c.waiting = nil
			for _, fn := range waiting {
				fn()
			}
		})
		for _, fn := range held {
			s := session
			h.post(func() { fn(s) })
		}
		held = nil
		mu.Unlock()
	}()
}

// interrupt aborts the running agent, then sends the next prompt. Events of
// the aborted run settle the previous turn before the new one becomes
// current.
func (h *Host) interrupt(c *conversation, next *transcript.Turn, text string, files []string) {
	if t := c.current; t != nil {
		t.Finish(transcript.Cancelled, h.opts.Now())
	}
	c.current = nil
	c.interrupting++
	s := c.session
	background(func(ctx context.Context) {
		err := s.Abort(ctx)
		h.post(func() {
			c.interrupting--
			if err != nil {
				c.current = next
				h.failRun(c, err.Error())
				return
			}
			next.Replacing = true
			c.current, c.running = next, true
			h.prompt(c, text, files)
		})
	})
}

// prompt sends text, images and attached paths; images go inline only to
// models that accept them.
func (h *Host) prompt(c *conversation, text string, files []string) {
	s := c.session
	if s == nil {
		h.failRun(c, "The agent exited unexpectedly")
		return
	}
	model, _ := h.model(c.model)
	background(func(ctx context.Context) {
		p := agent.Prompt{Text: text}
		for _, path := range files {
			if image := inlineImage(path); image != nil && model.Images {
				p.Images = append(p.Images, *image)
			} else {
				p.Paths = append(p.Paths, path)
			}
		}
		if err := s.Prompt(ctx, p); err != nil {
			h.post(func() { h.failRun(c, err.Error()) })
		}
	})
}

// Stop implements agio.Backend.
func (h *Host) Stop(id string) error {
	return h.do(func() error {
		c := h.find(id)
		if c == nil {
			return errNoConversation
		}
		switch {
		case c.session != nil && c.running:
			s := c.session
			background(func(ctx context.Context) { s.Abort(ctx) })
		case c.running:
			// The agent is still starting: its prompt never reached it.
			c.waiting = nil
			c.running = false
			if t := c.current; t != nil {
				t.Finish(transcript.Cancelled, h.opts.Now())
				c.current = nil
			}
		}
		return nil
	})
}

// failRun ends the current turn with an error and keeps the conversation
// usable.
func (h *Host) failRun(c *conversation, message string) {
	c.waiting = nil
	c.running = false
	if t := c.current; t != nil {
		t.Err = message
		t.Finish(transcript.Failed, h.opts.Now())
		c.current = nil
	} else {
		c.err = message
	}
}

func (h *Host) agentExited(c *conversation, s agent.Session, err error) {
	if c.session != s && s != nil {
		return
	}
	c.session = nil
	if h.closed {
		return
	}
	if c.running {
		message := "The agent exited unexpectedly"
		if err != nil {
			message += ": " + err.Error()
		}
		h.failRun(c, message)
	}
}

// applyEvent folds one agent event into the conversation.
func (h *Host) applyEvent(c *conversation, s agent.Session, e agent.Event) {
	if c.session != nil && c.session != s {
		return
	}
	now := h.opts.Now()
	t := c.current
	if t != nil && t.Replacing {
		switch e.Type {
		case agent.Started:
			t.Replacing = false
		case agent.Usage, agent.Title:
		default:
			// A late event of the aborted run, such as a nested tool call
			// ending, must neither join nor duplicate the new turn.
			return
		}
	}
	switch e.Type {
	case agent.Started:
		c.running = true
	case agent.UserMessage:
		if t != nil && len(t.Segments) == 0 {
			return // the optimistic turn for this prompt
		}
		// A message the agent started on its own, such as an extension
		// follow-up, starts a new visible turn.
		if t != nil {
			t.Finish(transcript.Completed, now)
		}
		t = transcript.New(newID(), e.Text, nil, now)
		c.turns = append(c.turns, t)
		c.current, c.running = t, true
	case agent.Usage:
		if e.Tokens > 0 {
			c.contextTokens = e.Tokens
		}
		if e.Window > 0 {
			c.contextWindow = e.Window
		}
	case agent.Title:
		if e.Text != "" {
			c.title = e.Text
		}
	case agent.Settled:
		if c.interrupting > 0 {
			// The aborted run settled; the replacement prompt follows.
			return
		}
		c.running = false
		if t != nil {
			status := transcript.Completed
			if e.Aborted {
				status = transcript.Cancelled
			}
			t.Finish(status, now)
		}
		c.current = nil
		c.updated = now
	default:
		if t != nil {
			t.Apply(e)
		}
	}
}

// Choose implements agio.Backend.
func (h *Host) Choose(id string, ch agio.Choice) error {
	return h.do(func() error {
		c := h.find(id)
		if c == nil {
			return errNoConversation
		}
		if ch.Title != nil {
			if title := strings.TrimSpace(*ch.Title); title != "" {
				c.title = title
			}
			if ch == (agio.Choice{Title: ch.Title}) {
				return nil
			}
		}
		if ch.Dir != nil {
			if c.bound || len(c.turns) > 0 {
				return errors.New("a conversation keeps the directory of its first message")
			}
			if info, err := os.Stat(*ch.Dir); err != nil || !info.IsDir() {
				return errors.New("not a directory: " + *ch.Dir)
			}
			c.dir = *ch.Dir
		}
		if ch.Model != nil {
			key := *ch.Model
			if !h.offered(key) {
				return errors.New("no such model: " + key)
			}
			tool, _ := agent.ParseKey(c.model)
			if next, _ := agent.ParseKey(key); c.bound && next != tool {
				return errors.New("a conversation keeps the agent of its first message")
			}
			c.model = key
			c.contextWindow = 0
		}
		if ch.Effort != nil {
			c.effort = *ch.Effort
		}
		if ch.Tier != nil {
			c.tier = *ch.Tier
		}
		// Choices the new model lacks fall back to its defaults.
		_, c.effort, c.tier = h.effective(c)
		h.configure(c)
		return nil
	})
}

// configure applies the conversation's settings to its live agent. One
// goroutine at a time applies them, the latest first, so quick changes
// never land out of order.
func (h *Host) configure(c *conversation) {
	s := c.session
	if s == nil {
		return
	}
	c.wantSettings = h.settings(c)
	if c.configuring {
		return
	}
	c.configuring = true
	go func() {
		for {
			h.mu.Lock()
			want := c.wantSettings
			done := want == c.sentSettings || c.session != s
			if done {
				c.configuring = false
			}
			h.mu.Unlock()
			if done {
				return
			}
			err := s.Configure(context.Background(), want)
			// The next round reads what was sent, so it waits for this.
			applied := make(chan struct{})
			h.post(func() {
				c.sentSettings = want
				if err != nil {
					c.err = err.Error()
				}
				close(applied)
			})
			<-applied
		}
	}()
}

// maxInlineImage is the largest image sent inline rather than by path.
const maxInlineImage = 20 << 20

// inlineImage reads an attached image for models that accept images, or
// returns nil for anything else.
func inlineImage(path string) *agent.Image {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxInlineImage {
		return nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil
	}
	return &agent.Image{Path: path, Data: base64.StdEncoding.EncodeToString(data), MimeType: mime}
}
