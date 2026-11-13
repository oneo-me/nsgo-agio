package chatview

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	menus "github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/chatview/locales"
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
	app.Press("id:composer >> role:button Send")
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "echo: hello agent") })
	if !hasText(app, "hello agent") {
		t.Fatalf("title missing: %q", app.Texts())
	}
	if c := h.State().Conversations[0]; !c.Bound || len(c.Turns) != 1 {
		t.Fatalf("conversation = %+v", c)
	}
}

func TestNewTaskStartsTasksInTheirGroups(t *testing.T) {
	app, h := startApp(t)
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 1 && h.State().Ready() })
	// New task asks for the project in its hero, and the draft it composes
	// is no task yet.
	app.WaitFor("id:new-task-project", 5*time.Second)
	app.WaitFor("text:No tasks yet.", 5*time.Second)
	app.WaitFor("role:button Opus", 5*time.Second)
	app.Type("id:composer", "hello")
	app.Press("role:button Send")
	// Sending shows the task, listed by when it was last active.
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "echo: hello") && hasText(app, "Today") })
	task := h.State().Conversations[0].ID
	app.Press("id:new-task")
	app.WaitFor("id:new-task-project", 5*time.Second)
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 2 })
	// Its row shows it again.
	app.Press("id:task-" + task)
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "echo: hello") && !app.Exists("id:new-task-page") })
	h.Remove(task)
	app.WaitFor("text:No tasks yet.", 5*time.Second)
}

func TestNewTaskStartsInTheFirstProject(t *testing.T) {
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	last, other := t.TempDir(), t.TempDir()
	if _, err := h.Create(agio.Create{Dir: other, Cursor: "saved", Model: "claude:opus"}); err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return App{Model: Bind(cx, h), Dir: t.TempDir(), Projects: func() []string { return []string{last} }}
	}, nsgotest.Size(900, 640))
	app.WaitUntil(5*time.Second, func() bool { return draftID(h.State()) != "" })
	if d := h.State().Conversation(draftID(h.State())).Dir; d != last {
		t.Fatalf("draft works in %q, want %q", d, last)
	}
	app.Press("id:new-task-project")
	app.WaitFor("id:project-"+filepath.Base(other), 5*time.Second)
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
		return App{Model: Bind(cx, h), Settings: func(own []ui.SettingsCategory) []ui.SettingsCategory {
			return append(own, ui.SettingsCategory{ID: "own", Title: "Own",
				Groups: []ui.SettingsGroup{{ID: "own", Items: []ui.SettingsItem{{ID: "item", Title: "Own item"}}}}})
		}}
	}, nsgotest.Size(900, 640))
	app.Press("id:open-settings")
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, "Own") })
	app.Press("text:Own")
	app.WaitFor("text:Own item", 5*time.Second)
}

func TestAppearanceMenuShowsAndReportsTheChoice(t *testing.T) {
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	var chosen []core.Appearance
	var env *core.Env
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		env = cx.Env()
		return App{Model: Bind(cx, h), OnAppearanceChange: func(a core.Appearance) { chosen = append(chosen, a) }}
	}, nsgotest.Size(900, 640))
	app.Press("id:appearance")
	app.Press("text:Dark")
	app.WaitUntil(5*time.Second, func() bool { return len(chosen) == 1 && chosen[0] == core.AppearanceDark })
	if env.Appearance() != core.AppearanceDark {
		t.Fatalf("the window shows %v, want dark", env.Appearance())
	}
}

func TestNewTaskWorksWhereTheShownTaskDid(t *testing.T) {
	app, h := startApp(t)
	app.WaitUntil(5*time.Second, func() bool { return len(h.State().Conversations) == 1 && h.State().Ready() })
	first := h.State().Conversations[0]
	other := t.TempDir()
	if err := h.Choose(first.ID, agio.Choice{Dir: &other}); err != nil {
		t.Fatal(err)
	}
	app.WaitUntil(5*time.Second, func() bool { return hasText(app, filepath.Base(other)) })
	// New task's picker offers the projects in use and a folder; the
	// composer only names the project.
	app.Press("id:new-task-project")
	app.WaitFor("id:choose-folder", 5*time.Second)
	app.Press("id:project-" + filepath.Base(other))
	if app.Exists("id:choose-folder") {
		t.Fatal("the picker stayed open")
	}
	app.Type("id:composer", "hello")
	app.Press("role:button Send")
	app.WaitUntil(5*time.Second, func() bool { return h.State().Conversation(first.ID).Bound })
	dir := t.TempDir()
	if err := h.Choose(first.ID, agio.Choice{Dir: &dir}); err == nil {
		t.Fatal("moved a started conversation")
	}
	// New task's next draft works where the task shown did.
	app.Press("id:new-task")
	app.WaitUntil(5*time.Second, func() bool {
		s := h.State()
		return len(s.Conversations) == 2 && s.Conversations[0].ID != first.ID && s.Conversations[0].Dir == other
	})
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

func TestAppMenuChecksForUpdatesOnlyWhenAsked(t *testing.T) {
	has := func(items []desktop.MenuItem) bool {
		for _, item := range items[0].Items {
			if item.Label == "Check for Updates…" {
				return true
			}
		}
		return false
	}
	msgs, words := locales.In("en"), menus.In("en")
	if has(appMenu("Agio", msgs, words, func() {}, func() {}, nil)) {
		t.Error("the menu checks for updates without updates")
	}
	if !has(appMenu("Agio", msgs, words, func() {}, func() {}, func() {})) {
		t.Error("the menu lacks Check for Updates…")
	}
}

func TestSettingsCheckForUpdatesOnlyWhenAsked(t *testing.T) {
	for _, updates := range []bool{false, true} {
		h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
			Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
		t.Cleanup(func() { h.Close() })
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return App{Model: Bind(cx, h), Updates: updates}
		}, nsgotest.Size(900, 640))
		app.Press("id:open-settings")
		app.WaitFor("id:settings", 5*time.Second)
		if hasText(app, "Updates") != updates {
			t.Errorf("with Updates %v, the settings show updates: %v", updates, !updates)
		}
		if updates {
			app.Press("text:Updates")
			app.WaitFor("id:check-for-updates", 5*time.Second)
		}
	}
}
