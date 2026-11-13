package main

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/host"
	"github.com/oneo-me/nsgo-agio/internal/fakeagent"
)

func TestTasksAndProjectsAreKeptAndResumed(t *testing.T) {
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	saved := state{Projects: []string{b, a}, Tasks: []savedTask{
		{Title: "newer", Dir: a, Model: "claude:opus", Cursor: "s2", Created: created.Add(time.Hour)},
		{Title: "gone", Dir: filepath.Join(a, "missing"), Model: "claude:opus", Cursor: "s0"},
		{Title: "older", Dir: b, Model: "claude:opus", Cursor: "s1", Created: created},
	}}
	h := host.New(host.Options{Env: []string{}, Tools: []agent.Tool{agent.Claude},
		Drivers: map[agent.Tool]agent.Driver{agent.Claude: &fakeagent.Driver{}}, Home: t.TempDir()})
	t.Cleanup(func() { h.Close() })
	kept := restoreTasks(h, saved.Tasks)
	if len(kept) != 1 || kept[0].Title != "gone" {
		t.Fatalf("kept = %+v", kept)
	}
	s := h.State()
	if len(s.Conversations) != 2 || s.Conversations[0].Title != "newer" || !s.Conversations[1].Created.Equal(created) {
		t.Fatalf("resumed = %+v", s.Conversations)
	}
	// New task's draft in a new folder makes it the one last used.
	if _, err := h.Create(agio.Create{Dir: c}); err != nil {
		t.Fatal(err)
	}
	noteTasks(&saved, h.State(), kept)
	if !slices.Equal(saved.Projects, []string{c, b, a}) {
		t.Fatalf("projects = %v", saved.Projects)
	}
	var titles []string
	for _, task := range saved.Tasks {
		titles = append(titles, task.Title)
	}
	if !slices.Equal(titles, []string{"newer", "older", "gone"}) {
		t.Fatalf("tasks = %v", titles)
	}
}

func TestStateNormalizeDropsWhatCannotBeShown(t *testing.T) {
	dir := t.TempDir()
	s := state{Window: windowState{X: float32(math.NaN()), Width: 900, Height: 800}, SidebarWidth: 300,
		Projects: []string{dir, filepath.Join(dir, "missing")}}
	s.normalize()
	if s.Window != (windowState{}) || s.SidebarWidth != 0 || !slices.Equal(s.Projects, []string{dir}) {
		t.Fatalf("normalized = %+v", s)
	}
	if (windowState{}).frame() != nil {
		t.Fatal("an unknown window has a frame")
	}
}

func TestWindowKeepsItsFrameWhileMaximized(t *testing.T) {
	var s state
	shown := shownWindow{sidebarOpen: true, sidebarWidth: 260}
	shown.frame.Width, shown.frame.Height, shown.frame.X, shown.frame.Move = 1000, 800, 40, true
	shown.note(&s)
	shown.maximized, shown.frame.Width = true, 1600
	shown.note(&s)
	if s.Window.Width != 1000 || !s.Window.Maximized || !s.Window.Positioned || s.SidebarWidth != 260 || s.SidebarHidden {
		t.Fatalf("state = %+v", s)
	}
}

func TestStoreMovesTheJSONOfEarlierVersionsToYAML(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"disabledModels":["claude:haiku"],"language":"zh-Hans","installID":"abc",` +
		`"providers":[{"id":"p","baseURL":"https://example.com","apiKey":"key"}]}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	st := loadStore[settings](dir, "settings")
	s := st.get()
	if s.Language != "zh-Hans" || s.InstallID != "abc" || len(s.DisabledModels) != 1 || len(s.Providers) != 1 || s.Providers[0].APIKey != "key" {
		t.Fatalf("settings = %+v", s)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("the JSON file stayed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.yaml"))
	if err != nil || !strings.Contains(string(data), "base_url: https://example.com") {
		t.Fatalf("settings.yaml = %q, %v", data, err)
	}
	st.update(func(s *settings) { s.Language = "en" })
	if got := loadStore[settings](dir, "settings").get(); got.Language != "en" || got.Providers[0].BaseURL != "https://example.com" {
		t.Fatalf("read back = %+v", got)
	}
}

func TestStateRoundTripsThroughYAML(t *testing.T) {
	dir := t.TempDir()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	want := state{Window: windowState{X: 10, Width: 900, Height: 800, Positioned: true}, SidebarWidth: 240,
		Projects: []string{dir}, Tasks: []savedTask{{Title: "t", Dir: dir, Model: "claude:opus", Cursor: "s", Created: created}}}
	loadStore[state](dir, "state").update(func(s *state) { *s = want })
	got := loadStore[state](dir, "state").get()
	if got.Window != want.Window || got.SidebarWidth != 240 || len(got.Tasks) != 1 || !got.Tasks[0].Created.Equal(created) || !got.Tasks[0].Updated.IsZero() {
		t.Fatalf("state = %+v", got)
	}
}
