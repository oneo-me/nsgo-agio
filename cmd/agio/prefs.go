package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/oneo-me/nsgo-agio/cmd/agio/locales"
	"github.com/oneo-me/nsgo-agio/internal/launchers"
)

// app is the window's side of the settings: their pages and what applies
// them as they change.
type app struct {
	cx    *core.Scope
	st    *store[settings]
	aw    *awake
	namer *namer
	// rev counts the changes made here, which the controls follow.
	rev *core.Signal[int]
	// category is the settings' page shown.
	category *core.Signal[string]
	// installed are this computer's launchers, once found.
	installed []launchers.Installed
}

// newApp applies the saved language and appearance, looks for the
// launchers off the engine and follows why naming fails.
func newApp(cx *core.Scope, st *store[settings], aw *awake, nm *namer) *app {
	a := &app{cx: cx, st: st, aw: aw, namer: nm, rev: core.NewSignal(cx, 0), category: core.NewSignal(cx, "")}
	nm.onChange(func() { cx.Post(func() { a.rev.Update(func(n int) int { return n + 1 }) }) })
	s := st.get()
	cx.SetLocale(s.Language)
	cx.SetAppearance(appearance(s.Appearance))
	go func() {
		found := launchers.Detect()
		cx.Post(func() {
			a.installed = found
			a.rev.Update(func(n int) int { return n + 1 })
		})
	}()
	return a
}

// change edits the settings, saving them, and has what shows them follow.
func (a *app) change(fn func(s *settings)) {
	a.st.update(fn)
	a.aw.follow(a.st.get())
	a.rev.Update(func(n int) int { return n + 1 })
}

// get reads the settings in a binding, which follows their changes.
func (a *app) get() settings {
	a.rev.Get()
	return a.st.get()
}

func (a *app) msgs() locales.Messages { return locales.For(a.cx) }

func appearance(value string) core.Appearance {
	switch value {
	case "light":
		return core.AppearanceLight
	case "dark":
		return core.AppearanceDark
	}
	return core.AppearanceSystem
}

func appearanceValue(a core.Appearance) string {
	switch a {
	case core.AppearanceLight:
		return "light"
	case core.AppearanceDark:
		return "dark"
	}
	return ""
}

// setAppearance keeps the appearance the window's menu chose.
func (a *app) setAppearance(ap core.Appearance) {
	a.change(func(s *settings) { s.Appearance = appearanceValue(ap) })
}

func (a *app) altShortcuts() bool { return a.get().ShortcutModifier == altShortcut }

// altShortcut is the ShortcutModifier preference that moves the shortcuts
// from Ctrl to Alt outside macOS.
const altShortcut = "alt"

// settingsKey says what the settings' pages list; their controls bind the
// values they show, so typing keeps its place.
func (a *app) settingsKey() string {
	s := a.get()
	var b strings.Builder
	fmt.Fprint(&b, len(a.installed), len(s.Providers), a.namer.lastFailure(), ";")
	for _, p := range s.Providers {
		fmt.Fprint(&b, p.ID, len(p.Models), ";")
	}
	return b.String()
}

