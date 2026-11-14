// Package acp runs agents speaking the Agent Client Protocol, JSON-RPC 2.0
// over stdio, as agent.Drivers. A Spec describes one agent CLI; its own
// package locates the executable. Sessions run in the agent's most
// permissive mode and approve every permission request, as Agio's other
// agents run with full access.
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

// Spec describes an agent CLI speaking ACP.
type Spec struct {
	Tool agent.Tool
	// Provider groups the agent's models.
	Provider string
	// Args start the CLI's ACP server, such as --acp.
	Args []string
	// Modes are mode IDs that skip permission prompts, most permissive
	// first; a session selects the first one the agent offers.
	Modes []string
	// AppendSystemPrompt is the CLI's option appending to its system
	// prompt, such as --append-system-prompt. Without one, sessions ask
	// for it in _meta.systemPrompt.append, as Claude Code's adapter reads.
	AppendSystemPrompt string
	// PluginDir is the CLI's option loading a plugin directory for one
	// session, such as --plugin-dir; sessions give their skills to it as
	// agent.PluginDir lays them out. Without one, agents get no skills.
	PluginDir string
}

// Driver runs an ACP agent CLI as an agent.Driver.
type Driver struct {
	Spec
	// Path runs the CLI after PathArgs, such as Bun running its script.
	Path     string
	PathArgs []string
	Env      []string
}

// Script is the CLI's own executable or script, as shown to the user.
func (d *Driver) Script() string {
	if n := len(d.PathArgs); n > 0 {
		return d.PathArgs[n-1]
	}
	return d.Path
}

func (d *Driver) Tool() agent.Tool { return d.Spec.Tool }

func (d *Driver) name() string { return d.Spec.Tool.Name() }

func (d *Driver) command(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command(d.Path, append(append(append([]string{}, d.PathArgs...), d.Args...), args...)...)
	cmd.Dir, cmd.Env = dir, d.Env
	return cmd
}

// Version runs the CLI with --version and returns the first version-like
// field of its output.
func (d *Driver) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.Path, append(append([]string{}, d.PathArgs...), "--version")...)
	childproc.HideConsole(cmd)
	cmd.Env = d.Env
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	for _, field := range strings.Fields(string(out)) {
		if v := strings.TrimPrefix(field, "v"); v != "" && v[0] >= '0' && v[0] <= '9' {
			return v, nil
		}
	}
	return "", fmt.Errorf("unrecognized %s version %q", d.name(), out)
}

// explain asks for a sign-in the agent needs, which Agio cannot offer.
func (d *Driver) explain(err error) error {
	if authRequired(err) {
		return fmt.Errorf("sign in to %s in a terminal first: %w", d.name(), err)
	}
	return err
}

// maxProbedModels bounds the models whose reasoning levels Catalog reads.
const maxProbedModels = 32

// Catalog opens a session in dir and reads its models. ACP reports the
// reasoning levels of the selected model only, so the probe selects each
// model in turn, then restores the one selected before. The session is
// never prompted.
func (d *Driver) Catalog(ctx context.Context, dir string) (agent.Catalog, error) {
	c, err := dial(d.command(dir), d.name(), nil, nil)
	if err != nil {
		return agent.Catalog{}, err
	}
	defer c.proc.Close(time.Second)
	var st state
	if err := c.callInto(ctx, "session/new", newSession(dir), &st); err != nil {
		return agent.Catalog{}, d.explain(err)
	}
	choices, current := st.models()
	efforts := map[string][]string{current: st.efforts()}
	if model := st.option(categoryModel); model != nil && st.option(categoryThought) != nil {
		selected := current
		for i, m := range choices {
			if m.Value == selected || i >= maxProbedModels {
				continue
			}
			if st.set(ctx, c, model.ID, m.Value) != nil {
				break
			}
			selected = m.Value
			efforts[m.Value] = st.efforts()
		}
		if selected != current {
			st.set(ctx, c, model.ID, current)
		}
	}
	var catalog agent.Catalog
	for _, m := range choices {
		model := agent.Model{Tool: d.Spec.Tool, ID: m.Value, Name: firstNonEmpty(m.Name, m.Value), Provider: d.Provider,
			Detail: m.Description, Images: c.caps.PromptCapabilities.Image, Efforts: efforts[m.Value]}
		model.DefaultEffort = defaultEffort(model.Efforts)
		if m.Value == current {
			model.Default = true
			catalog.Model, catalog.Effort = model.ID, model.DefaultEffort
		}
		catalog.Models = append(catalog.Models, model)
	}
	if catalog.Model == "" && len(catalog.Models) > 0 {
		catalog.Model, catalog.Effort = catalog.Models[0].ID, catalog.Models[0].DefaultEffort
	}
	return catalog, nil
}

