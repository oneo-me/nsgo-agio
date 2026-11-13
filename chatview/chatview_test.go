package chatview

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/internal/fakeagent"
)

// TestMain hands the main thread to the platform under nsgo test -target.
func TestMain(m *testing.M) { nsgotest.Main(m) }

func startApp(t *testing.T) (*nsgotest.App, *host.Host) {
	t.Helper()
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	dir := t.TempDir()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return App{Model: Bind(cx, h), Dir: dir}
	}, nsgotest.Size(900, 640))
	return app, h
}

// hasText reports whether any shown text contains s.
func hasText(app *nsgotest.App, s string) bool {
	for _, text := range app.Texts() {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func TestAppSendsAndShowsTheReply(t *testing.T) {
	app, h := startApp(t)
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 1 && h.State().Ready() })
	app.WaitFor("role:button Opus", 5*time.Second)
	app.Type("id:composer", "hello agent")
	app.Press("role:button Send")
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "echo: hello agent") })
	if !hasText(app, "hello agent") {
		t.Fatalf("title missing: %q", app.Texts())
	}
	if c := h.State().Conversations[0]; !c.Bound || len(c.Turns) != 1 {
		t.Fatalf("conversation = %+v", c)
	}
}

func TestAppStartsAndRemovesConversations(t *testing.T) {
	app, h := startApp(t)
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 1 })
	app.Press("id:new-conversation")
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 2 })
	for _, c := range h.State().Conversations {
		h.Remove(c.ID)
	}
	// Removing the last starts another, so the window never stands empty.
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 1 })
}

func TestAppChoosesTheModel(t *testing.T) {
	app, h := startApp(t)
	app.WaitFor("role:button Opus", 5*time.Second)
	app.Press("role:button Opus")
	app.Press("role:button Haiku")
	app.WaitUntil(5*time.Second, func() bool {
		s := h.State()
		return len(s.Conversations) == 1 && s.Conversations[0].Model == "claude:haiku"
	})
	app.WaitFor("role:button Haiku", 5*time.Second)
}