// settingsCategories group the pages under the application, its tasks and
// the model services, with privacy and updates last; own are chatview's
// agents and updates.
func (a *app) settingsCategories(own []ui.SettingsCategory) []ui.SettingsCategory {
	m := a.msgs()
	general := ui.SettingsCategory{
		ID: "general", Title: m.General(), Icon: lucide.Settings,
		Groups: []ui.SettingsGroup{{
			ID: "general", Items: []ui.SettingsItem{{
				ID: "language", Title: m.Language(),
				Keywords: []string{"language", "English", "中文", "语言", m.System()},
				Control:  a.selectChoice(func(s *settings) *string { return &s.Language }, m.LanguagePreference(), []string{"", "en", "zh-Hans"}, a.choiceName),
			}, {
				ID: "theme", Title: m.Theme(),
				Keywords: []string{"appearance", "theme", "system", "light", "dark", "外观", "主题", "浅色", "深色", m.System()},
				Control:  a.selectChoice(func(s *settings) *string { return &s.Appearance }, m.ThemePreference(), []string{"", "light", "dark"}, a.choiceName),
			}},
		}},
	}
	if runtime.GOOS != "darwin" {
		general.Groups = append(general.Groups, ui.SettingsGroup{
			ID: "keyboard", Title: m.Keyboard(), Items: []ui.SettingsItem{{
				ID: "shortcut", Title: m.ShortcutModifier(), Description: m.ShortcutModifierDescription(),
				Keywords: []string{"keyboard", "shortcut", "Ctrl", "Alt", "Command", "键盘", "快捷键"},
				Control: a.selectChoice(func(s *settings) *string { return &s.ShortcutModifier }, m.ShortcutModifier(), []string{"", altShortcut}, func(v string) string {
					if v == altShortcut {
						return m.ShortcutAlt()
					}
					return m.ShortcutCtrl()
				}),
			}},
		})
	}
	power := ui.SettingsCategory{
		ID: "power", Title: m.Power(), Icon: lucide.Zap,
		Groups: []ui.SettingsGroup{{
			ID: "power", Items: []ui.SettingsItem{{
				ID: "keep-awake", Title: m.KeepAwake(), Description: m.KeepAwakeDescription(),
				Keywords: []string{"sleep", "awake", "power", "energy", "休眠", "睡眠", "电源", "唤醒"},
				Control:  a.switchSetting(func(s *settings) *bool { return &s.KeepAwake }, m.KeepAwake()),
			}, {
				ID: "keep-display-awake", Title: m.KeepDisplayAwake(), Description: m.KeepDisplayAwakeDescription(),
				Keywords: []string{"lock", "screen", "display", "screensaver", "power", "锁屏", "屏幕", "显示器", "屏保", "电源"},
				Control:  a.switchSetting(func(s *settings) *bool { return &s.KeepDisplayAwake }, m.KeepDisplayAwake()),
			}},
		}},
	}
	alerts := ui.SettingsCategory{
		ID: "alerts", Title: m.Alerts(), Icon: lucide.Bell,
		Groups: []ui.SettingsGroup{{
			ID: "alerts", Items: []ui.SettingsItem{{
				ID: "sound", Title: m.TaskSound(), Description: m.TaskSoundDescription(),
				Keywords: []string{"sound", "audio", "alert", "finish", "preview", "声音", "音效", "提醒", "完成", "试听"},
				Control:  a.soundSetting(),
			}, {
				ID: "notification", Title: m.TaskNotification(),
				Keywords: []string{"notification", "alert", "finish", "通知", "提醒", "完成"},
				Control:  a.notificationSetting(),
			}},
		}},
	}
	tools := ui.SettingsCategory{
		ID: "tools", Title: m.Tools(), Icon: lucide.Wrench,
		Groups: []ui.SettingsGroup{{
			ID: "launchers", Items: []ui.SettingsItem{{
				ID: "terminal", Title: m.Terminal(),
				Keywords: []string{"terminal", "shell", "console", "终端", "命令行"},
				Control:  a.launcherSetting(func(s *settings) *string { return &s.Terminal }, launchers.Terminal, m.Terminal()),
			}, {
				ID: "editor", Title: m.Editor(),
				Keywords: []string{"editor", "IDE", "code", "development", "编辑器", "开发工具", "代码"},
				Control:  a.launcherSetting(func(s *settings) *string { return &s.Editor }, launchers.Editor, m.Editor()),
			}},
		}},
	}
	// Git clients are macOS applications.
	if runtime.GOOS == "darwin" {
		group := &tools.Groups[0]
		group.Items = append(group.Items, ui.SettingsItem{
			ID: "git-tool", Title: m.GitTool(),
			Keywords: []string{"git", "Fork", "client", "repository", "仓库", "客户端"},
			Control:  a.launcherSetting(func(s *settings) *string { return &s.GitTool }, launchers.Git, m.GitTool()),
		})
	}
	// The application's own pages, then those of its tasks: the agents
	// they run, how they are named, how they alert and the tools they open
	// in.
	application := []ui.SettingsCategory{general, power}
	tasks := []ui.SettingsCategory{own[0], a.namingSettings(), alerts, tools}
	for i := range application {
		application[i].Section = m.Application()
	}
	for i := range tasks {
		tasks[i].Section = m.Tasks()
	}
	categories := append(application, tasks...)
	categories = append(categories, a.providerCategories()...)
	return append(append(categories, a.statsSettings()), own[1:]...)
}

// sectionActions add a model service from its section's heading.
func (a *app) sectionActions(cx *core.Scope, section string) core.Node {
	if section == a.msgs().ModelServices() {
		return a.addProviderButton(cx)
	}
	return nil
}

// choiceName names a language or appearance choice.
func (a *app) choiceName(value string) string {
	m := a.msgs()
	switch value {
	case "":
		return m.System()
	case "light":
		return m.Light()
	case "dark":
		return m.Dark()
	case "en":
		return m.En()
	case "zh-Hans":
		return m.ZhHans()
	}
	return value
}

// selectChoice picks one of choices into the setting field returns,
// applying the language and appearance at once.
func (a *app) selectChoice(field func(*settings) *string, label string, choices []string, name func(string) string) ui.Select[string] {
	options := make([]ui.Option[string], len(choices))
	for i, choice := range choices {
		options[i] = ui.Option[string]{Value: choice, Label: name(choice)}
	}
	return ui.Select[string]{Options: options, Label: label, Style: core.Style{Width: core.Pt(160), MaxWidth: core.Pct(100)},
		Value: func() string { s := a.get(); return *field(&s) },
		OnChange: func(v string) {
			a.change(func(s *settings) { *field(s) = v })
			s := a.st.get()
			a.cx.SetLocale(s.Language)
			a.cx.SetAppearance(appearance(s.Appearance))
		}}
}

