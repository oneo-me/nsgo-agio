package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
)

// abortTimeout bounds the wait for an interrupted run's result.
const abortTimeout = 30 * time.Second

// Start launches Claude Code in o.Dir with a new or resumed session. Claude
// accepts a caller-chosen session ID, so the cursor exists before the first
// prompt.
func (d *Driver) Start(o agent.StartOptions, onEvent func(agent.Event), onExit func(error)) (agent.Session, agent.Info, error) {
	s := &session{driver: d, dir: o.Dir, id: o.Cursor, resume: o.Cursor != "", settings: o.Settings,
		emitter: agent.NewEmitter(onEvent), onExit: onExit}
	if s.id == "" {
		s.id = newSessionID()
	}
	if err := s.launch(); err != nil {
		return nil, agent.Info{}, err
	}
	s.emitter.Open()
	return s, agent.Info{Cursor: s.id, Settings: o.Settings}, nil
}

// session is one Claude Code conversation. Changing the reasoning level
// relaunches the process on the same session between runs, because the CLI
// takes it only as a launch option.
type session struct {
	driver  *Driver
	dir, id string
	emitter *agent.Emitter
	onExit  func(error)

	mu       sync.Mutex
	proc     *agent.Process
	resume   bool
	settings agent.Settings
	// launched holds the settings the process started with.
	launched agent.Settings
	running  bool
	aborting bool
	settled  chan struct{}
	closed   bool
	next     int
	stream   streamState
	// background lists the process's unfinished background tasks. Claude
	// Code ends a run while they continue, then starts a new run when one
	// finishes; Agio's run spans both until none remain.
	background []string
	// waiting is set between such a result and the run that resumes it.
	waiting bool
}

// streamState follows the main conversation's messages; it is only used on
// the reader goroutine of the current process.
type streamState struct {
	message string
	// blocks counts content blocks per message, because each assistant
	// record carries one block without its index.
	blocks map[string]int
	model  string
	// retrying is set while a failed API request is being retried.
	retrying bool
}

func (s *session) args() []string {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--include-partial-messages", "--thinking-display", "summarized",
		"--permission-mode", "bypassPermissions", "--dangerously-skip-permissions"}
	if s.settings.Model != "" {
		args = append(args, "--model", s.settings.Model)
	}
	if s.settings.Effort != "" {
		args = append(args, "--effort", s.settings.Effort)
	}
	if s.resume {
		return append(args, "--resume", s.id)
	}
	return append(args, "--session-id", s.id)
}

// launch starts a process; the caller holds no lock.
func (s *session) launch() error {
	s.mu.Lock()
	args := s.args()
	s.launched, s.stream = s.settings, streamState{blocks: map[string]int{}}
	s.background, s.waiting = nil, false
	s.mu.Unlock()
	var proc *agent.Process
	ready := make(chan struct{})
	proc, err := agent.StartProcess(s.driver.command(s.dir, args...), func(line []byte) {
		<-ready
		s.handle(proc, line)
	}, func(err error) {
		<-ready
		s.exited(proc, err)
	})
	if err != nil {
		return fmt.Errorf("start claude: %w", err)
	}
	s.mu.Lock()
	s.proc, s.resume = proc, true
	s.mu.Unlock()
	close(ready)
	return nil
}

func (s *session) current(proc *agent.Process) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proc == proc
}

func (s *session) exited(proc *agent.Process, err error) {
	s.mu.Lock()
	if s.proc != proc {
		s.mu.Unlock()
		return // replaced by a relaunch
	}
	s.end()
	s.mu.Unlock()
	if s.onExit != nil {
		s.onExit(err)
	}
}

func (s *session) end() {
	if s.running {
		s.running, s.aborting, s.waiting = false, false, false
		close(s.settled)
	}
}

func (s *session) Prompt(ctx context.Context, p agent.Prompt) error {
	s.mu.Lock()
	old := s.proc
	relaunch := !s.running && (old == nil || s.settings.Effort != s.launched.Effort)
	if relaunch {
		s.proc = nil // its exit is not the session's
	}
	s.mu.Unlock()
	if relaunch {
		if old != nil {
			old.Close(3 * time.Second)
		}
		if err := s.launch(); err != nil {
			return err
		}
	}
	content := []map[string]any{{"type": "text", "text": p.Message()}}
	for _, img := range p.Images {
		content = append(content, map[string]any{"type": "image",
			"source": map[string]any{"type": "base64", "media_type": img.MimeType, "data": img.Data}})
	}
	s.mu.Lock()
	if !s.running {
		s.running, s.settled = true, make(chan struct{})
	}
	proc := s.proc
	s.mu.Unlock()
	if proc == nil {
		return agent.ErrClosed
	}
	s.emitter.Emit(agent.Event{Type: agent.Started})
	err := proc.Send(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}, "parent_tool_use_id": nil})
	if err != nil {
		s.mu.Lock()
		s.end()
		s.mu.Unlock()
	}
	return err
}

