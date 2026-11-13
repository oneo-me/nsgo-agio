// Command agio is the Agio app: a whole agent application in one process,
// the window and the coding agents it runs, which work in the folder a new
// conversation picks, else the project last used, else AGIO_DIR or the home
// directory. It keeps its preferences (prefs.go), the agents and models
// turned off and the statistics' choice in Agio/settings.yaml under the
// user's configuration directory, and its window, projects and tasks in
// Agio/state.yaml beside it (state.go). It shows the signed-in accounts'
// limits and reports its use (stats.go). Run it with go tool nsgo dev
// cmd/agio.
package main

import (
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
	"go.yaml.in/yaml/v3"
)

// settings are what the application keeps between runs. Empty choices
// follow the system or pick automatically.
type settings struct {
	Disabled       []agent.Tool `yaml:"disabled,omitempty"`
	DisabledModels []string     `yaml:"disabled_models,omitempty"`
	// Language is a language tag and Appearance "light" or "dark";
	// ShortcutModifier "alt" moves the shortcuts outside macOS to Alt.
	Language   string `yaml:"language,omitempty"`
	Appearance string `yaml:"appearance,omitempty"`
	// LightScheme and DarkScheme are the color schemes' IDs (themes.go)
	// shown in each appearance, empty for the default.
	LightScheme string `yaml:"light_scheme,omitempty"`
	DarkScheme  string `yaml:"dark_scheme,omitempty"`
	// Grain scatters a film grain over the window's chrome, GrainAmount
	// strong from 1 to 100, empty for the default.
	Grain            bool   `yaml:"grain,omitempty"`
	GrainAmount      int    `yaml:"grain_amount,omitempty"`
	ShortcutModifier string `yaml:"shortcut_modifier,omitempty"`
	KeepAwake        bool   `yaml:"keep_awake,omitempty"`
	KeepDisplayAwake bool   `yaml:"keep_display_awake,omitempty"`
	// TaskSound names the sound played when an agent stops, empty for none.
	TaskSound        string `yaml:"task_sound,omitempty"`
	TaskNotification bool   `yaml:"task_notification,omitempty"`
	// Terminal, Editor and GitTool are launchers' IDs, or launchers.None.
	Terminal  string          `yaml:"terminal,omitempty"`
	Editor    string          `yaml:"editor,omitempty"`
	GitTool   string          `yaml:"git_tool,omitempty"`
	Providers []modelProvider `yaml:"providers,omitempty"`
	// NamingModel is the providers' model titling new tasks, none to keep
	// their first message; TitleLanguage "en" or "zh-Hans" is the titles'
	// language, empty for the interface's.
	NamingModel   modelReference `yaml:"naming_model,omitempty"`
	TitleLanguage string         `yaml:"title_language,omitempty"`
	// InstallID is the install's in the usage statistics; StatsOff is set
	// once the settings turned them off.
	InstallID string `yaml:"install_id,omitempty"`
	StatsOff  bool   `yaml:"stats_off,omitempty"`
}

// store reads and writes a value in one YAML file, from any goroutine.
type store[T any] struct {
	path string
	mu   sync.Mutex
	s    T
}

// openStore reads name.yaml in Agio's directory under the user's
// configuration directory, keeping nothing when there is none.
func openStore[T any](name string) *store[T] {
	dir, err := os.UserConfigDir()
	if err != nil {
		return &store[T]{}
	}
	return loadStore[T](filepath.Join(dir, "Agio"), name)
}

// loadStore reads name.yaml in dir. Without it, the name.json of earlier
// versions is read and moved to it.
func loadStore[T any](dir, name string) *store[T] {
	st := &store[T]{path: filepath.Join(dir, name+".yaml")}
	data, err := os.ReadFile(st.path)
	if err == nil {
		if err := yaml.Unmarshal(data, &st.s); err != nil {
			log.Printf("read %s: %v", st.path, err)
		}
		return st
	}
	legacy := filepath.Join(dir, name+".json")
	if data, err = os.ReadFile(legacy); err != nil {
		return st
	}
	// The JSON keys match the fields' names, as encoding/json ignores case.
	if err := json.Unmarshal(data, &st.s); err != nil {
		log.Printf("read %s: %v", legacy, err)
		return st
	}
	if st.save() {
		os.Remove(legacy)
	}
	return st
}

