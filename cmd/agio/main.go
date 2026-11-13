// Command agio is the Agio app: a whole agent application in one process,
// the window and the coding agents it runs, which work in the folder a new
// conversation picks, else in AGIO_DIR or the home directory. It keeps its
// preferences (prefs.go), the agents and models turned off and the
// statistics' choice in Agio/settings.json under the user's configuration
// directory, shows the signed-in accounts' limits and reports its use
// (stats.go). Run it with go tool nsgo dev cmd/agio.
package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/chatview"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/quota"
)

// heroIcon is the app's icon heading New task, at the size it shows.
//
//go:embed assets/Hero.png
var heroIcon []byte

// settings are what the application keeps between runs. Empty choices
// follow the system or pick automatically.
type settings struct {
	Disabled       []agent.Tool `json:"disabled,omitempty"`
	DisabledModels []string     `json:"disabledModels,omitempty"`
	// Language is a language tag and Appearance "light" or "dark";
	// ShortcutModifier "alt" moves the shortcuts outside macOS to Alt.
	Language         string `json:"language,omitempty"`
	Appearance       string `json:"appearance,omitempty"`
	ShortcutModifier string `json:"shortcutModifier,omitempty"`
	KeepAwake        bool   `json:"keepAwake,omitempty"`
	KeepDisplayAwake bool   `json:"keepDisplayAwake,omitempty"`
	// TaskSound names the sound played when an agent stops, empty for none.
	TaskSound        string `json:"taskSound,omitempty"`
	TaskNotification bool   `json:"taskNotification,omitempty"`
	// Terminal, Editor and GitTool are launchers' IDs, or launchers.None.
	Terminal  string          `json:"terminal,omitempty"`
	Editor    string          `json:"editor,omitempty"`
	GitTool   string          `json:"gitTool,omitempty"`
	Providers []modelProvider `json:"providers,omitempty"`
	// InstallID is the install's in the usage statistics; StatsOff is set
	// once the settings turned them off.
	InstallID string `json:"installID,omitempty"`
	StatsOff  bool   `json:"statsOff,omitempty"`
}

// store reads and writes settings in one file, from any goroutine.
type store struct {
	path string
	mu   sync.Mutex
	s    settings
}

func openStore() *store {
	st := &store{}
	if dir, err := os.UserConfigDir(); err == nil {
		st.path = filepath.Join(dir, "Agio", "settings.json")
		if data, err := os.ReadFile(st.path); err == nil {
			if err := json.Unmarshal(data, &st.s); err != nil {
				log.Printf("read %s: %v", st.path, err)
			}
		}
	}
	return st
}

func (st *store) get() settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s
}

// update changes the settings and saves them when they changed.
func (st *store) update(fn func(s *settings)) {
	st.mu.Lock()
	defer st.mu.Unlock()
	before, _ := json.Marshal(st.s)
	fn(&st.s)
	after, _ := json.Marshal(st.s)
	if st.path == "" || string(before) == string(after) {
		return
	}
	// The file holds the providers' API keys.
	err := os.MkdirAll(filepath.Dir(st.path), 0o755)
	if err == nil {
		err = os.WriteFile(st.path, after, 0o600)
	}
	if err != nil {
		log.Printf("save %s: %v", st.path, err)
	}
}

func main() {
	home, _ := os.UserHomeDir()
	dir := os.Getenv("AGIO_DIR")
	if dir == "" {
		dir = home
	}
	st := openStore()
	h := host.New(host.Options{Dir: dir, Disabled: st.get().Disabled, DisabledModels: st.get().DisabledModels,
		ReadQuota: quota.Reader(home)})
	var lang atomic.Value
	lang.Store("en")
	al, aw := &alerts{}, &awake{}
	// The agents and models turned on or off in the settings are kept for
	// next time; an agent stopping alerts, and one working keeps the
	// computer awake as the settings ask.
	stop := h.Watch(func() {
		s := h.State()
		var disabled []agent.Tool
		var models []string
		for _, t := range s.Tools {
			if t.Disabled {
				disabled = append(disabled, t.Tool)
			}
			for _, id := range t.DisabledModels {
				models = append(models, agent.Key(t.Tool, id))
			}
		}
		st.update(func(s *settings) { s.Disabled, s.DisabledModels = disabled, models })
		working := false
		for _, c := range s.Conversations {
			working = working || c.Running
		}
		al.look(st.get(), s.Conversations, lang.Load().(string))
		aw.update(st.get(), working)
	})
	nsgo.Run(func(cx *core.Scope) core.Node {
		reportUsage(cx, h, st)
		a := newApp(cx, st, aw)
		cx.Effect(func() { lang.Store(i18n.Language(cx)) })
		return chatview.App{Model: chatview.Bind(cx, h), Dir: dir, Name: agent.Client.Title, Updates: true, Icon: heroIcon,
			Header: a.launchButtons, Settings: a.settingsCategories, SettingsKey: a.settingsKey,
			SettingsSectionActions: a.sectionActions, SettingsCategory: a.category,
			OnAppearanceChange: a.setAppearance, AltShortcuts: a.altShortcuts}
	}, nsgo.WindowSize(1040, minWindowHeight),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: minWindowWidth, MinHeight: minWindowHeight}))
	// The window closed: its agents end with it.
	stop()
	aw.stop()
	h.Close()
}

// The smallest the window may be.
const minWindowWidth, minWindowHeight = 820, 740