// Abort interrupts the run and waits for its result.
func (s *session) Abort(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.aborting = true
	proc, settled, waiting, tasks := s.proc, s.settled, s.waiting, s.background
	s.mu.Unlock()
	if proc == nil {
		return agent.ErrClosed
	}
	// Stopped tasks do not resume the conversation.
	for _, task := range tasks {
		if err := s.control(proc, map[string]any{"subtype": "stop_task", "task_id": task}); err != nil {
			return err
		}
	}
	if waiting {
		// No run is active, so an interrupt would produce no result.
		s.mu.Lock()
		s.end()
		s.mu.Unlock()
		s.emitter.Emit(agent.Event{Type: agent.Settled, Aborted: true})
		return nil
	}
	if err := s.control(proc, map[string]any{"subtype": "interrupt"}); err != nil {
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

// control sends one control request with a new request ID.
func (s *session) control(proc *agent.Process, request map[string]any) error {
	s.mu.Lock()
	s.next++
	id := fmt.Sprintf("%s-%d", agent.Client.Name, s.next)
	s.mu.Unlock()
	return proc.Send(map[string]any{"type": "control_request", "request_id": id, "request": request})
}

// Configure switches models on the live process; a new reasoning level
// applies from the next prompt, which relaunches the process.
func (s *session) Configure(ctx context.Context, settings agent.Settings) error {
	s.mu.Lock()
	changed := settings.Model != s.settings.Model
	s.settings = settings
	proc := s.proc
	s.mu.Unlock()
	if !changed || settings.Model == "" || proc == nil {
		return nil
	}
	return s.control(proc, map[string]any{"subtype": "set_model", "model": settings.Model})
}

func (s *session) Close(grace time.Duration) {
	s.mu.Lock()
	proc := s.proc
	s.mu.Unlock()
	if proc != nil {
		proc.Close(grace)
	}
}

// record is one stream-json output line.
type record struct {
	Type            string          `json:"type"`
	Subtype         string          `json:"subtype"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	IsReplay        bool            `json:"isReplay"`
	Event           json.RawMessage `json:"event"`
	Message         *apiMessage     `json:"message"`
	IsError         bool            `json:"is_error"`
	Result          string          `json:"result"`
	ModelUsage      map[string]struct {
		ContextWindow  int64  `json:"contextWindow"`
		CanonicalModel string `json:"canonicalModel"`
	} `json:"modelUsage"`
	Tasks []struct {
		TaskID string `json:"task_id"`
	} `json:"tasks"`
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype  string          `json:"subtype"`
		ToolName string          `json:"tool_name"`
		Input    json.RawMessage `json:"input"`
	} `json:"request"`
}

type apiMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   json.RawMessage `json:"usage"`
}

type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (m *apiMessage) blocks() []block {
	if m == nil {
		return nil
	}
	var blocks []block
	if json.Unmarshal(m.Content, &blocks) != nil {
		var text string
		if json.Unmarshal(m.Content, &text) == nil && text != "" {
			blocks = []block{{Type: "text", Text: text}}
		}
	}
	return blocks
}

func (s *session) handle(proc *agent.Process, line []byte) {
	var r record
	if json.Unmarshal(line, &r) != nil || !s.current(proc) {
		return
	}
	emit := s.emitter.Emit
	st := &s.stream
	// Subagents run in their own context; their steps stay out of the
	// main conversation.
	if r.ParentToolUseID != nil && *r.ParentToolUseID != "" {
		return
	}
	switch r.Type {
	case "stream_event":
		var e struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
			ContentBlock block `json:"content_block"`
			Delta        struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"delta"`
		}
		if json.Unmarshal(r.Event, &e) != nil {
			return
		}
		segment := fmt.Sprintf("%s-%d", st.message, e.Index)
		switch e.Type {
		case "message_start":
			st.message = e.Message.ID
			s.mu.Lock()
			s.waiting = false
			s.mu.Unlock()
			if st.retrying {
				st.retrying = false
				emit(agent.Event{Type: agent.Error})
			}
		case "content_block_start":
			if e.ContentBlock.Type == "tool_use" {
				emit(agent.Event{Type: agent.ToolStart, Segment: e.ContentBlock.ID, Tool: toolName(e.ContentBlock.Name)})
			}
		case "content_block_delta":
			switch {
			case e.Delta.Type == "text_delta" && e.Delta.Text != "":
				emit(agent.Event{Type: agent.TextDelta, Segment: segment, Text: e.Delta.Text})
			case e.Delta.Type == "thinking_delta" && e.Delta.Thinking != "":
				emit(agent.Event{Type: agent.ThinkingDelta, Segment: segment, Text: e.Delta.Thinking})
			}
		}
	case "assistant":
		m := r.Message
		if m == nil {
			return
		}
		st.model = m.Model
		for _, b := range m.blocks() {
			index := st.blocks[m.ID]
			st.blocks[m.ID] = index + 1
			if ev, ok := blockEvent(b, fmt.Sprintf("%s-%d", m.ID, index), time.Time{}); ok {
				emit(ev)
			}
		}
		if n := usageTokens(m.Usage); n > 0 {
			emit(agent.Event{Type: agent.Usage, Tokens: n})
		}
	case "user":
		if r.IsReplay {
			return
		}
		for _, b := range r.Message.blocks() {
			if b.Type == "tool_result" {
				emit(agent.Event{Type: agent.ToolEnd, Segment: b.ToolUseID, Text: resultText(b.Content), Failed: b.IsError})
			}
		}
	case "system":
		// Claude Code retries failed requests for minutes, such as without
		// a network; the failure is shown while the run keeps going.
		var retry apiRetry
		if r.Subtype == "api_retry" && json.Unmarshal(line, &retry) == nil {
			st.retrying = true
			emit(agent.Event{Type: agent.Error, Text: retry.text()})
		}
		if r.Subtype == "background_tasks_changed" {
			var tasks []string
			for _, task := range r.Tasks {
				tasks = append(tasks, task.TaskID)
			}
			s.mu.Lock()
			s.background = tasks
			s.mu.Unlock()
		}
	case "control_request":
		s.answer(proc, r)
	case "result":
		s.mu.Lock()
		// A result after Abort settled a waiting run belongs to no run.
		if !s.running {
			s.mu.Unlock()
			return
		}
		aborted := s.aborting
		s.waiting = !aborted && len(s.background) > 0
		waiting := s.waiting
		s.mu.Unlock()
		if r.IsError && !aborted {
			emit(agent.Event{Type: agent.Error, Text: firstNonEmpty(r.Result, r.Subtype, "error")})
		}
		var window int64
		for name, u := range r.ModelUsage {
			if name == st.model || u.CanonicalModel == st.model {
				window = u.ContextWindow
				break
			}
			window = max(window, u.ContextWindow)
		}
		if window > 0 {
			emit(agent.Event{Type: agent.Usage, Window: window})
		}
		if waiting {
			return
		}
		emit(agent.Event{Type: agent.Settled, Aborted: aborted})
		s.mu.Lock()
		s.end()
		s.mu.Unlock()
	}
}

