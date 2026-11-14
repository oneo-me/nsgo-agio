package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// abortTimeout bounds the wait for a cancelled prompt's response.
const abortTimeout = 30 * time.Second

// Start opens a new session in o.Dir, or reopens o.Cursor: resumed when the
// agent supports it, else loaded with its replayed history discarded.
func (d *Driver) Start(o agent.StartOptions, onEvent func(agent.Event), onExit func(error)) (agent.Session, agent.Info, error) {
	s := &session{emitter: agent.NewEmitter(onEvent), loading: o.Cursor != ""}
	exited := make(chan error, 1)
	var args []string
	if o.SystemPrompt != "" && d.AppendSystemPrompt != "" {
		args = []string{d.AppendSystemPrompt, o.SystemPrompt}
	}
	plugin := ""
	if len(o.Skills) > 0 && d.PluginDir != "" {
		dir, err := agent.PluginDir(o.Skills)
		if err != nil {
			return nil, agent.Info{}, err
		}
		// Joined, as such options may take several values.
		plugin, args = dir, append(args, d.PluginDir+"="+dir)
	}
	c, err := dial(d.command(o.Dir, args...), d.name(), func(c *conn) {
		c.onUpdate, c.onRequest = s.update, s.request
	}, func(err error) {
		s.exited()
		os.RemoveAll(plugin)
		exited <- err
	})
	if err != nil {
		os.RemoveAll(plugin)
		return nil, agent.Info{}, err
	}
	s.conn = c
	ctx := context.Background()
	if d.AppendSystemPrompt != "" {
		o.SystemPrompt = "" // given on the command line
	}
	if err := s.open(ctx, o, d.Modes); err != nil {
		go c.proc.Close(3 * time.Second)
		return nil, agent.Info{}, d.explain(err)
	}
	settings, err := s.state.configure(ctx, c, o.Settings)
	if err != nil {
		go c.proc.Close(3 * time.Second)
		return nil, agent.Info{}, err
	}
	s.emitter.Open()
	go func() {
		err := <-exited
		if onExit != nil {
			onExit(err)
		}
	}()
	return s, agent.Info{Cursor: s.id, Settings: settings}, nil
}

// open creates or reopens the session, then selects a mode that skips
// permission prompts.
func (s *session) open(ctx context.Context, o agent.StartOptions, modes []string) error {
	c := s.conn
	var st state
	var err error
	switch {
	case o.Cursor == "":
		err = c.callInto(ctx, "session/new", withSystemPrompt(newSession(o.Dir), o.SystemPrompt), &st)
	case c.caps.canResume():
		err = c.callInto(ctx, "session/resume", withSystemPrompt(map[string]any{"sessionId": o.Cursor, "cwd": o.Dir, "mcpServers": []any{}}, o.SystemPrompt), &st)
	case c.caps.LoadSession:
		err = c.callInto(ctx, "session/load", withSystemPrompt(map[string]any{"sessionId": o.Cursor, "cwd": o.Dir, "mcpServers": []any{}}, o.SystemPrompt), &st)
	default:
		err = errNoSession
	}
	s.mu.Lock()
	s.loading = false
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if st.SessionID == "" {
		st.SessionID = o.Cursor
	}
	if st.SessionID == "" {
		return errors.New("the agent opened no session")
	}
	if err := st.selectMode(ctx, c, modes); err != nil {
		return err
	}
	s.id, s.state = st.SessionID, st
	return nil
}

// session is one ACP conversation; each run is one session/prompt request.
type session struct {
	conn    *conn
	emitter *agent.Emitter
	id      string

	// config guards state across configuration requests, which the reader
	// answers, so it is never held with mu.
	config sync.Mutex
	state  state

	mu sync.Mutex
	// loading discards the history session/load replays.
	loading  bool
	running  bool
	aborting bool
	settled  chan struct{}
	// stream is only used on the reader goroutine.
	stream stream
}

func (s *session) exited() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.end()
}

