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
	st := &store{s: settings{Language: "zh-Hans"}}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := newApp(cx, st, &awake{})
		return chatview.App{Model: chatview.Bind(cx, h), Dir: t.TempDir(),
			Header: a.launchButtons, Settings: a.settingsCategories, SettingsKey: a.settingsKey,
			SettingsSectionActions: a.sectionActions, SettingsCategory: a.category,
			OnAppearanceChange: a.setAppearance, AltShortcuts: a.altShortcuts}
	}, nsgotest.Size(minWindowWidth, minWindowHeight))
	app.Press("id:open-settings")
	app.WaitFor("text:语言", 5*time.Second)
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