// apiRetry is a system record announcing a retried API request.
type apiRetry struct {
	Attempt     int    `json:"attempt"`
	MaxRetries  int    `json:"max_retries"`
	ErrorStatus int    `json:"error_status"`
	Error       string `json:"error"`
}

// text explains the failure. A request without an HTTP status never reached
// the API.
func (r apiRetry) text() string {
	cause := r.Error
	switch {
	case r.ErrorStatus > 0 && (cause == "" || cause == "unknown"):
		cause = fmt.Sprintf("HTTP %d", r.ErrorStatus)
	case r.ErrorStatus > 0:
		cause = fmt.Sprintf("HTTP %d %s", r.ErrorStatus, cause)
	case cause == "" || cause == "unknown":
		cause = "connection error"
	}
	if r.MaxRetries > 0 {
		return fmt.Sprintf("API request failed (%s), retrying %d/%d", cause, r.Attempt, r.MaxRetries)
	}
	return fmt.Sprintf("API request failed (%s), retrying", cause)
}

// answer approves tool use, which bypassed permissions rarely ask for, and
// declines questions because Agio has no UI to answer them.
func (s *session) answer(proc *agent.Process, r record) {
	if r.Request.Subtype != "can_use_tool" {
		return
	}
	decision := map[string]any{"behavior": "allow", "updatedInput": r.Request.Input}
	if r.Request.ToolName == "AskUserQuestion" {
		decision = map[string]any{"behavior": "deny", "message": agent.Client.Title + " cannot ask the user questions. Continue with your best judgment."}
	}
	go proc.Send(map[string]any{"type": "control_response",
		"response": map[string]any{"subtype": "success", "request_id": r.RequestID, "response": decision}})
}

