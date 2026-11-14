package pi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// Driver runs pi through its RPC mode as an agent.Driver.
type Driver struct {
	Command Command
}

func (d *Driver) Tool() agent.Tool { return agent.Pi }

// Catalog runs a short-lived, sessionless process to read the default model
// and every configured model.
func (d *Driver) Catalog(ctx context.Context, dir string) (agent.Catalog, error) {
	c, err := Start(Options{Command: d.Command, Dir: dir, Args: []string{"--no-session"}}, nil, nil)
	if err != nil {
		return agent.Catalog{}, err
	}
	defer c.Close(3 * time.Second)
	var state State
	if err := c.CallInto(ctx, map[string]any{"type": "get_state"}, &state); err != nil {
		return agent.Catalog{}, err
	}
	var list struct {
		Models []Model `json:"models"`
	}
	if err := c.CallInto(ctx, map[string]any{"type": "get_available_models"}, &list); err != nil {
		return agent.Catalog{}, err
	}
	catalog := agent.Catalog{Effort: state.ThinkingLevel}
	if state.Model != nil {
		catalog.Model = state.Model.Key()
	}
	for _, m := range list.Models {
		catalog.Models = append(catalog.Models, m.Agent())
	}
	return catalog, nil
}

// Agent converts a configured model to the tool-neutral form.
func (m Model) Agent() agent.Model {
	name := m.Name
	if name == "" {
		name = m.ID
	}
	return agent.Model{Tool: agent.Pi, ID: m.Key(), Name: name, Provider: m.Provider, Detail: m.ID,
		Efforts: m.Levels(), ContextWindow: m.ContextWindow, Images: m.AcceptsImages()}
}

// Start launches pi in o.Dir, applies the settings and reads its state.
func (d *Driver) Start(o agent.StartOptions, onEvent func(agent.Event), onExit func(error)) (agent.Session, agent.Info, error) {
	emitter := agent.NewEmitter(onEvent)
	s := &session{emitter: emitter, ready: make(chan struct{})}
	exited := make(chan error, 1)
	var args []string
	if o.SystemPrompt != "" {
		args = []string{"--append-system-prompt", o.SystemPrompt}
	}
	c, err := Start(Options{Command: d.Command, Dir: o.Dir, Session: o.Cursor, Args: args},
		s.translate,
		func(err error) { exited <- err })
	if err != nil {
		return nil, agent.Info{}, err
	}
	s.client = c
	close(s.ready)
	ctx := context.Background()
	if provider, id, ok := strings.Cut(o.Settings.Model, "/"); ok {
		_, err = c.Call(ctx, map[string]any{"type": "set_model", "provider": provider, "modelId": id})
	}
	if err == nil && o.Settings.Effort != "" {
		_, err = c.Call(ctx, map[string]any{"type": "set_thinking_level", "level": o.Settings.Effort})
	}
	var state State
	if err == nil {
		err = c.CallInto(ctx, map[string]any{"type": "get_state"}, &state)
	}
	if err != nil {
		go c.Close(3 * time.Second)
		return nil, agent.Info{}, err
	}
	if o.Cursor == "" && o.Title != "" {
		c.Call(ctx, map[string]any{"type": "set_session_name", "name": o.Title})
	}
	info := agent.Info{Cursor: state.SessionFile, Settings: o.Settings}
	if state.Model != nil {
		info.Settings.Model = state.Model.Key()
		info.ContextWindow = state.Model.ContextWindow
	}
	if state.ThinkingLevel != "" {
		info.Settings.Effort = state.ThinkingLevel
	}
	var stats Stats
	if c.CallInto(ctx, map[string]any{"type": "get_session_stats"}, &stats) == nil {
		info.ContextWindow, info.ContextTokens = stats.apply(info.ContextWindow, 0)
	}
	emitter.Open()
	go func() {
		err := <-exited
		if onExit != nil {
			onExit(err)
		}
	}()
	return s, info, nil
}

