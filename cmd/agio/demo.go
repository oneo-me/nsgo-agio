//go:build agiodemo

// A demo build, go build -tags agiodemo ./cmd/agio, is Agio showing
// scripted tasks instead of running agents, for the website's screenshots
// and for reviewing the app without an agent CLI. It reports no use. The
// website's screenshot script runs it with a HOME of its own, whose
// settings choose the language, appearance and color schemes.

package main

import (
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/quota"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// AGIO_DEMO_DETAIL shows the follow-up turn alone, for the screenshot
// script to open its steps, AGIO_DEMO_NEW opens on New task, and
// AGIO_DEMO_LANG=zh writes the tasks in Chinese. They are read once at
// start.
var (
	demoDetail = os.Getenv("AGIO_DEMO_DETAIL") != ""
	demoWords  = demoEnglish
	shownFirst = "task-1"
)

func init() {
	if os.Getenv("AGIO_DEMO_LANG") == "zh" {
		demoWords = demoChinese
	}
	if os.Getenv("AGIO_DEMO_NEW") != "" {
		shownFirst = ""
	}
}

// demoText is what the tasks say, in one language.
type demoText struct {
	Titles                       [5]string
	DetailTitle                  string
	Ask, Thinking, Prose, Answer string
	FollowUp, FollowThinking     string
	FollowAnswer                 string
}

var demoEnglish = demoText{
	Titles: [5]string{"Stop reconnects after sleep", "Translate the settings page", "Review the wire protocol",
		"Add a CodeBuddy driver", "Fix the gallery's dark mode"},
	DetailTitle: "Add a regression test",
	Ask:         "The client reconnects every time the Mac wakes. Should it?",
	Thinking:    "A client keeps the stream open while the machine sleeps, so the kernel closes the socket. The client sees EOF, reports it, and dials again.",
	Prose:       "It reconnects by design, but not while the machine sleeps: the client only dials when it has a request to send.",
	Answer: "## It only reconnects when needed\n\nThe client **does not** hold the connection open to stay fresh. When the socket goes away, it reports the loss through `State.Err` and waits. The next request dials again:\n\n" +
		"```go\nif c.conn == nil {\n    if err := c.dial(); err != nil {\n        return err\n    }\n}\n```\n\nA served session publishes at most 30 times a second, so a reconnect costs one publication, not a resync of every turn.",
	FollowUp:       "Add a regression test for it.",
	FollowThinking: "The test needs to drop the socket, then spend a request. It should not wait for a reconnect timer.",
	FollowAnswer:   "Added `TestSendAfterConnectionLoss`: it drops the socket, sends a message, and checks that one dial brings the turns back. `go test ./wire` passes.",
}

var demoChinese = demoText{
	Titles:      [5]string{"睡眠后不再重连", "翻译设置页", "审阅 wire 协议", "添加 CodeBuddy 驱动", "修复示例的深色模式"},
	DetailTitle: "添加回归测试",
	Ask:         "每次 Mac 唤醒，客户端都会重连。这是应该的吗？",
	Thinking:    "机器睡眠时客户端仍保持着流，于是内核关闭了套接字。客户端读到 EOF，报告它，然后重新拨号。",
	Prose:       "重连是有意为之，但不会在睡眠时发生：客户端只在有请求要发时才拨号。",
	Answer: "## 只在需要时重连\n\n客户端**不会**为了保持新鲜而一直占着连接。套接字断开时，它通过 `State.Err` 报告并等待，下一个请求再重新拨号：\n\n" +
		"```go\nif c.conn == nil {\n    if err := c.dial(); err != nil {\n        return err\n    }\n}\n```\n\n服务端的会话每秒最多发布 30 次，所以一次重连只花一次发布，而不是重新同步每个轮次。",
	FollowUp:       "为它加一个回归测试。",
	FollowThinking: "测试需要先断开套接字，再发一个请求，不应该等重连计时器。",
	FollowAnswer:   "已添加 `TestSendAfterConnectionLoss`：它断开套接字、发送一条消息，并检查一次拨号就能取回轮次。`go test ./wire` 通过。",
}

func openBackend(string, string, settings) agio.Backend {
	now := time.Now()
	w := demoWords
	turns := []*transcript.Turn{demoTurn(w, now.Add(-8*time.Minute), 6*time.Second), demoFollowUp(w, now.Add(-2*time.Minute), 4*time.Second)}
	title := w.Titles[0]
	if demoDetail {
		turns = turns[1:]
		title = w.DetailTitle
	}
	conv := func(id, title, dir, model string, created, updated time.Duration) *agio.Conversation {
		return &agio.Conversation{ID: id, Title: title, Dir: dir, Model: model, Bound: true,
			Created: now.Add(-created), Updated: now.Add(-updated)}
	}
	first := conv("task-1", title, "~/src/agio", "anthropic/claude-opus-4-6", 26*time.Minute, time.Minute)
	first.Effort, first.Turns = "high", turns
	first.ContextTokens, first.ContextWindow = 18420, 200000
	running := conv("task-2", w.Titles[1], "~/src/neorio", "gpt-5.4-codex", 4*time.Minute, 0)
	running.Running = true
	return &demoBackend{state: agio.State{
		Tools: []agio.Tool{{
			Tool: agent.Pi, Path: "/Users/you/.local/bin/pi", Version: "1.5.0",
			Catalog: agent.Catalog{Model: "anthropic/claude-opus-4-6", Effort: "high", Models: []agent.Model{{
				Tool: agent.Pi, ID: "anthropic/claude-opus-4-6", Name: "Claude Opus 4.6", Provider: "Anthropic",
				Efforts: []string{"low", "medium", "high"}, DefaultEffort: "high", ContextWindow: 200000, Images: true, Default: true,
			}, {
				Tool: agent.Pi, ID: "openai/gpt-5.4", Name: "GPT-5.4", Provider: "OpenAI",
				Efforts: []string{"low", "medium", "high"}, DefaultEffort: "medium", ContextWindow: 400000, Images: true,
			}}},
		}, {
			Tool: agent.Codex, Path: "/opt/homebrew/bin/codex", Version: "0.58.0",
			Catalog: agent.Catalog{Model: "gpt-5.4-codex", Models: []agent.Model{{
				Tool: agent.Codex, ID: "gpt-5.4-codex", Name: "GPT-5.4 Codex", ContextWindow: 400000,
			}}},
		}, {
			Tool: agent.Claude, Path: "/Users/you/.local/bin/claude", Version: "2.1.7",
			Catalog: agent.Catalog{Model: "opus", Models: []agent.Model{{Tool: agent.Claude, ID: "opus", Name: "Opus"}}},
		}},
		Conversations: []*agio.Conversation{
			running, first,
			conv("task-3", w.Titles[2], "~/src/agio", "openai/gpt-5.4", 2*time.Hour, 41*time.Minute),
			conv("task-4", w.Titles[3], "~/src/agio", "anthropic/claude-opus-4-6", 3*time.Hour, 2*time.Hour),
			conv("task-5", w.Titles[4], "~/src/nsgo", "openai/gpt-5.4", 26*time.Hour, 25*time.Hour),
		},
	}}
}

// demoTurn is one finished exchange with a little of everything the view
// shows.
func demoTurn(w demoText, start time.Time, worked time.Duration) *transcript.Turn {
	t := transcript.New("turn-1", w.Ask, nil, start)
	t.Apply(agent.Event{Type: agent.ThinkingDelta, Segment: "think", Text: w.Thinking})
	t.Apply(agent.Event{Type: agent.ThinkingDone, Segment: "think", Text: w.Thinking})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "search", Tool: "grep", Preview: "reconnect wire"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "search", Text: "wire/client.go:118: // dial reconnects after the connection is lost.\nwire/client.go:204: c.reconnect()\nwire/wire_test.go:311: TestReconnect"})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "search", Tool: "grep"})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "read", Tool: "read", Preview: "wire/client.go"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "read", Text: "118	// dial reconnects after the connection is lost.\n119	func (c *Client) dial() error {\n120		conn, err := net.Dial(\"tcp\", c.addr)\n..."})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "read", Tool: "read"})
	t.Apply(agent.Event{Type: agent.TextDone, Segment: "prose", Text: w.Prose})
	t.Apply(agent.Event{Type: agent.TextDone, Segment: "final", Text: w.Answer})
	t.Finish(transcript.Completed, start.Add(worked))
	return t
}

