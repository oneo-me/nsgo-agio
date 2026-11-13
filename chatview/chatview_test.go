package chatview

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/internal/fakeagent"
	"github.com/oneo-me/nsgo-agio/quota"
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

func TestSettingsTurnAgentsOffAndOn(t *testing.T) {
	app, h := startApp(t)
	app.WaitUntil(5*time.Second, func() bool { return h.State().Ready() })
	app.Press("id:open-settings")
	app.WaitFor("id:agent-claude", 5*time.Second)
	app.Press("id:agent-claude")
	app.WaitUntil(5*time.Second, func() bool { return h.State().Tools[0].Disabled })
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "Off") })
	app.Press("id:agent-claude")
	app.WaitUntil(5*time.Second, func() bool { return h.State().Ready() })
}

func TestSettingsShowTheAppsCategories(t *testing.T) {
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return App{Model: Bind(cx, h), Settings: []ui.SettingsCategory{{ID: "own", Title: "Own",
			Groups: []ui.SettingsGroup{{ID: "own", Items: []ui.SettingsItem{{ID: "item", Title: "Own item"}}}}}}}
	}, nsgotest.Size(900, 640))
	app.Press("id:open-settings")
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "Own") })
	app.Press("text:Own")
	app.WaitFor("text:Own item", 5*time.Second)
}

func TestNewConversationChoosesItsProject(t *testing.T) {
	app, h := startApp(t)
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 1 && h.State().Ready() })
	first := h.State().Conversations[0]
	other := t.TempDir()
	h.Choose(first.ID, agio.Choice{Dir: &other})
	app.WaitFor("text:"+filepath.Base(other), 5*time.Second)
	// A new conversation works where the selected one does.
	app.Press("id:new-conversation")
	app.WaitUntil(5*time.Second, func() bool { s := h.State(); return len(s.Conversations) == 2 && s.Conversations[0].Dir == other })
	// Its picker offers the projects in use until the first message.
	app.Press("id:project")
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, other) && hasText(app, "Choose a folder…") })
	app.Press("id:project")
	app.Type("id:composer", "hello")
	app.Press("role:button Send")
	app.WaitUntil(5*time.Second, func() bool { return h.State().Conversations[0].Bound })
	dir := t.TempDir()
	if err := h.Choose(h.State().Conversations[0].ID, agio.Choice{Dir: &dir}); err == nil {
		t.Fatal("moved a started conversation")
	}
}

func TestSettingsTurnModelsOffAndOn(t *testing.T) {
	app, h := startApp(t)
	app.WaitUntil(5*time.Second, func() bool { return h.State().Ready() })
	app.Press("id:open-settings")
	app.WaitFor("id:models-claude", 5*time.Second)
	if !hasText(app, "2/2 models") {
		t.Fatalf("the models' count is missing: %q", app.Texts())
	}
	app.Press("id:models-claude")
	app.Press("id:model-claude:haiku")
	app.WaitUntil(5*time.Second, func() bool {
		t := h.State().Tools[0]
		return len(t.DisabledModels) == 1 && t.DisabledModels[0] == "haiku"
	})
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "1/2 models") })
	app.Press("id:select-no-models")
	app.WaitUntil(5*time.Second, func() bool { return !h.State().Ready() })
	app.Press("id:select-all-models")
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Tools[0].DisabledModels) == 0 })
}

func TestComposerShowsTheModelsLimit(t *testing.T) {
	read := func(_ context.Context, _ []string, p quota.Provider) (quota.Usage, error) {
		if p != quota.Claude {
			return quota.Usage{}, quota.ErrNoCredentials
		}
		return quota.Usage{Plan: "Max", Windows: []quota.Window{
			{Minutes: 300, Percent: 25, ResetsAt: time.Now().Add(2 * time.Hour)},
			{Minutes: 7 * 24 * 60, Percent: 60},
		}}, nil
	}
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir(), ReadQuota: read})
	t.Cleanup(func() { h.Close() })
	dir := t.TempDir()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return App{Model: Bind(cx, h), Dir: dir} }, nsgotest.Size(900, 640))
	app.WaitFor("id:quota", 5*time.Second)
	if !hasText(app, "Resets in 1h 59m") && !hasText(app, "Resets in 2h") {
		t.Fatalf("the five-hour window's reset is missing: %q", app.Texts())
	}
	app.Press("id:quota")
	app.WaitUntil(5*time.Second, func() bool {
		return hasText(app, "Claude · Max") && hasText(app, "75% left") && hasText(app, "Weekly limit")
	})
	app.Press("id:quota")
	// The model picker's heading shows the account's limit too.
	app.Press("id:model-picker")
	app.WaitUntil(5*time.Second, func() bool {
		n := 0
		for _, text := range app.Texts() {
			if strings.HasPrefix(text, "Resets in") {
				n++
			}
		}
		return n == 2
	})
}