func (s Stats) apply(window, tokens int64) (int64, int64) {
	if u := s.ContextUsage; u != nil {
		if u.ContextWindow > 0 {
			window = u.ContextWindow
		}
		if u.Tokens != nil {
			tokens = *u.Tokens
		}
	}
	return window, tokens
}

// session translates one pi process's events; translate runs only on the
// client's reader goroutine.
type session struct {
	client  *Client
	emitter *agent.Emitter
	// ready is closed once client is set; the reader may start first.
	ready chan struct{}
	// message counts assistant messages so content indexes stay distinct.
	message int
}

func (s *session) Prompt(ctx context.Context, p agent.Prompt) error {
	command := map[string]any{"type": "prompt", "message": p.Message()}
	var images []Image
	for _, img := range p.Images {
		images = append(images, Image{Type: "image", Data: img.Data, MimeType: img.MimeType})
	}
	if len(images) > 0 {
		command["images"] = images
	}
	_, err := s.client.Call(ctx, command)
	return err
}

// Abort returns after pi's response, which follows the aborted run's
// agent_settled event.
func (s *session) Abort(ctx context.Context) error {
	_, err := s.client.Call(ctx, map[string]any{"type": "abort"})
	return err
}

func (s *session) Configure(ctx context.Context, settings agent.Settings) error {
	if provider, id, ok := strings.Cut(settings.Model, "/"); ok {
		if _, err := s.client.Call(ctx, map[string]any{"type": "set_model", "provider": provider, "modelId": id}); err != nil {
			return err
		}
	}
	if settings.Effort == "" {
		return nil
	}
	_, err := s.client.Call(ctx, map[string]any{"type": "set_thinking_level", "level": settings.Effort})
	return err
}

func (s *session) Close(grace time.Duration) { s.client.Close(grace) }

func (s *session) segment(index int) string { return fmt.Sprintf("m%d-%d", s.message, index) }

func (s *session) translate(e Event) {
	emit := s.emitter.Emit
	switch e.Type {
	case "agent_start", "turn_start":
		emit(agent.Event{Type: agent.Started})
	case "message_start":
		if e.Message == nil {
			return
		}
		switch e.Message.Role {
		case "user":
			emit(agent.Event{Type: agent.UserMessage, Text: e.Message.Content.Text()})
		case "assistant":
			s.message++
		}
	case "message_update":
		u := e.AssistantMessageEvent
		if u == nil {
			return
		}
		id := s.segment(u.ContentIndex)
		switch u.Type {
		case "text_delta":
			emit(agent.Event{Type: agent.TextDelta, Segment: id, Text: u.Delta})
		case "text_end":
			emit(agent.Event{Type: agent.TextDone, Segment: id, Text: u.Content})
		case "thinking_delta":
			emit(agent.Event{Type: agent.ThinkingDelta, Segment: id, Text: u.Delta})
		case "thinking_end":
			emit(agent.Event{Type: agent.ThinkingDone, Segment: id, Text: u.Content})
		case "toolcall_start":
			emit(agent.Event{Type: agent.ToolStart, Segment: u.ID, Tool: u.ToolName})
		case "toolcall_end":
			if u.ToolCall != nil {
				emit(agent.Event{Type: agent.ToolStart, Segment: u.ToolCall.ID, Tool: u.ToolCall.Name, Preview: agent.Preview(u.ToolCall.Arguments)})
			}
		}
		if n := e.Usage.ContextTokens(); n > 0 {
			emit(agent.Event{Type: agent.Usage, Tokens: n})
		}
	case "message_end":
		if e.Message != nil && e.Message.Role == "assistant" {
			for _, ev := range assistantEvents(e.Message, s.segment) {
				emit(ev)
			}
		}
	case "tool_execution_start", "tool_execution_update", "tool_execution_end":
		emit(agent.Event{Type: agent.ToolStart, Segment: e.ToolCallID, Tool: e.ToolName, Preview: agent.Preview(e.Args)})
		if e.PartialResult != nil {
			emit(agent.Event{Type: agent.ToolOutput, Segment: e.ToolCallID, Text: e.PartialResult.Content.Text()})
		}
		if e.Type == "tool_execution_end" {
			end := agent.Event{Type: agent.ToolEnd, Segment: e.ToolCallID, Failed: e.IsError}
			if e.Result != nil {
				end.Text = e.Result.Content.Text()
			}
			emit(end)
		}
	case "auto_retry_end":
		if e.Success {
			emit(agent.Event{Type: agent.Error})
		} else {
			emit(agent.Event{Type: agent.Error, Text: e.FinalError})
		}
	case "session_info_changed":
		if e.Name != nil && strings.TrimSpace(*e.Name) != "" {
			emit(agent.Event{Type: agent.Title, Text: strings.TrimSpace(*e.Name)})
		}
	case "extension_ui_request":
		// Dialog requests would block the run; Agio has no extension UI.
		switch e.Method {
		case "select", "confirm", "input", "editor":
			go func() {
				<-s.ready
				s.client.Send(map[string]any{"type": "extension_ui_response", "id": e.ID, "cancelled": true})
			}()
		}
	case "extension_error":
		if e.Error != "" {
			emit(agent.Event{Type: agent.Error, Text: e.Error})
		}
	case "agent_settled":
		emit(agent.Event{Type: agent.Settled, Aborted: e.Aborted})
		go func() {
			<-s.ready
			var stats Stats
			if s.client.CallInto(context.Background(), map[string]any{"type": "get_session_stats"}, &stats) == nil {
				window, tokens := stats.apply(0, 0)
				s.emitter.Emit(agent.Event{Type: agent.Usage, Tokens: tokens, Window: window})
			}
		}()
	}
}