func (st *store[T]) get() T {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s
}

// update changes the value and saves it when it changed.
func (st *store[T]) update(fn func(s *T)) {
	st.mu.Lock()
	defer st.mu.Unlock()
	before, _ := yaml.Marshal(st.s)
	fn(&st.s)
	after, _ := yaml.Marshal(st.s)
	if string(before) != string(after) {
		st.save()
	}
}

// save writes the value, reporting whether it did.
func (st *store[T]) save() bool {
	if st.path == "" {
		return false
	}
	data, err := yaml.Marshal(st.s)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(st.path), 0o755)
	}
	// The settings hold the providers' API keys.
	if err == nil {
		err = os.WriteFile(st.path, data, 0o600)
	}
	if err != nil {
		log.Printf("save %s: %v", st.path, err)
	}
	return err == nil
}

func main() {
	home, _ := os.UserHomeDir()
	dir := os.Getenv("AGIO_DIR")
	if dir == "" {
		dir = home
	}
	st := openStore[settings]("settings")
	ws := openStore[state]("state")
	ws.update((*state).normalize)
	h := host.New(host.Options{Dir: dir, Disabled: st.get().Disabled, DisabledModels: st.get().DisabledModels,
		ReadQuota: quota.Reader(home)})
	// Tasks resume before their list is followed, which would drop those
	// not resumed yet.
	kept := restoreTasks(h, ws.get().Tasks)
	var lang atomic.Value
	lang.Store("en")
	al, aw := &alerts{}, &awake{}
	nm := newNamer(h, st, func() string { return lang.Load().(string) })
	// The agents and models turned on or off in the settings, the tasks and
	// their projects are kept for next time; an agent stopping alerts, and
	// one working keeps the computer awake as the settings ask, and new
	// tasks are named.
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
		ws.update(func(w *state) { noteTasks(w, s, kept) })
		working := false
		for _, c := range s.Conversations {
			working = working || c.Running
		}
		al.look(st.get(), s.Conversations, lang.Load().(string))
		aw.update(st.get(), working)
		nm.look(s)
	})
	options := []nsgo.Option{nsgo.WindowSize(1040, minWindowHeight), nsgo.WindowMaterial(windowMaterial()),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: minWindowWidth, MinHeight: minWindowHeight})}
	if f := ws.get().Window.frame(); f != nil {
		options = append(options, nsgo.WindowFrame(*f))
	}
	var window *windowKeeper
	nsgo.Run(func(cx *core.Scope) core.Node {
		reportUsage(cx, h, st)
		a := newApp(cx, st, aw, nm)
		window = keepWindow(cx, ws)
		cx.Effect(func() { lang.Store(i18n.Language(cx)) })
		// Pressing an agent's notification shows its conversation.
		selected := core.NewSignal(cx, "")
		al.attach(cx, selected, func(id string) bool { return h.State().Conversation(id) != nil })
		return chatview.App{Model: chatview.Bind(cx, h), Dir: dir, Name: agent.Client.Title, Updates: true, Icon: logo{},
			Projects: func() []string { return ws.get().Projects }, SidebarOpen: window.open, SidebarWidth: window.width, Selected: selected,
			Header: a.launchButtons, Settings: a.settingsCategories, SettingsKey: a.settingsKey,
			SettingsSectionActions: a.sectionActions, SettingsCategory: a.category,
			OnAppearanceChange: a.setAppearance, AltShortcuts: a.altShortcuts, ChromeStyle: a.chrome, Backdrop: a.grain()}
	}, options...)
	// The window closed: its place is kept, and its agents end with it.
	if window != nil {
		window.save()
	}
	stop()
	aw.stop()
	h.Close()
}

// The smallest the window may be.
const minWindowWidth, minWindowHeight = 820, 740