// demoFollowUp is the short second exchange, so the task shows a session
// rather than a single answer.
func demoFollowUp(w demoText, start time.Time, worked time.Duration) *transcript.Turn {
	t := transcript.New("turn-2", w.FollowUp, nil, start)
	t.Apply(agent.Event{Type: agent.ThinkingDelta, Segment: "think-2", Text: w.FollowThinking})
	t.Apply(agent.Event{Type: agent.ThinkingDone, Segment: "think-2", Text: w.FollowThinking})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "edit", Tool: "edit", Preview: "wire/client_test.go"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "edit", Text: "added TestSendAfterConnectionLoss"})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "edit", Tool: "edit"})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "bash", Tool: "bash", Preview: "go test ./wire"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "bash", Text: "ok  \tgithub.com/oneo-me/nsgo-agio/wire\t0.41s"})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "bash", Tool: "bash"})
	t.Apply(agent.Event{Type: agent.TextDone, Segment: "final-2", Text: w.FollowAnswer})
	t.Finish(transcript.Completed, start.Add(worked))
	return t
}

// demoBackend is a scripted agio.Backend: nothing runs, so requests change
// nothing but Create, which adds New task's draft on the default model.
type demoBackend struct {
	mu       sync.Mutex
	state    agio.State
	watchers []func()
}

func (b *demoBackend) State() agio.State {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.state
	s.Conversations = slices.Clone(s.Conversations)
	return s
}

func (b *demoBackend) Watch(fn func()) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.watchers = append(b.watchers, fn)
	return func() {}
}

func (b *demoBackend) Create(c agio.Create) (string, error) {
	if c.Model == "" {
		c.Model, c.Effort = "anthropic/claude-opus-4-6", "high"
	}
	b.mu.Lock()
	id := fmt.Sprintf("draft-%d", len(b.state.Conversations))
	b.state.Conversations = append(b.state.Conversations, &agio.Conversation{ID: id, Title: c.Title, Dir: c.Dir,
		Model: c.Model, Effort: c.Effort, Created: time.Now(), Updated: time.Now()})
	watchers := slices.Clone(b.watchers)
	b.mu.Unlock()
	for _, fn := range watchers {
		fn()
	}
	return id, nil
}

func (b *demoBackend) Send(string, agio.Message) error   { return nil }
func (b *demoBackend) Stop(string) error                 { return nil }
func (b *demoBackend) Choose(string, agio.Choice) error  { return nil }
func (b *demoBackend) Remove(string) error               { return nil }
func (b *demoBackend) Discover()                         {}
func (b *demoBackend) Enable(agent.Tool, bool) error     { return nil }
func (b *demoBackend) EnableModels([]string, bool) error { return nil }
func (b *demoBackend) RefreshQuota(quota.Provider)       {}
func (b *demoBackend) Close() error                      { return nil }
