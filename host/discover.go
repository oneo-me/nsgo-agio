package host

import (
	"context"
	"errors"
	"path/filepath"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/agent/claude"
	"github.com/oneo-me/nsgo-agio/agent/codebuddy"
	"github.com/oneo-me/nsgo-agio/agent/codex"
	"github.com/oneo-me/nsgo-agio/agent/pi"
	"github.com/oneo-me/nsgo-agio/shellenv"
)

// toolInfo is what looking for one tool found. A located tool keeps its
// driver even when its catalog failed, so saved conversations stay
// readable.
type toolInfo struct {
	driver  agent.Driver
	path    string
	version string
	catalog agent.Catalog
	err     error
}

func (t *toolInfo) ready() bool { return t != nil && t.err == nil && t.driver != nil }

func (t *toolInfo) state(tool agent.Tool) agio.Tool {
	s := agio.Tool{Tool: tool, Path: t.path, Version: t.version, Catalog: t.catalog}
	if t.err != nil {
		s.Err = t.err.Error()
	}
	return s
}

var (
	errUnsupportedPi = errors.New("pi 1.0 or later is required")
	errNoModels      = errors.New("no models are configured")
)

// Discover implements agio.Backend: it reads the login environment once,
// then looks for each tool and reads its models in parallel; each tool is
// usable as soon as its own check ends.
func (h *Host) Discover() {
	h.mu.Lock()
	if h.discovering > 0 || h.closed {
		h.mu.Unlock()
		return
	}
	h.discovering = len(h.opts.Tools)
	h.mu.Unlock()
	h.changed()
	go func() {
		env := h.opts.Env
		if env == nil {
			env = shellenv.Load()
		}
		for _, tool := range h.opts.Tools {
			go func() {
				info := h.discoverTool(tool, env)
				h.post(func() {
					h.tools[tool] = info
					h.discovering--
					// Saved conversations of a tool become readable once it is located.
					for _, c := range h.conversations {
						h.loadHistory(c)
					}
				})
			}()
		}
	}()
}

// discoverTool locates a tool on the login PATH and reads its catalog.
func (h *Host) discoverTool(tool agent.Tool, env []string) *toolInfo {
	info := &toolInfo{}
	ctx := context.Background()
	if d := h.opts.Drivers[tool]; d != nil {
		info.driver = d
	} else {
		switch tool {
		case agent.Pi:
			command, err := pi.Locate(env)
			if err != nil {
				info.err = err
				return info
			}
			info.driver, info.path = &pi.Driver{Command: command}, command.Path
			if n := len(command.Args); n > 0 {
				info.path = command.Args[n-1] // run by Bun
			}
			if info.version, info.err = command.Version(ctx); info.err == nil && !pi.Supported(info.version) {
				info.err = errUnsupportedPi
			}
		case agent.Codex:
			d, err := codex.Locate(env)
			if err != nil {
				info.err = err
				return info
			}
			// The version is only shown, so failing to read it is not an error.
			info.driver, info.path = d, d.Path
			info.version, _ = d.Version(ctx)
		case agent.Claude:
			d, err := claude.Locate(env)
			if err != nil {
				info.err = err
				return info
			}
			info.driver, info.path = d, d.Path
			info.version, _ = d.Version(ctx)
		case agent.CodeBuddy:
			d, err := codebuddy.Locate(env)
			if err != nil {
				info.err = err
				return info
			}
			info.driver, info.path = d, d.Script()
			info.version, _ = d.Version(ctx)
		default:
			info.err = errors.New("unknown tool " + string(tool))
			return info
		}
	}
	info.path = homeRelative(info.path, h.opts.Home)
	if info.err == nil {
		info.catalog, info.err = info.driver.Catalog(ctx, h.opts.Home)
	}
	if info.err == nil && len(info.catalog.Models) == 0 {
		info.err = errNoModels
	}
	return info
}

// homeRelative abbreviates the home directory of path to ~.
func homeRelative(path, home string) string {
	if rel, err := filepath.Rel(home, path); home != "" && path != "" && err == nil && filepath.IsLocal(rel) {
		return filepath.Join("~", rel)
	}
	return path
}

// driver returns a usable tool's driver.
func (h *Host) driver(tool agent.Tool) agent.Driver {
	if t := h.tools[tool]; t.ready() {
		return t.driver
	}
	return nil
}

// historyDriver can read a tool's saved conversations. pi and Claude Code
// keep them in files that need no executable; Codex and ACP agents replay
// them through their own process once located.
func (h *Host) historyDriver(tool agent.Tool) agent.Driver {
	if t := h.tools[tool]; t != nil && t.driver != nil {
		return t.driver
	}
	return nil
}

// model returns a usable model by key.
func (h *Host) model(key string) (agent.Model, bool) {
	tool, id := agent.ParseKey(key)
	if t := h.tools[tool]; t.ready() {
		for _, m := range t.catalog.Models {
			if m.ID == id {
				return m, true
			}
		}
	}
	return agent.Model{}, false
}

// defaultModel is the configured default of the first usable tool, else its
// first model, with the tool's default level.
func (h *Host) defaultModel() (string, string) {
	for _, tool := range h.opts.Tools {
		t := h.tools[tool]
		if !t.ready() {
			continue
		}
		if t.catalog.Model != "" {
			if _, ok := h.model(agent.Key(tool, t.catalog.Model)); ok {
				return agent.Key(tool, t.catalog.Model), t.catalog.Effort
			}
		}
		return t.catalog.Models[0].Key(), t.catalog.Effort
	}
	return "", ""
}