// switchSetting edits a boolean setting, saving it on every change.
func (a *app) switchSetting(field func(*settings) *bool, label string) core.Node {
	return ui.Switch{Label: label, Checked: func() bool { s := a.get(); return *field(&s) }, OnChange: func(on bool) {
		a.change(func(s *settings) { *field(s) = on })
	}}
}

// notificationSetting turns the notifications on, asking the system to let
// the app show them as the user does.
func (a *app) notificationSetting() core.Node {
	return ui.Switch{Label: a.msgs().TaskNotification(), Checked: func() bool { return a.get().TaskNotification }, OnChange: func(on bool) {
		a.change(func(s *settings) { s.TaskNotification = on })
		if on {
			a.cx.RequestNotificationPermission(nil)
		}
	}}
}

// soundSetting chooses the sound played when an agent stops. Each sound's
// row has a button playing it, to hear it before choosing.
func (a *app) soundSetting() core.Node {
	m := a.msgs()
	name := func(id string) string {
		if id == "" {
			return m.SoundNone()
		}
		return a.soundName(id)
	}
	choices := append([]string{""}, taskSoundIDs()...)
	field := a.selectChoice(func(s *settings) *string { return &s.TaskSound }, m.TaskSound(), choices, name)
	for i, id := range choices {
		if id != "" {
			field.Options[i].Trailing = ui.IconButton{Icon: lucide.Play, Label: m.TaskSoundPreview(name(id)),
				Size: 24, OnPress: func() { playTaskSound(id, false) }}
		}
	}
	return field
}

func (a *app) soundName(id string) string {
	m := a.msgs()
	switch id {
	case "chime":
		return m.SoundChime()
	case "glass":
		return m.SoundGlass()
	case "marimba":
		return m.SoundMarimba()
	case "pop":
		return m.SoundPop()
	case "digital":
		return m.SoundDigital()
	}
	return id
}

// textInput is a settings field holding what the user types, which it
// reports: normalizing may change what is saved while the user is typing.
func textInput(initial, label, placeholder string, secure bool, apply func(string)) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		value := core.NewSignal(cx, initial)
		s := core.Style{Width: core.Pt(256), MaxWidth: core.Pct(100), FlexShrink: 1, MinWidth: core.Pt(0)}
		return ui.Input{Value: value.Get, Label: label, Placeholder: placeholder, Masked: secure, Style: s,
			OnChange: func(text string) {
				value.Set(text)
				apply(text)
			}}
	})
}

// newID is a stable local identity.
func newID() string {
	var b [12]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// launcherSetting offers automatic choice, the installed launchers of kind
// and hiding the button. A saved launcher that is not installed stays listed.
func (a *app) launcherSetting(field func(*settings) *string, kind, label string) core.Node {
	s := a.st.get()
	value := *field(&s)
	choices := []string{""}
	for _, l := range a.installed {
		if l.Kind == kind {
			choices = append(choices, l.ID)
		}
	}
	if value != "" && value != launchers.None && !slices.Contains(choices, value) {
		choices = append(choices, value)
	}
	choices = append(choices, launchers.None)
	f := a.selectChoice(field, label, choices, func(id string) string { return a.launcherName(kind, id) })
	f.Style.Width = core.Pt(188)
	for i, id := range choices {
		f.Options[i].Leading = a.launcherIcon(kind, id)
	}
	return f
}

// launcherIcon is what stands for a choice, nil for hiding the button.
func (a *app) launcherIcon(kind, id string) core.Node {
	if id == launchers.None {
		return nil
	}
	if l := launchers.Resolve(a.installed, kind, id); l != nil && l.Icon != nil && (id == "" || l.ID == id) {
		return core.Image{Source: core.ImageSource{Data: l.Icon}, Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), FlexShrink: 0}}
	}
	return core.Func(func(cx *core.Scope) core.Node { return ui.Icon(launcherGlyph(kind), 14, ui.ColorsOf(cx).Muted) })
}

// launcherName labels a choice; automatic names the launcher it resolves to.
func (a *app) launcherName(kind, id string) string {
	m := a.msgs()
	switch id {
	case launchers.None:
		return m.HideLauncher()
	case "":
		if l := launchers.Resolve(a.installed, kind, ""); l != nil {
			return m.AutomaticLauncher(l.Name)
		}
		return m.Automatic()
	}
	if l := launchers.Resolve(a.installed, kind, id); l != nil && l.ID == id {
		return l.Name
	}
	if l, ok := launchers.ByID(id); ok {
		return m.LauncherMissing(l.Name)
	}
	return id
}
