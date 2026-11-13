// Package claude runs Anthropic's Claude Code CLI through its stream-json
// print mode. Every session runs with permission checks bypassed, as
// Agio's other agents run with full access.
package claude

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

// ErrNotFound reports that claude is not on the login PATH.
var ErrNotFound = errors.New("claude was not found on PATH")

// probeTimeout bounds the sessionless initialization that reads models.
const probeTimeout = 30 * time.Second

// Locate finds claude on env's PATH, then in its own installation
// directories, which a minimal PATH may omit.
func Locate(env []string) (*Driver, error) {
	path := agent.LookPath("claude", env)
	if home := agent.EnvValue(env, "HOME"); path == "" && home != "" {
		for _, candidate := range []string{filepath.Join(home, ".claude", "local", "claude"), filepath.Join(home, ".local", "bin", "claude")} {
			if agent.IsExecutable(candidate) {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		return nil, ErrNotFound
	}
	return &Driver{Path: path, Env: env}, nil
}

// Driver runs Claude Code as an agent.Driver.
type Driver struct {
	Path string
	Env  []string
}

func (d *Driver) Tool() agent.Tool { return agent.Claude }

func (d *Driver) command(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command(d.Path, args...)
	cmd.Dir, cmd.Env = dir, d.Env
	return cmd
}

// Version runs `claude --version`.
func (d *Driver) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.Path, "--version")
	childproc.HideConsole(cmd)
	cmd.Env = d.Env
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", fmt.Errorf("unrecognized claude version %q", out)
	}
	return fields[0], nil
}

// modelInfo is one entry of the initialization response.
type modelInfo struct {
	Value                 string   `json:"value"`
	ResolvedModel         string   `json:"resolvedModel"`
	DisplayName           string   `json:"displayName"`
	Description           string   `json:"description"`
	SupportsEffort        *bool    `json:"supportsEffort"`
	SupportedEffortLevels []string `json:"supportedEffortLevels"`
}

// Catalog reads models from a sessionless initialization request, which
// neither runs a model turn nor saves a conversation.
func (d *Driver) Catalog(ctx context.Context, dir string) (agent.Catalog, error) {
	cmd := d.command(dir, "--output-format", "stream-json", "--verbose", "--input-format", "stream-json",
		"--no-session-persistence", "--setting-sources", "user",
		"--mcp-config", `{"mcpServers":{}}`, "--strict-mcp-config")
	cmd.Env = append(append([]string{}, d.Env...), "ENABLE_CLAUDEAI_MCP_SERVERS=false")
	responses := make(chan json.RawMessage, 1)
	proc, err := agent.StartProcess(cmd, func(line []byte) {
		var m struct {
			Type     string `json:"type"`
			Response struct {
				RequestID string          `json:"request_id"`
				Response  json.RawMessage `json:"response"`
				Error     string          `json:"error"`
			} `json:"response"`
		}
		if json.Unmarshal(line, &m) == nil && m.Type == "control_response" && m.Response.RequestID == "agio-catalog" {
			select {
			case responses <- m.Response.Response:
			default:
			}
		}
	}, nil)
	if err != nil {
		return agent.Catalog{}, fmt.Errorf("start claude: %w", err)
	}
	defer proc.Close(time.Second)
	if err := proc.Send(map[string]any{"type": "control_request", "request_id": "agio-catalog", "request": map[string]any{"subtype": "initialize"}}); err != nil {
		return agent.Catalog{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var data json.RawMessage
	select {
	case data = <-responses:
	case <-proc.Done():
		return agent.Catalog{}, fmt.Errorf("claude exited: %s", proc.Stderr())
	case <-ctx.Done():
		return agent.Catalog{}, fmt.Errorf("claude initialize: %w", ctx.Err())
	}
	var init struct {
		Models []modelInfo `json:"models"`
	}
	if err := json.Unmarshal(data, &init); err != nil {
		return agent.Catalog{}, err
	}
	return catalog(init.Models), nil
}

// catalog names models specifically. The default alias is folded into the
// entry for the model it resolves to, so a model appears once.
func catalog(models []modelInfo) agent.Catalog {
	var c agent.Catalog
	defaultModel := ""
	resolved := map[string]bool{}
	for _, m := range models {
		if m.Value != "default" {
			resolved[firstNonEmpty(m.ResolvedModel, m.Value)] = true
		}
	}
	for _, m := range models {
		id := strings.TrimSpace(m.Value)
		if id == "" {
			continue
		}
		concrete := firstNonEmpty(m.ResolvedModel, id)
		if id == "default" {
			defaultModel = concrete
			if resolved[concrete] {
				continue
			}
		}
		name := strings.TrimSpace(m.DisplayName)
		if id == "default" || name == "" {
			// "Default (recommended)" names a policy; the description names
			// the model, as in "Opus 5.5 · Best for everyday tasks".
			name, _, _ = strings.Cut(m.Description, " · ")
			name = firstNonEmpty(strings.TrimSpace(name), concrete)
		}
		if !strings.HasPrefix(strings.ToLower(name), "claude") {
			name = "Claude " + name
		}
		model := agent.Model{Tool: agent.Claude, ID: id, Name: name, Provider: "Anthropic", Detail: concrete, Images: true}
		if m.SupportsEffort == nil || *m.SupportsEffort {
			for _, level := range m.SupportedEffortLevels {
				if !agent.ThinkingOff(level) {
					model.Efforts = append(model.Efforts, level)
				}
			}
		}
		for _, preferred := range []string{"high", "medium"} {
			if model.DefaultEffort == "" && model.HasEffort(preferred) {
				model.DefaultEffort = preferred
			}
		}
		if model.DefaultEffort == "" && len(model.Efforts) > 0 {
			model.DefaultEffort = model.Efforts[0]
		}
		c.Models = append(c.Models, model)
	}
	for i, m := range c.Models {
		if m.ID == "default" || m.Detail == defaultModel && c.Model == "" {
			c.Models[i].Default = true
			c.Model, c.Effort = m.ID, m.DefaultEffort
		}
	}
	if c.Model == "" && len(c.Models) > 0 {
		c.Model, c.Effort = c.Models[0].ID, c.Models[0].DefaultEffort
	}
	return c
}

// toolName maps Claude Code's tools to the names Agio groups activities by.
func toolName(name string) string {
	switch name {
	case "Bash", "Read", "Edit", "Write", "Grep", "MultiEdit":
		return strings.ToLower(name)
	case "Glob", "LS":
		return "find"
	case "WebSearch", "WebFetch":
		return "web_search"
	case "NotebookEdit":
		return "edit"
	}
	return name
}

// usageTokens is the context a call occupied: its input, cached input and
// output tokens, taken from its last iteration when present.
func usageTokens(raw json.RawMessage) int64 {
	type usage struct {
		InputTokens              int64 `json:"input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
	}
	var u struct {
		usage
		Iterations []usage `json:"iterations"`
	}
	if json.Unmarshal(raw, &u) != nil {
		return 0
	}
	call := u.usage
	if n := len(u.Iterations); n > 0 {
		call = u.Iterations[n-1]
	}
	return call.InputTokens + call.CacheReadInputTokens + call.CacheCreationInputTokens + call.OutputTokens
}

// resultText joins a tool result's text content, a string or blocks.
func resultText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct {
		Type, Text string
	}
	json.Unmarshal(raw, &blocks)
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func newSessionID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// projectsDir is where Claude Code saves conversations.
func (d *Driver) projectsDir() string {
	if dir := agent.EnvValue(d.Env, "CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "projects")
	}
	home := agent.EnvValue(d.Env, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".claude", "projects")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