// blockEvent converts a completed assistant content block.
func blockEvent(b block, segment string, at time.Time) (agent.Event, bool) {
	switch b.Type {
	case "text":
		if b.Text != "" {
			return agent.Event{Type: agent.TextDone, Segment: segment, Text: b.Text, Time: at}, true
		}
	case "thinking":
		if strings.TrimSpace(b.Thinking) != "" {
			return agent.Event{Type: agent.ThinkingDone, Segment: segment, Text: b.Thinking, Time: at}, true
		}
	case "tool_use":
		return agent.Event{Type: agent.ToolStart, Segment: b.ID, Tool: toolName(b.Name), Preview: agent.Preview(b.Input), Time: at}, true
	}
	return agent.Event{}, false
}

// interrupted is the text Claude Code records when the user stops a run.
const interrupted = "[Request interrupted by user"

// History reads the session's active branch from Claude Code's saved
// transcript.
func (d *Driver) History(cursor, dir string) ([]agent.Event, error) {
	if cursor == "" {
		return nil, errors.New("claude session is unknown")
	}
	matches, _ := filepath.Glob(filepath.Join(d.projectsDir(), "*", cursor+".jsonl"))
	if len(matches) == 0 {
		return nil, os.ErrNotExist
	}
	f, err := os.Open(matches[0])
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return historyEvents(f)
}

func historyEvents(f *os.File) ([]agent.Event, error) {
	type entry struct {
		Type        string      `json:"type"`
		UUID        string      `json:"uuid"`
		ParentUUID  *string     `json:"parentUuid"`
		IsSidechain bool        `json:"isSidechain"`
		IsMeta      bool        `json:"isMeta"`
		Timestamp   time.Time   `json:"timestamp"`
		Message     *apiMessage `json:"message"`
		Origin      struct {
			Kind string `json:"kind"`
		} `json:"origin"`
	}
	byID := map[string]*entry{}
	leaf := ""
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64<<20)
	for scanner.Scan() {
		var e entry
		if json.Unmarshal(scanner.Bytes(), &e) != nil || e.UUID == "" || e.IsSidechain {
			continue
		}
		byID[e.UUID] = &e
		leaf = e.UUID
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	var chain []*entry
	for id, seen := leaf, map[string]bool{}; id != "" && !seen[id]; {
		seen[id] = true
		e := byID[id]
		if e == nil {
			break
		}
		chain = append(chain, e)
		if e.ParentUUID == nil {
			break
		}
		id = *e.ParentUUID
	}
	var events []agent.Event
	for i := len(chain) - 1; i >= 0; i-- {
		e := chain[i]
		switch {
		// A background task's notification resumes the run that started it.
		case e.Type == "user" && !e.IsMeta && e.Origin.Kind != "task-notification":
			var texts []string
			for _, b := range e.Message.blocks() {
				switch b.Type {
				case "tool_result":
					events = append(events, agent.Event{Type: agent.ToolEnd, Segment: b.ToolUseID, Text: resultText(b.Content), Failed: b.IsError, Time: e.Timestamp})
				case "text":
					texts = append(texts, b.Text)
				}
			}
			text := strings.Join(texts, "\n")
			switch {
			case strings.HasPrefix(text, interrupted):
				events = append(events, agent.Event{Type: agent.Cancelled, Time: e.Timestamp})
			case strings.TrimSpace(text) != "":
				events = append(events, agent.Event{Type: agent.UserMessage, Text: text, Time: e.Timestamp})
			}
		case e.Type == "assistant" && e.Message != nil:
			for j, b := range e.Message.blocks() {
				if ev, ok := blockEvent(b, fmt.Sprintf("%s-%d", e.UUID, j), e.Timestamp); ok {
					events = append(events, ev)
				}
			}
			if n := usageTokens(e.Message.Usage); n > 0 {
				events = append(events, agent.Event{Type: agent.Usage, Tokens: n, Time: e.Timestamp})
			}
		}
	}
	return events, nil
}
