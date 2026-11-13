package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/chatview"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/internal/fakeagent"
)

func TestSettingsApplyTheLanguageAndKeepProviders(t *testing.T) {
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	st := &store[settings]{s: settings{Language: "zh-Hans"}}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := newApp(cx, st, &awake{}, newNamer(h, st, func() string { return "en" }))
		return chatview.App{Model: chatview.Bind(cx, h), Dir: t.TempDir(),
			Header: a.launchButtons, Settings: a.settingsCategories, SettingsKey: a.settingsKey,
			SettingsSectionActions: a.sectionActions, SettingsCategory: a.category,
			OnAppearanceChange: a.setAppearance, AltShortcuts: a.altShortcuts}
	}, nsgotest.Size(minWindowWidth, minWindowHeight))
	app.Press("id:open-settings")
	app.WaitFor("text:语言", 5*time.Second)
	// Privacy ends the list under a section of its own.
	app.WaitFor("text:关于", 5*time.Second)
	app.Press("role:button 添加提供商")
	app.Press("text:OpenAI 兼容")
	app.WaitUntil(5*time.Second, func() bool { return len(st.get().Providers) == 1 })
	app.WaitFor("text:Base URL", 5*time.Second)
	app.Press("role:button 删除提供商")
	app.WaitUntil(5*time.Second, func() bool { return len(st.get().Providers) == 0 })
	app.Press("text:通用")
	app.Press("role:combobox 语言偏好")
	app.Press("text:English")
	app.WaitFor("text:Theme", 5*time.Second)
	if st.get().Language != "en" {
		t.Fatalf("kept language %q, want en", st.get().Language)
	}
}

// TestColorSchemes keeps a scheme chosen for each appearance, which shows
// only in its own.
func TestColorSchemes(t *testing.T) {
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	st := &store[settings]{s: settings{Appearance: "light"}}
	var accent func() core.Color
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := newApp(cx, st, &awake{}, newNamer(h, st, func() string { return "en" }))
		accent = func() core.Color { return core.Untracked(cx, cx.Palette).Accent }
		return chatview.App{Model: chatview.Bind(cx, h), Dir: t.TempDir(),
			Settings: a.settingsCategories, SettingsKey: a.settingsKey, SettingsCategory: a.category}
	}, nsgotest.Size(minWindowWidth, minWindowHeight))
	app.Press("id:open-settings")
	app.WaitFor("text:Light colors", 5*time.Second)
	if len(lightSchemes) != 8 || len(darkSchemes) != 8 {
		t.Fatalf("%d light and %d dark schemes, want 8 of each", len(lightSchemes), len(darkSchemes))
	}
	app.Press("id:dark-scheme-dusk")
	if st.get().DarkScheme != "dusk" {
		t.Fatalf("dark scheme = %q, want dusk", st.get().DarkScheme)
	}
	if got := accent(); got != lightSchemes[0].Accent {
		t.Fatalf("accent = %x after a dark scheme in light, want the light one kept", got)
	}
	app.Press("id:light-scheme-meadow")
	if got, want := accent(), scheme(lightSchemes, "meadow").palette().Accent; got != want {
		t.Fatalf("accent = %x after Meadow, want %x", got, want)
	}
	// A scheme chosen under the pointer hears it leave, so it shows no
	// hover once another is chosen.
	app.Hover("id:light-scheme-sunset")
	app.Press("id:light-scheme-sunset")
	app.HoverOut()
	app.Press("id:light-scheme-lagoon")
	if got, want := app.Find("id:light-scheme-sunset").Style.Background, app.Find("id:light-scheme-citrus").Style.Background; got != want {
		t.Fatalf("Sunset's background = %v after leaving it and choosing Lagoon, want %v as unhovered", got, want)
	}
}

// TestLogoFollowsTheAccent draws the hero's logo in the scheme's accent.
func TestLogoFollowsTheAccent(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(theme(settings{LightScheme: "sunset"}))
		cx.SetAppearance(core.AppearanceLight)
		return core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64)}, Children: []core.Node{logo{}}}
	})
	want := scheme(lightSchemes, "sunset").Accent
	for _, op := range app.Find("logo").Children[0].Ops {
		if op.Color == want {
			return
		}
	}
	t.Fatalf("the bubble is not drawn in the accent %x", want)
}

// TestGrain scatters the grain behind the window once the settings turn it
// on, as strong as they say.
func TestGrain(t *testing.T) {
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	st := &store[settings]{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := newApp(cx, st, &awake{}, newNamer(h, st, func() string { return "en" }))
		return chatview.App{Model: chatview.Bind(cx, h), Dir: t.TempDir(), Backdrop: a.grain(),
			Settings: a.settingsCategories, SettingsKey: a.settingsKey, SettingsCategory: a.category}
	}, nsgotest.Size(minWindowWidth, minWindowHeight))
	if app.Exists("grain") {
		t.Fatal("grain shows before the settings turn it on")
	}
	if st.get().grainAmount() != defaultGrain {
		t.Fatalf("grain strength = %d, want the default", st.get().grainAmount())
	}
	app.Press("id:open-settings")
	app.Press("role:switch Background grain")
	if !st.get().Grain {
		t.Fatal("the switch left the grain off")
	}
	app.WaitFor("backdrop >> grain", 5*time.Second)
}