// end closes the current run; the caller holds mu.
func (s *session) end() {
	if s.running {
		s.running, s.aborting = false, false
		close(s.settled)
	}
}

func (s *session) Prompt(ctx context.Context, p agent.Prompt) error {
	s.mu.Lock()
	if !s.running {
		s.running, s.settled = true, make(chan struct{})
	}
	settled := s.settled
	s.mu.Unlock()
	blocks := []map[string]any{{"type": "text", "text": p.Message()}}
	for _, img := range p.Images {
		blocks = append(blocks, map[string]any{"type": "image", "mimeType": img.MimeType, "data": img.Data})
	}
	s.emitter.Emit(agent.Event{Type: agent.Started})
	ch, _, err := s.conn.request("session/prompt", map[string]any{"sessionId": s.id, "prompt": blocks})
	if err != nil {
		s.mu.Lock()
		s.end()
		s.mu.Unlock()
		return err
	}
	go s.await(ch, settled)
	return nil
}

// await settles the run once its prompt request returns. The reader
// delivered the run's updates before the response.
func (s *session) await(ch <-chan result, settled chan struct{}) {
	r := <-ch
	if errors.Is(r.err, agent.ErrClosed) {
		return // the exit ends the run
	}
	var res struct {
		StopReason string `json:"stopReason"`
	}
	json.Unmarshal(r.data, &res)
	s.mu.Lock()
	if !s.running || s.settled != settled {
		s.mu.Unlock()
		return
	}
	aborted := s.aborting || res.StopReason == "cancelled"
	s.mu.Unlock()
	switch {
	case aborted:
	case r.err != nil:
		s.emitter.Emit(agent.Event{Type: agent.Error, Text: r.err.Error()})
	case res.StopReason == "max_tokens":
		s.emitter.Emit(agent.Event{Type: agent.Error, Text: "The response reached the maximum token limit"})
	case res.StopReason == "max_turn_requests":
		s.emitter.Emit(agent.Event{Type: agent.Error, Text: "The run reached the maximum number of model requests"})
	case res.StopReason == "refusal":
		s.emitter.Emit(agent.Event{Type: agent.Error, Text: "The agent refused to continue"})
	}
	s.emitter.Emit(agent.Event{Type: agent.Settled, Aborted: aborted})
	s.mu.Lock()
	if s.settled == settled {
		s.end()
	}
	s.mu.Unlock()
}

