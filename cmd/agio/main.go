// Command agio is the Agio app: a whole agent application in one process,
// the window and the coding agents it runs, which work in the folder a new
// conversation picks, else in AGIO_DIR or the home directory. It keeps the
// agents and models turned off and the statistics' choice in
// Agio/settings.json under the user's configuration directory, shows the
// signed-in accounts' limits and reports its use (stats.go). Run it with
// go tool nsgo dev cmd/agio.
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/chatview"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/quota"
)

// settings are what the application keeps between runs.
type settings struct {
	Disabled       []agent.Tool `json:"disabled,omitempty"`
	DisabledModels []string     `json:"disabledModels,omitempty"`
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
	err := os.MkdirAll(filepath.Dir(st.path), 0o755)
	if err == nil {
		err = os.WriteFile(st.path, after, 0o644)
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
	// The agents and models turned on or off in the settings are kept for
	// next time.
	stop := h.Watch(func() {
		var disabled []agent.Tool
		var models []string
		for _, t := range h.State().Tools {
			if t.Disabled {
				disabled = append(disabled, t.Tool)
			}
			for _, id := range t.DisabledModels {
				models = append(models, agent.Key(t.Tool, id))
			}
		}
		st.update(func(s *settings) { s.Disabled, s.DisabledModels = disabled, models })
	})
	nsgo.Run(func(cx *core.Scope) core.Node {
		reportUsage(cx, h, st)
		return chatview.App{Model: chatview.Bind(cx, h), Dir: dir, Name: agent.Client.Title,
			Settings: []ui.SettingsCategory{statsSettings(cx, st)}}
	}, nsgo.WindowSize(1040, 720))
	// The window closed: its agents end with it.
	stop()
	h.Close()
}
