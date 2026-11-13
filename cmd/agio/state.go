package main

import (
	"log"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/host"
)

// state is what Agio keeps of its window and tasks between runs, apart from
// the settings the user chooses.
type state struct {
	Window        windowState `yaml:"window,omitempty"`
	SidebarHidden bool        `yaml:"sidebar_hidden,omitempty"`
	SidebarWidth  float32     `yaml:"sidebar_width,omitempty"`
	// Projects are the folders tasks worked in, the one last used first,
	// where New task starts.
	Projects []string `yaml:"projects,omitempty"`
	// Tasks are the tasks resumed at start, newest first.
	Tasks []savedTask `yaml:"tasks,omitempty"`
}

// windowState is the window's frame from when it last was neither
// maximized nor in full screen, as desktop.Frame reports it, and whether it
// is one of those now. Positioned is false where the platform gives no
// position.
type windowState struct {
	X          float32 `yaml:"x,omitempty"`
	Y          float32 `yaml:"y,omitempty"`
	Positioned bool    `yaml:"positioned,omitempty"`
	Width      float32 `yaml:"width,omitempty"`
	Height     float32 `yaml:"height,omitempty"`
	Maximized  bool    `yaml:"maximized,omitempty"`
	FullScreen bool    `yaml:"full_screen,omitempty"`
}

// savedTask is what resumes a task: its agent's session and choices.
type savedTask struct {
	Title   string    `yaml:"title,omitempty"`
	Dir     string    `yaml:"dir"`
	Model   string    `yaml:"model"`
	Effort  string    `yaml:"effort,omitempty"`
	Tier    string    `yaml:"tier,omitempty"`
	Cursor  string    `yaml:"cursor"`
	Created time.Time `yaml:"created,omitempty"`
	Updated time.Time `yaml:"updated,omitempty"`
}

// normalize drops what a damaged file or a folder gone since holds.
func (s *state) normalize() {
	w := &s.Window
	for _, v := range []float32{w.X, w.Y, w.Width, w.Height, s.SidebarWidth} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			*w, s.SidebarWidth = windowState{}, 0
			break
		}
	}
	if w.Width <= 0 || w.Height <= 0 {
		w.X, w.Y, w.Positioned, w.Width, w.Height = 0, 0, false, 0, 0
	}
	s.SidebarWidth = max(s.SidebarWidth, 0)
	s.Projects = slices.DeleteFunc(s.Projects, func(d string) bool { return !isDir(d) })
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// frame is what the window opens with, nil to open centered.
func (w windowState) frame() *core.WindowFrame {
	if w.Width <= 0 {
		return nil
	}
	return &core.WindowFrame{X: core.Points(w.X), Y: core.Points(w.Y), Move: w.Positioned,
		Width: core.Points(w.Width), Height: core.Points(w.Height)}
}

// restoreTasks resumes the saved tasks, oldest first so the newest leads,
// and returns those that could not be, such as in a folder not mounted now,
// to keep for next time.
func restoreTasks(h *host.Host, tasks []savedTask) []savedTask {
	var kept []savedTask
	for _, t := range slices.Backward(tasks) {
		_, err := h.Create(agio.Create{Dir: t.Dir, Title: t.Title, Model: t.Model, Effort: t.Effort, Tier: t.Tier,
			Cursor: t.Cursor, Created: t.Created, Updated: t.Updated})
		if err != nil {
			log.Printf("resume %q: %v", t.Title, err)
			kept = append([]savedTask{t}, kept...)
		}
	}
	return kept
}

// noteTasks keeps the tasks whose agents started a session, then kept, and
// the folders they work in. The folder of the task not started yet, New
// task's, is the one last used.
func noteTasks(s *state, b agio.State, kept []savedTask) {
	s.Tasks = nil
	for _, c := range b.Conversations {
		if c.Cursor != "" && c.Model != "" {
			s.Tasks = append(s.Tasks, savedTask{Title: c.Title, Dir: c.Dir, Model: c.Model, Effort: c.Effort, Tier: c.Tier,
				Cursor: c.Cursor, Created: c.Created, Updated: c.Updated})
		}
	}
	s.Tasks = append(s.Tasks, kept...)
	var dirs []string
	for _, c := range b.Conversations {
		if !c.Bound && c.Dir != "" {
			dirs = append(dirs, c.Dir)
			break
		}
	}
	dirs = append(dirs, s.Projects...)
	for _, c := range b.Conversations {
		dirs = append(dirs, c.Dir)
	}
	s.Projects = nil
	for _, d := range dirs {
		if d != "" && !slices.Contains(s.Projects, d) {
			s.Projects = append(s.Projects, d)
		}
	}
}

// windowSettle is how long after the window or its sidebar last changed
// they are saved, which also outlasts macOS's zoom animation.
const windowSettle = 300 * time.Millisecond

// shownWindow is what the window and its sidebar last reported.
type shownWindow struct {
	frame                 core.WindowFrame
	maximized, fullScreen bool
	sidebarOpen           bool
	sidebarWidth          core.Points
}

// note copies what the window reported into s. A maximized or full-screen
// window keeps the frame it returns to.
func (w shownWindow) note(s *state) {
	s.SidebarHidden, s.SidebarWidth = !w.sidebarOpen, float32(w.sidebarWidth)
	// The platform has not reported the window yet.
	if w.frame.Width <= 0 {
		return
	}
	s.Window.Maximized, s.Window.FullScreen = w.maximized && !w.fullScreen, w.fullScreen
	if !w.maximized && !w.fullScreen {
		f := w.frame
		s.Window.X, s.Window.Y, s.Window.Positioned = float32(f.X), float32(f.Y), f.Move
		s.Window.Width, s.Window.Height = float32(f.Width), float32(f.Height)
	}
}

// windowKeeper saves the window's place and its sidebar once they settle,
// and at once when the window closes.
type windowKeeper struct {
	st      *store[state]
	mu      sync.Mutex
	timer   *time.Timer
	pending *shownWindow
	// Open and Width hold the sidebar.
	open  *core.Signal[bool]
	width *core.Signal[core.Points]
}

// keepWindow puts the window back as it was, the frame being
// nsgo.WindowFrame's, and follows it and the sidebar from then on.
func keepWindow(cx *core.Scope, st *store[state]) *windowKeeper {
	s := st.get()
	width := core.Points(s.SidebarWidth)
	if width <= 0 {
		width = 220
	}
	k := &windowKeeper{st: st, open: core.NewSignal(cx, !s.SidebarHidden), width: core.NewSignal(cx, width)}
	switch {
	case s.Window.FullScreen:
		desktop.ToggleFullScreen(cx)
	case s.Window.Maximized:
		desktop.ToggleMaximize(cx)
	}
	cx.Effect(func() {
		shown := shownWindow{desktop.Frame(cx), desktop.Maximized(cx), desktop.FullScreen(cx), k.open.Get(), k.width.Get()}
		k.mu.Lock()
		defer k.mu.Unlock()
		k.pending = &shown
		if k.timer == nil {
			k.timer = time.AfterFunc(windowSettle, k.save)
		} else {
			k.timer.Reset(windowSettle)
		}
	})
	return k
}

// save saves what the window last reported, if not yet saved.
func (k *windowKeeper) save() {
	k.mu.Lock()
	shown := k.pending
	k.pending = nil
	k.mu.Unlock()
	if shown != nil {
		k.st.update(shown.note)
	}
}
