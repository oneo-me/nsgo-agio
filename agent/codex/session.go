package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// maxOutput bounds the command output kept while it streams.
const maxOutput = 16 << 10

// Start opens or resumes a thread in o.Dir.
func (d *Driver) Start(o agent.StartOptions, onEvent func(agent.Event), onExit func(error)) (agent.Session, agent.Info, error) {
	s := &session{emitter: agent.NewEmitter(onEvent), settings: o.Settings, outputs: map[string]string{}, ready: make(chan struct{})}
	exited := make(chan error, 1)
	c, err := start(d.command(o.Dir), s.handle, func(err error) { exited <- err })
	if err != nil {
		return nil, agent.Info{}, err
	}
	s.client = c
	close(s.ready)
	ctx := context.Background()
	if len(o.Skills) > 0 {
		// Each session runs its own app server, so its extra skill roots
		// reach this thread alone. A root is searched for SKILL.md files,
		// so a skill directory is one.
		if _, err := c.call(ctx, "skills/extraRoots/set", map[string]any{"extraRoots": o.Skills}); err != nil {
			go c.proc.Close(3 * time.Second)
			return nil, agent.Info{}, fmt.Errorf("codex could not add skills, it may need updating: %w", err)
		}
	}
	params := map[string]any{"cwd": o.Dir, "approvalPolicy": "never", "sandbox": "danger-full-access"}
	if o.Settings.Model != "" {
		params["model"] = o.Settings.Model
	}
	if o.SystemPrompt != "" {
		// Developer instructions follow Codex's own base instructions.
		params["developerInstructions"] = o.SystemPrompt
	}
	method := "thread/start"
	if o.Cursor != "" {
		method = "thread/resume"
		params["threadId"], params["excludeTurns"] = o.Cursor, true
	} else {
		params["serviceName"] = agent.Client.Name
	}
	var opened struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
	}
	if err := c.callInto(ctx, method, params, &opened); err != nil {
		go c.proc.Close(3 * time.Second)
		return nil, agent.Info{}, err
	}
	s.thread = opened.Thread.ID
	if o.Cursor == "" && o.Title != "" {
		c.call(ctx, "thread/name/set", map[string]any{"threadId": s.thread, "name": o.Title})
	}
	info := agent.Info{Cursor: s.thread, Settings: o.Settings}
	if opened.Model != "" {
		info.Settings.Model = opened.Model
	}
	if info.Settings.Effort == "" {
		info.Settings.Effort = opened.ReasoningEffort
	}
	s.settings = info.Settings
	s.emitter.Open()
	go func() {
		err := <-exited
		if onExit != nil {
			onExit(err)
		}
	}()
	return s, info, nil
}

type session struct {
	client  *client
	emitter *agent.Emitter
	thread  string
	// ready is closed once client is set; the reader may run first.
	ready chan struct{}

	mu       sync.Mutex
	settings agent.Settings
	running  bool
	turn     string
	settled  chan struct{}
	// outputs accumulate streamed command output by item.
	outputs map[string]string
	// reasoning is the last reasoning part, to separate parts.
	reasoning string
	// retrying is set while a reported connection error is being retried;
	// it is only used on the reader goroutine.
	retrying bool
}

// begin marks a run as active and returns the channel closed when it settles.
func (s *session) begin() chan struct{} {
	if !s.running {
		s.running, s.settled = true, make(chan struct{})
	}
	return s.settled
}

func (s *session) end() {
	if s.running {
		s.running, s.turn = false, ""
		close(s.settled)
	}
}

func (s *session) Prompt(ctx context.Context, p agent.Prompt) error {
	input := []map[string]any{{"type": "text", "text": p.Message(), "text_elements": []any{}}}
	for _, img := range p.Images {
		if img.Path != "" {
			input = append(input, map[string]any{"type": "localImage", "path": img.Path})
		}
	}
	s.mu.Lock()
	params := map[string]any{"threadId": s.thread, "input": input, "summary": "auto"}
	if s.settings.Model != "" {
		params["model"] = s.settings.Model
	}
	if s.settings.Effort != "" {
		params["effort"] = s.settings.Effort
	}
	params["serviceTierForTurn"] = firstNonEmpty(s.settings.Tier, "default")
	s.begin()
	s.mu.Unlock()
	var started struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	err := s.client.callInto(ctx, "turn/start", params, &started)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.end()
		return err
	}
	if s.running && s.turn == "" {
		s.turn = started.Turn.ID
	}
	return nil
}

