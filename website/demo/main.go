// Command demo shows chatview with a scripted conversation, for the
// website's screenshots and for reviewing the views without an agent CLI.
package main

import (
	"os"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/chatview"
	"github.com/oneo-me/nsgo-agio/quota"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// AGIO_DEMO_DETAIL shows the follow-up turn alone, with its steps opened by
// the screenshot script; AGIO_DEMO_DARK shows the dark appearance. Both are
// read once at start.
func main() {
	now := time.Now()
	detail := os.Getenv("AGIO_DEMO_DETAIL") != ""
	turns := []*transcript.Turn{turn(now.Add(-8*time.Minute), 6*time.Second), followUp(now.Add(-2*time.Minute), 4*time.Second)}
	title := "Stop reconnects after sleep"
	if detail {
		turns = []*transcript.Turn{followUp(now.Add(-2*time.Minute), 4*time.Second)}
		title = "Add a regression test"
	}
	b := &backend{state: agio.State{
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
			{
				ID: "conv-1", Title: title, Dir: "~/src/agio", Model: "anthropic/claude-opus-4-6",
				Effort: "high", Bound: true, Created: now.Add(-26 * time.Minute), Updated: now.Add(-1 * time.Minute),
				ContextTokens: 18420, ContextWindow: 200000,
				Turns: turns,
			},
			{ID: "conv-2", Title: "Review the wire protocol", Dir: "~/src/agio", Model: "openai/gpt-5.4", Bound: true,
				Created: now.Add(-2 * time.Hour), Updated: now.Add(-41 * time.Minute)},
			{ID: "conv-3", Title: "Add a CodeBuddy driver", Dir: "~/src/agio", Model: "anthropic/claude-opus-4-6", Bound: true,
				Created: now.Add(-3 * time.Hour), Updated: now.Add(-2 * time.Hour)},
			{ID: "conv-4", Title: "为什么流式输出会卡顿", Dir: "~/src/neorio", Model: "openai/gpt-5.4", Bound: true,
				Created: now.Add(-26 * time.Hour), Updated: now.Add(-25 * time.Hour)},
		},
	}}
	nsgo.Run(func(cx *core.Scope) core.Node {
		return chatview.App{Model: chatview.Bind(cx, b), Dir: "~/src/agio", Name: agent.Client.Title}
	}, nsgo.WindowSize(1180, 780), appearance())
}

// turn is one finished exchange with a little of everything the view shows.
func turn(start time.Time, worked time.Duration) *transcript.Turn {
	t := transcript.New("turn-1", "The client reconnects every time the Mac wakes. Should it?",
		nil, start)
	t.Apply(agent.Event{Type: agent.ThinkingDelta, Segment: "think", Text: "A client keeps the stream open while the machine sleeps, so the kernel closes the socket. The client sees EOF, reports it, and dials again."})
	t.Apply(agent.Event{Type: agent.ThinkingDone, Segment: "think", Text: "A client keeps the stream open while the machine sleeps, so the kernel closes the socket. The client sees EOF, reports it, and dials again."})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "search", Tool: "grep", Preview: "reconnect wire"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "search", Text: "wire/client.go:118: // dial reconnects after the connection is lost.\nwire/client.go:204: c.reconnect()\nwire/wire_test.go:311: TestReconnect"})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "search", Tool: "grep"})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "read", Tool: "read", Preview: "wire/client.go"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "read", Text: "118	// dial reconnects after the connection is lost.\n119	func (c *Client) dial() error {\n120		conn, err := net.Dial(\"tcp\", c.addr)\n..."})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "read", Tool: "read"})
	t.Apply(agent.Event{Type: agent.TextDone, Segment: "prose", Text: "It reconnects by design, but not while the machine sleeps: the client only dials when it has a request to send."})
	t.Apply(agent.Event{Type: agent.TextDone, Segment: "final", Text: "## It only reconnects when needed\n\nThe client **does not** hold the connection open to stay fresh. When the socket goes away, it reports the loss through `State.Err` and waits. The next request dials again:\n\n```go\nif c.conn == nil {\n    if err := c.dial(); err != nil {\n        return err\n    }\n}\n```\n\nA served session publishes at most 30 times a second, so a reconnect costs one publication, not a resync of every turn."})
	t.Finish(transcript.Completed, start.Add(worked))
	return t
}

// backend is a still agio.Backend: the screenshot script only reads State,
// and the settings and limits calls do not change it.
type backend struct {
	mu    sync.Mutex
	state agio.State
}

func (b *backend) State() agio.State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *backend) Watch(func()) func()                { return func() {} }
func (b *backend) Create(agio.Create) (string, error) { return "conv-1", nil }
func (b *backend) Send(string, agio.Message) error    { return nil }
func (b *backend) Stop(string) error                  { return nil }
func (b *backend) Choose(string, agio.Choice) error   { return nil }
func (b *backend) Remove(string) error                { return nil }
func (b *backend) Discover()                          {}

// Enable, EnableModels and RefreshQuota do nothing: this backend is a still
// frame, not an application that keeps settings or reads accounts.
func (b *backend) Enable(agent.Tool, bool) error     { return nil }
func (b *backend) EnableModels([]string, bool) error { return nil }
func (b *backend) RefreshQuota(quota.Provider)       {}
func (b *backend) Close() error                      { return nil }

// appearance shows the demo dark for its dark screenshots.
func appearance() nsgo.Option {
	if os.Getenv("AGIO_DEMO_DARK") != "" {
		return nsgo.Appearance(core.AppearanceDark)
	}
	return nsgo.Appearance(core.AppearanceLight)
}

// followUp is the short second exchange, so the window shows a session
// rather than a single answer.
func followUp(start time.Time, worked time.Duration) *transcript.Turn {
	t := transcript.New("turn-2", "Add a regression test for it.", nil, start)
	t.Apply(agent.Event{Type: agent.ThinkingDelta, Segment: "think-2", Text: "The test needs to drop the socket, then spend a request. It should not wait for a reconnect timer."})
	t.Apply(agent.Event{Type: agent.ThinkingDone, Segment: "think-2", Text: "The test needs to drop the socket, then spend a request. It should not wait for a reconnect timer."})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "edit", Tool: "edit", Preview: "wire/client_test.go"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "edit", Text: "added TestSendAfterConnectionLoss"})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "edit", Tool: "edit"})
	t.Apply(agent.Event{Type: agent.ToolStart, Segment: "bash", Tool: "bash", Preview: "go test ./wire"})
	t.Apply(agent.Event{Type: agent.ToolOutput, Segment: "bash", Text: "ok  \tgithub.com/oneo-me/nsgo-agio/wire\t0.41s"})
	t.Apply(agent.Event{Type: agent.ToolEnd, Segment: "bash", Tool: "bash"})
	t.Apply(agent.Event{Type: agent.TextDone, Segment: "final-2", Text: "Added `TestSendAfterConnectionLoss`: it drops the socket, sends a message, and checks that one dial brings the turns back. `go test ./wire` passes."})
	t.Finish(transcript.Completed, start.Add(worked))
	return t
}