// assistantEvents reconciles a message with its authoritative final content.
func assistantEvents(m *Message, segment func(int) string) []agent.Event {
	var events []agent.Event
	at := time.Time{}
	if m.Timestamp > 0 {
		at = time.UnixMilli(m.Timestamp)
	}
	for i, b := range m.Content {
		switch b.Type {
		case "text":
			if b.Text != "" {
				events = append(events, agent.Event{Type: agent.TextDone, Segment: segment(i), Text: b.Text, Time: at})
			}
		case "thinking":
			if b.Thinking != "" {
				events = append(events, agent.Event{Type: agent.ThinkingDone, Segment: segment(i), Text: b.Thinking, Time: at})
			}
		case "toolCall":
			events = append(events, agent.Event{Type: agent.ToolStart, Segment: b.ID, Tool: b.Name, Preview: agent.Preview(b.Arguments), Time: at})
		}
	}
	switch m.StopReason {
	case "error":
		text := m.ErrorMessage
		if strings.TrimSpace(text) == "" {
			text = "error"
		}
		events = append(events, agent.Event{Type: agent.Error, Text: text, Time: at})
	case "aborted":
		events = append(events, agent.Event{Type: agent.Cancelled, Time: at})
	}
	if n := m.Usage.ContextTokens(); n > 0 {
		events = append(events, agent.Event{Type: agent.Usage, Tokens: n, Time: at})
	}
	return events
}

// History reads the active branch of a saved session as settled events.
func (d *Driver) History(cursor, dir string) ([]agent.Event, error) {
	history, err := ReadSession(cursor)
	if err != nil {
		return nil, err
	}
	return HistoryEvents(history.Messages), nil
}

// HistoryEvents converts saved messages to events.
func HistoryEvents(messages []Message) []agent.Event {
	var events []agent.Event
	for i := range messages {
		m := &messages[i]
		switch m.Role {
		case "user":
			events = append(events, agent.Event{Type: agent.UserMessage, Text: m.Content.Text(), Time: time.UnixMilli(m.Timestamp)})
		case "assistant":
			events = append(events, assistantEvents(m, func(index int) string { return fmt.Sprintf("h%d-%d", i, index) })...)
		case "toolResult":
			events = append(events, agent.Event{Type: agent.ToolEnd, Segment: m.ToolCallID, Tool: m.ToolName, Text: m.Content.Text(), Failed: m.IsError})
		}
	}
	return events
}