// Abort cancels the run and waits for its prompt request to return.
func (s *session) Abort(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.aborting = true
	settled := s.settled
	s.mu.Unlock()
	if err := s.conn.notify("session/cancel", map[string]any{"sessionId": s.id}); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, abortTimeout)
	defer cancel()
	select {
	case <-settled:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Configure applies a model and reasoning level to later runs.
func (s *session) Configure(ctx context.Context, settings agent.Settings) error {
	s.config.Lock()
	defer s.config.Unlock()
	_, err := s.state.configure(ctx, s.conn, settings)
	return err
}

func (s *session) Close(grace time.Duration) { s.conn.proc.Close(grace) }

// request answers the agent's requests. Permission requests are approved;
// the client offers no file system or terminal methods.
func (s *session) request(method string, params json.RawMessage) (any, *rpcError) {
	if method != "session/request_permission" {
		return nil, errMethodNotFound
	}
	var p struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	json.Unmarshal(params, &p)
	chosen := ""
	for _, kind := range []string{"allow_always", "allow_once", "allow"} {
		for _, o := range p.Options {
			if chosen == "" && o.Kind == kind {
				chosen = o.OptionID
			}
		}
	}
	if chosen == "" {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil
	}
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": chosen}}, nil
}

// update reports a session/update notification. Content outside a run, such
// as a loaded session's replay, belongs to no turn.
func (s *session) update(params json.RawMessage) {
	var n notification
	if json.Unmarshal(params, &n) != nil {
		return
	}
	s.mu.Lock()
	loading, running := s.loading, s.running
	s.mu.Unlock()
	if loading {
		return
	}
	for _, e := range s.stream.events(n.Update, false) {
		switch e.Type {
		case agent.Usage, agent.Title:
		default:
			if !running {
				continue
			}
		}
		s.emitter.Emit(e)
	}
}

// notification is a session/update's parameters.
type notification struct {
	SessionID string `json:"sessionId"`
	Update    update `json:"update"`
}

// update is one session update; fields depend on SessionUpdate.
type update struct {
	SessionUpdate string `json:"sessionUpdate"`
	// Content is a content block in message chunks and a list of tool call
	// contents in tool calls.
	Content    json.RawMessage `json:"content"`
	MessageID  string          `json:"messageId"`
	ToolCallID string          `json:"toolCallId"`
	Title      *string         `json:"title"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	RawInput   json.RawMessage `json:"rawInput"`
	RawOutput  json.RawMessage `json:"rawOutput"`
	Locations  []struct {
		Path string `json:"path"`
	} `json:"locations"`
	Used *int64 `json:"used"`
	Size *int64 `json:"size"`
}

// stream gives text and thinking chunks segments: a chunk continues the
// segment of the previous update when that was a chunk of its kind and
// message.
type stream struct {
	kind, message, segment string
	n                      int
	// kinds holds each tool call's kind.
	kinds map[string]string
}

func (st *stream) chunk(kind, message string) string {
	if st.kind != kind || message != "" && message != st.message {
		st.n++
		st.segment = fmt.Sprintf("%s-%d", kind, st.n)
	}
	st.kind, st.message = kind, message
	return st.segment
}

// events converts an update. User messages are reported only from history,
// as live ones are Agio's own prompts.
func (st *stream) events(u update, history bool) []agent.Event {
	switch u.SessionUpdate {
	case "user_message_chunk":
		if text := blockText(u.Content); history && text != "" {
			return []agent.Event{{Type: agent.UserMessage, Segment: st.chunk("user", u.MessageID), Text: text}}
		}
	case "agent_message_chunk":
		if text := blockText(u.Content); text != "" {
			return []agent.Event{{Type: agent.TextDelta, Segment: st.chunk("text", u.MessageID), Text: text}}
		}
	case "agent_thought_chunk":
		if text := blockText(u.Content); text != "" {
			return []agent.Event{{Type: agent.ThinkingDelta, Segment: st.chunk("thinking", u.MessageID), Text: text}}
		}
	case "tool_call", "tool_call_update":
		st.kind = ""
		if u.ToolCallID == "" {
			return nil
		}
		var events []agent.Event
		title := ""
		if u.Title != nil {
			title = *u.Title
		}
		// Updates repeat only changed fields; the call's kind names it.
		if st.kinds == nil {
			st.kinds = map[string]string{}
		}
		kind, known := st.kinds[u.ToolCallID]
		if u.Kind != "" {
			kind = u.Kind
		}
		st.kinds[u.ToolCallID] = kind
		ev := agent.Event{Type: agent.ToolStart, Segment: u.ToolCallID, Preview: agent.Preview(u.RawInput)}
		if !known || u.Kind != "" || toolName(kind, "") == "" {
			ev.Tool = toolName(kind, title)
		}
		if ev.Preview == "" && len(u.Locations) > 0 {
			ev.Preview = u.Locations[0].Path
		}
		if ev.Preview == "" && !known && ev.Tool != agent.FirstLine(title) {
			ev.Preview = agent.FirstLine(title)
		}
		if !known || ev.Tool != "" || ev.Preview != "" {
			events = append(events, ev)
		}
		// Content beside input in an unfinished call is its arguments or
		// description as they stream, not its output; a result may come as
		// raw output only.
		finished := u.Status == "completed" || u.Status == "failed"
		output := ""
		if len(u.RawInput) == 0 || finished {
			output = toolText(u.Content)
		}
		if output == "" && finished {
			output = rawText(u.RawOutput)
		}
		switch {
		case finished:
			events = append(events, agent.Event{Type: agent.ToolEnd, Segment: u.ToolCallID, Text: output, Failed: u.Status == "failed"})
		default:
			if output != "" {
				events = append(events, agent.Event{Type: agent.ToolOutput, Segment: u.ToolCallID, Text: output})
			}
		}
		return events
	case "usage_update":
		var e agent.Event
		if u.Used != nil {
			e.Tokens = *u.Used
		}
		if u.Size != nil {
			e.Window = *u.Size
		}
		if e.Tokens > 0 || e.Window > 0 {
			e.Type = agent.Usage
			return []agent.Event{e}
		}
	case "session_info_update":
		if u.Title != nil && strings.TrimSpace(*u.Title) != "" {
			return []agent.Event{{Type: agent.Title, Text: strings.TrimSpace(*u.Title)}}
		}
	}
	return nil
}

// toolName maps ACP's tool kinds to the names Agio groups activities by;
// other kinds keep the agent's title.
func toolName(kind, title string) string {
	switch kind {
	case "read":
		return "read"
	case "edit", "delete", "move":
		return "edit"
	case "search":
		return "grep"
	case "execute":
		return "bash"
	case "fetch":
		return "web_search"
	}
	return agent.FirstLine(title)
}

// blockText is a content block's text; other blocks have none.
func blockText(raw json.RawMessage) string {
	var b struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &b) != nil || b.Type != "text" {
		return ""
	}
	return b.Text
}

// rawText is a tool's raw output when it is a string or a text block.
func rawText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return blockText(raw)
}

// toolText joins the text of a tool call's content, which replaces earlier
// content when reported.
func toolText(raw json.RawMessage) string {
	var items []struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	var parts []string
	for _, item := range items {
		if item.Type == "content" {
			if text := blockText(item.Content); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// History reads a saved session by loading it in a new process, which
// replays it as updates before the load returns. Replayed chunks are joined
// into settled messages.
func (d *Driver) History(cursor, dir string) ([]agent.Event, error) {
	if cursor == "" {
		return nil, fmt.Errorf("%s session is unknown", d.name())
	}
	var (
		mu     sync.Mutex
		st     stream
		events []agent.Event
	)
	c, err := dial(d.command(dir), d.name(), func(c *conn) {
		c.onUpdate = func(params json.RawMessage) {
			var n notification
			if json.Unmarshal(params, &n) != nil {
				return
			}
			mu.Lock()
			events = append(events, st.events(n.Update, true)...)
			mu.Unlock()
		}
	}, nil)
	if err != nil {
		return nil, err
	}
	defer c.proc.Close(3 * time.Second)
	if !c.caps.LoadSession {
		return nil, errNoSession
	}
	if err := c.callInto(context.Background(), "session/load", map[string]any{"sessionId": cursor, "cwd": dir, "mcpServers": []any{}}, nil); err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	return settle(events), nil
}

// settle joins consecutive chunks of a segment into one message.
func settle(events []agent.Event) []agent.Event {
	var out []agent.Event
	for _, e := range events {
		if n := len(out); n > 0 && e.Segment != "" && out[n-1].Segment == e.Segment {
			switch last := &out[n-1]; {
			case e.Type == agent.UserMessage && last.Type == agent.UserMessage,
				e.Type == agent.TextDelta && last.Type == agent.TextDone,
				e.Type == agent.ThinkingDelta && last.Type == agent.ThinkingDone:
				last.Text += e.Text
				continue
			}
		}
		switch e.Type {
		case agent.TextDelta:
			e.Type = agent.TextDone
		case agent.ThinkingDelta:
			e.Type = agent.ThinkingDone
		}
		out = append(out, e)
	}
	// A user message's segment only joined its chunks.
	for i := range out {
		if out[i].Type == agent.UserMessage {
			out[i].Segment = ""
		}
	}
	return out
}