// defaultEffort prefers high reasoning, then medium, as for Claude Code.
func defaultEffort(levels []string) string {
	for _, preferred := range []string{"high", "medium"} {
		for _, level := range levels {
			if level == preferred {
				return level
			}
		}
	}
	if len(levels) > 0 {
		return levels[0]
	}
	return ""
}

func newSession(dir string) map[string]any {
	return map[string]any{"cwd": dir, "mcpServers": []any{}}
}

// withSystemPrompt asks the agent to append system to its system prompt.
func withSystemPrompt(params map[string]any, system string) map[string]any {
	if system != "" {
		params["_meta"] = map[string]any{"systemPrompt": map[string]any{"append": system}}
	}
	return params
}

// Session configuration option categories ACP defines.
const (
	categoryMode    = "mode"
	categoryModel   = "model"
	categoryThought = "thought_level"
)

// state is a session's configuration, as session/new and session/load
// return it. Agents report configOptions, or the older modes and models.
type state struct {
	SessionID string `json:"sessionId"`
	Modes     *struct {
		CurrentModeID  string `json:"currentModeId"`
		AvailableModes []struct {
			ID string `json:"id"`
		} `json:"availableModes"`
	} `json:"modes"`
	Models *struct {
		CurrentModelID  string `json:"currentModelId"`
		AvailableModels []struct {
			ModelID     string `json:"modelId"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"availableModels"`
	} `json:"models"`
	ConfigOptions []configOption `json:"configOptions"`
}

// configOption is a session setting the agent offers.
type configOption struct {
	ID           string          `json:"id"`
	Category     string          `json:"category"`
	Type         string          `json:"type"`
	CurrentValue json.RawMessage `json:"currentValue"`
	Options      json.RawMessage `json:"options"`
}

// choice is one value of a select option.
type choice struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// current is the option's value when it is a string.
func (o *configOption) current() string {
	var v string
	json.Unmarshal(o.CurrentValue, &v)
	return v
}

// choices lists a select option's values, flattening groups.
func (o *configOption) choices() []choice {
	var items []struct {
		choice
		Options []choice `json:"options"`
	}
	json.Unmarshal(o.Options, &items)
	var out []choice
	for _, item := range items {
		if item.Value != "" {
			out = append(out, item.choice)
		}
		out = append(out, item.Options...)
	}
	return out
}

func (o *configOption) has(value string) bool {
	for _, c := range o.choices() {
		if c.Value == value {
			return true
		}
	}
	return false
}

// option returns the select option of a category.
func (st *state) option(category string) *configOption {
	for i, o := range st.ConfigOptions {
		if o.Category == category && (o.Type == "" || o.Type == "select") {
			return &st.ConfigOptions[i]
		}
	}
	return nil
}

// models lists the selectable models and the selected one.
func (st *state) models() ([]choice, string) {
	if o := st.option(categoryModel); o != nil {
		return o.choices(), o.current()
	}
	if st.Models == nil {
		return nil, ""
	}
	var out []choice
	for _, m := range st.Models.AvailableModels {
		out = append(out, choice{Value: m.ModelID, Name: m.Name, Description: m.Description})
	}
	return out, st.Models.CurrentModelID
}

// efforts lists the selected model's reasoning levels. Levels turning
// thinking off or deferring to the model's default have no Agio choice.
func (st *state) efforts() []string {
	o := st.option(categoryThought)
	if o == nil {
		return nil
	}
	var levels []string
	for _, c := range o.choices() {
		switch c.Value {
		case "", "disabled", "enabled", "default", "auto":
			continue
		}
		if !agent.ThinkingOff(c.Value) {
			levels = append(levels, c.Value)
		}
	}
	return levels
}

// set changes a configuration option and takes the configuration the agent
// returns.
func (st *state) set(ctx context.Context, c *conn, id, value string) error {
	var res struct {
		ConfigOptions []configOption `json:"configOptions"`
	}
	err := c.callInto(ctx, "session/set_config_option", map[string]any{"sessionId": st.SessionID, "configId": id, "value": value}, &res)
	if err != nil {
		return err
	}
	if res.ConfigOptions != nil {
		st.ConfigOptions = res.ConfigOptions
	} else if o := st.optionByID(id); o != nil {
		o.CurrentValue, _ = json.Marshal(value)
	}
	return nil
}

func (st *state) optionByID(id string) *configOption {
	for i := range st.ConfigOptions {
		if st.ConfigOptions[i].ID == id {
			return &st.ConfigOptions[i]
		}
	}
	return nil
}

// selectMode switches to the first of modes the agent offers.
func (st *state) selectMode(ctx context.Context, c *conn, modes []string) error {
	if o := st.option(categoryMode); o != nil {
		for _, mode := range modes {
			if o.current() == mode {
				return nil
			}
			if o.has(mode) {
				return st.set(ctx, c, o.ID, mode)
			}
		}
		return nil
	}
	if st.Modes == nil {
		return nil
	}
	for _, mode := range modes {
		if st.Modes.CurrentModeID == mode {
			return nil
		}
		for _, m := range st.Modes.AvailableModes {
			if m.ID == mode {
				err := c.callInto(ctx, "session/set_mode", map[string]any{"sessionId": st.SessionID, "modeId": mode}, nil)
				if err == nil {
					st.Modes.CurrentModeID = mode
				}
				return err
			}
		}
	}
	return nil
}

// configure applies a model and reasoning level the agent offers, and
// returns the settings in effect.
func (st *state) configure(ctx context.Context, c *conn, s agent.Settings) (agent.Settings, error) {
	choices, current := st.models()
	if s.Model != "" && s.Model != current && hasChoice(choices, s.Model) {
		var err error
		if o := st.option(categoryModel); o != nil {
			err = st.set(ctx, c, o.ID, s.Model)
		} else {
			err = c.callInto(ctx, "session/set_model", map[string]any{"sessionId": st.SessionID, "modelId": s.Model}, nil)
			if err == nil {
				st.Models.CurrentModelID = s.Model
			}
		}
		if err != nil {
			return st.settings(), err
		}
	}
	if o := st.option(categoryThought); o != nil && s.Effort != "" && s.Effort != o.current() && o.has(s.Effort) {
		if err := st.set(ctx, c, o.ID, s.Effort); err != nil {
			return st.settings(), err
		}
	}
	return st.settings(), nil
}

// settings are the selected model and reasoning level.
func (st *state) settings() agent.Settings {
	_, model := st.models()
	s := agent.Settings{Model: model}
	if o := st.option(categoryThought); o != nil {
		for _, level := range st.efforts() {
			if level == o.current() {
				s.Effort = level
			}
		}
	}
	return s
}

func hasChoice(choices []choice, value string) bool {
	for _, c := range choices {
		if c.Value == value {
			return true
		}
	}
	return false
}

// errNoSession reports a saved session the agent cannot reopen.
var errNoSession = errors.New("the agent cannot reopen saved sessions")

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