// Abort interrupts the active turn and waits for its completion.
func (s *session) Abort(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	for {
		s.mu.Lock()
		running, turn, settled := s.running, s.turn, s.settled
		s.mu.Unlock()
		if !running {
			return nil
		}
		if turn != "" {
			if _, err := s.client.call(ctx, "turn/interrupt", map[string]any{"threadId": s.thread, "turnId": turn}); err != nil {
				return err
			}
			select {
			case <-settled:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		// The turn was requested but its ID has not arrived yet.
		select {
		case <-settled:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Configure takes effect with the next turn, which carries its own model,
// effort and service tier.
func (s *session) Configure(ctx context.Context, settings agent.Settings) error {
	s.mu.Lock()
	s.settings = settings
	s.mu.Unlock()
	return nil
}

func (s *session) Close(grace time.Duration) { s.client.proc.Close(grace) }

func (s *session) handle(m message) {
	if len(m.ID) > 0 {
		go func() {
			<-s.ready
			s.client.reply(m)
		}()
		return
	}
	emit := s.emitter.Emit
	switch m.Method {
	case "turn/started":
		var p struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		json.Unmarshal(m.Params, &p)
		s.mu.Lock()
		s.begin()
		s.turn, s.reasoning = p.Turn.ID, ""
		s.mu.Unlock()
		s.retrying = false
		emit(agent.Event{Type: agent.Started})
	case "item/started", "item/completed":
		var p struct {
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		done := m.Method == "item/completed"
		var head struct {
			Type, ID string
		}
		json.Unmarshal(p.Item, &head)
		if head.Type == "userMessage" && done {
			return
		}
		if head.Type != "userMessage" {
			s.recovered()
		}
		if done {
			s.mu.Lock()
			delete(s.outputs, head.ID)
			s.mu.Unlock()
		}
		for _, ev := range itemEvents(p.Item, done, time.Time{}) {
			emit(ev)
		}
	case "item/agentMessage/delta":
		var p delta
		if json.Unmarshal(m.Params, &p) == nil && p.Delta != "" {
			s.recovered()
			emit(agent.Event{Type: agent.TextDelta, Segment: p.ItemID, Text: p.Delta})
		}
	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		var p delta
		if json.Unmarshal(m.Params, &p) != nil || p.Delta == "" {
			return
		}
		s.recovered()
		// Parts arrive without separators of their own.
		part := p.ItemID + "/" + string(p.SummaryIndex) + string(p.ContentIndex)
		s.mu.Lock()
		separate := s.reasoning != "" && s.reasoning != part && strings.HasPrefix(s.reasoning, p.ItemID+"/")
		s.reasoning = part
		s.mu.Unlock()
		if separate {
			emit(agent.Event{Type: agent.ThinkingDelta, Segment: p.ItemID, Text: "\n\n"})
		}
		emit(agent.Event{Type: agent.ThinkingDelta, Segment: p.ItemID, Text: p.Delta})
	case "item/commandExecution/outputDelta":
		var p delta
		if json.Unmarshal(m.Params, &p) != nil || p.Delta == "" {
			return
		}
		s.mu.Lock()
		out := s.outputs[p.ItemID] + p.Delta
		if len(out) > maxOutput {
			out = out[len(out)-maxOutput:]
		}
		s.outputs[p.ItemID] = out
		s.mu.Unlock()
		emit(agent.Event{Type: agent.ToolOutput, Segment: p.ItemID, Text: out})
	case "thread/tokenUsage/updated":
		var p struct {
			TokenUsage struct {
				Last struct {
					TotalTokens int64 `json:"totalTokens"`
				} `json:"last"`
				ModelContextWindow int64 `json:"modelContextWindow"`
			} `json:"tokenUsage"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			emit(agent.Event{Type: agent.Usage, Tokens: p.TokenUsage.Last.TotalTokens, Window: p.TokenUsage.ModelContextWindow})
		}
	case "thread/name/updated":
		var p struct {
			ThreadID   string `json:"threadId"`
			ThreadName string `json:"threadName"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.ThreadID == s.thread && strings.TrimSpace(p.ThreadName) != "" {
			emit(agent.Event{Type: agent.Title, Text: strings.TrimSpace(p.ThreadName)})
		}
	case "error":
		// Errors that will be retried, such as reconnecting without a
		// network, are shown while the turn keeps running, since Codex may
		// wait for the network indefinitely.
		var p struct {
			Error     turnError `json:"error"`
			WillRetry bool      `json:"willRetry"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.Error.text() != "" {
			s.retrying = p.WillRetry
			emit(agent.Event{Type: agent.Error, Text: p.Error.text()})
		}
	case "turn/completed":
		var p struct {
			Turn turn `json:"turn"`
		}
		json.Unmarshal(m.Params, &p)
		if p.Turn.Status == "failed" && p.Turn.Error != nil && p.Turn.Error.text() != "" {
			emit(agent.Event{Type: agent.Error, Text: p.Turn.Error.text()})
		}
		emit(agent.Event{Type: agent.Settled, Aborted: p.Turn.Status == "interrupted"})
		s.mu.Lock()
		s.end()
		s.outputs = map[string]string{}
		s.mu.Unlock()
	}
}

// recovered clears a retried error once the turn produces output again.
func (s *session) recovered() {
	if s.retrying {
		s.retrying = false
		s.emitter.Emit(agent.Event{Type: agent.Error})
	}
}

type delta struct {
	ItemID       string          `json:"itemId"`
	Delta        string          `json:"delta"`
	SummaryIndex json.RawMessage `json:"summaryIndex"`
	ContentIndex json.RawMessage `json:"contentIndex"`
}

type turn struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Items       []json.RawMessage `json:"items"`
	StartedAt   int64             `json:"startedAt"`
	CompletedAt int64             `json:"completedAt"`
	Error       *turnError        `json:"error"`
}

// turnError is a turn's failure; details carry the underlying cause, such
// as a refused connection.
type turnError struct {
	Message string `json:"message"`
	Details string `json:"additionalDetails"`
}

func (e turnError) text() string {
	message, details := strings.TrimSpace(e.Message), strings.TrimSpace(e.Details)
	switch {
	case details == "" || details == message:
		return message
	case message == "":
		return details
	}
	return message + ": " + details
}

// item holds the fields of the thread items Agio shows.
type item struct {
	Type, ID, Text, Status, Command, Tool, Query, Path, Prompt string
	Summary                                                    []string
	Content                                                    json.RawMessage
	CommandActions                                             []struct {
		Type, Command, Path, Query string
	}
	AggregatedOutput *string
	ExitCode         *int
	Changes          []struct {
		Path, Diff string
	}
	Arguments json.RawMessage
	Result    *struct {
		Content []struct {
			Type, Text string
		}
	}
	ContentItems []struct {
		Type, Text string
	}
	Success *bool
	Error   *struct {
		Message string
	}
}

// itemEvents converts a thread item. Started items open their step;
// completed ones settle it with final content.
func itemEvents(raw json.RawMessage, done bool, at time.Time) []agent.Event {
	var it item
	if json.Unmarshal(raw, &it) != nil {
		return nil
	}
	ev := func(t agent.EventType) agent.Event { return agent.Event{Type: t, Segment: it.ID, Time: at} }
	tool := func(name, preview, output string, failed bool) []agent.Event {
		start := ev(agent.ToolStart)
		start.Tool, start.Preview = name, preview
		if !done {
			return []agent.Event{start}
		}
		end := ev(agent.ToolEnd)
		end.Tool, end.Text, end.Failed = name, output, failed
		return []agent.Event{start, end}
	}
	failed := it.Status == "failed" || it.Status == "declined"
	switch it.Type {
	case "userMessage":
		var content []struct {
			Type, Text string
		}
		json.Unmarshal(it.Content, &content)
		var parts []string
		for _, c := range content {
			if c.Type == "text" && c.Text != "" {
				parts = append(parts, c.Text)
			}
		}
		e := ev(agent.UserMessage)
		e.Text = strings.Join(parts, "\n")
		return []agent.Event{e}
	case "agentMessage":
		if done && it.Text != "" {
			e := ev(agent.TextDone)
			e.Text = it.Text
			return []agent.Event{e}
		}
	case "reasoning":
		text := strings.Join(it.Summary, "\n\n")
		if text == "" {
			var content []string
			json.Unmarshal(it.Content, &content)
			text = strings.Join(content, "\n\n")
		}
		if done && strings.TrimSpace(text) != "" {
			e := ev(agent.ThinkingDone)
			e.Text = text
			return []agent.Event{e}
		}
	case "commandExecution":
		name, preview := commandTool(it)
		output := ""
		if it.AggregatedOutput != nil {
			output = *it.AggregatedOutput
		}
		return tool(name, preview, output, failed || it.ExitCode != nil && *it.ExitCode != 0)
	case "fileChange":
		var paths, diffs []string
		for _, c := range it.Changes {
			paths = append(paths, c.Path)
			diffs = append(diffs, c.Diff)
		}
		preview := strings.Join(paths, ", ")
		if len(paths) > 1 {
			preview = paths[0] + " …"
		}
		return tool("edit", preview, strings.Join(diffs, "\n"), failed)
	case "mcpToolCall", "dynamicToolCall":
		var texts []string
		if it.Result != nil {
			for _, c := range it.Result.Content {
				if c.Text != "" {
					texts = append(texts, c.Text)
				}
			}
		}
		for _, c := range it.ContentItems {
			if c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		if it.Error != nil && it.Error.Message != "" {
			texts, failed = append(texts, it.Error.Message), true
		}
		if it.Success != nil && !*it.Success {
			failed = true
		}
		return tool(it.Tool, agent.Preview(it.Arguments), strings.Join(texts, "\n"), failed)
	case "webSearch":
		return tool("web_search", it.Query, "", false)
	case "imageView":
		return tool("read", it.Path, "", false)
	case "collabAgentToolCall":
		return tool("agent", agent.FirstLine(it.Prompt), "", failed)
	}
	return nil
}

var shellWrapper = regexp.MustCompile(`^\S*/(?:ba|z)?sh -l?c '(.*)'$`)

// commandTool names a command like pi's built-in tools when Codex parsed it
// into a single read, listing or search.
func commandTool(it item) (string, string) {
	if len(it.CommandActions) == 1 {
		a := it.CommandActions[0]
		switch a.Type {
		case "read":
			return "read", a.Path
		case "listFiles":
			return "ls", firstNonEmpty(a.Path, a.Command)
		case "search":
			return "grep", firstNonEmpty(a.Query, a.Command)
		}
		if a.Command != "" {
			return "bash", agent.FirstLine(a.Command)
		}
	}
	command := it.Command
	if m := shellWrapper.FindStringSubmatch(command); m != nil {
		command = strings.ReplaceAll(m[1], `'\''`, `'`)
	}
	return "bash", agent.FirstLine(command)
}

// History reads a thread's turns through a short-lived app server.
func (d *Driver) History(cursor, dir string) ([]agent.Event, error) {
	if cursor == "" {
		return nil, errors.New("codex thread is unknown")
	}
	c, err := start(d.command(dir), nil, nil)
	if err != nil {
		return nil, err
	}
	defer c.proc.Close(3 * time.Second)
	var events []agent.Event
	page := ""
	for i := 0; i < 64; i++ {
		params := map[string]any{"threadId": cursor, "sortDirection": "asc", "itemsView": "full", "limit": 100}
		if page != "" {
			params["cursor"] = page
		}
		var list struct {
			Data       []turn  `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := c.callInto(context.Background(), "thread/turns/list", params, &list); err != nil {
			return nil, err
		}
		for _, t := range list.Data {
			events = append(events, turnEvents(t)...)
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			break
		}
		page = *list.NextCursor
	}
	return events, nil
}

func turnEvents(t turn) []agent.Event {
	started, ended := time.Unix(t.StartedAt, 0), time.Time{}
	if t.CompletedAt > 0 {
		ended = time.Unix(t.CompletedAt, 0)
	}
	var events []agent.Event
	prompted := false
	for _, raw := range t.Items {
		var head struct{ Type string }
		json.Unmarshal(raw, &head)
		at := ended
		if head.Type == "userMessage" {
			if prompted {
				continue
			}
			prompted, at = true, started
		}
		events = append(events, itemEvents(raw, true, at)...)
	}
	if !prompted {
		events = append([]agent.Event{{Type: agent.UserMessage, Time: started}}, events...)
	}
	switch {
	case t.Status == "failed" && t.Error != nil:
		events = append(events, agent.Event{Type: agent.Error, Text: t.Error.text(), Time: ended})
	case t.Status == "interrupted":
		events = append(events, agent.Event{Type: agent.Cancelled, Time: ended})
	}
	return events
}
