// Package acpcli runs the coding agent CLIs that serve the Agent Client
// Protocol themselves, as T3 Code runs them: Cursor, OpenCode, Grok Build,
// Antigravity, Kiro, Muse Code and fx. Each reuses the sign-in its CLI
// keeps, so the user signs in with the CLI first.
package acpcli

import (
	"fmt"
	"os"
	"strings"

	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/agent/acp"
)

// CLI is how one agent CLI is found and started.
type CLI struct {
	// Names are the executable's names, tried in order.
	Names []string
	Spec  acp.Spec
	// configure adapts the Spec to the login environment.
	configure func(*acp.Spec, []string)
}

// CLIs are the supported CLIs by tool.
var CLIs = map[agent.Tool]CLI{
	// Cursor's CLI installs as cursor-agent and agent; its agent mode has
	// full tool access.
	agent.Cursor: {Names: []string{"cursor-agent", "agent"}, Spec: acp.Spec{
		Tool: agent.Cursor, Provider: "Cursor", Args: []string{"acp"}, Modes: []string{"agent"},
		AuthMethod: "cursor_login"}},
	agent.OpenCode: {Names: []string{"opencode"}, Spec: acp.Spec{
		Tool: agent.OpenCode, Provider: "OpenCode", Args: []string{"acp"}}},
	// Grok Build approves every tool itself when told to on the command
	// line, which overrides its configuration.
	agent.Grok: {Names: []string{"grok"}, Spec: acp.Spec{
		Tool: agent.Grok, Provider: "xAI", Args: []string{"agent", "--always-approve", "stdio"}, AuthMethod: "cached_token"},
		configure: func(s *acp.Spec, env []string) {
			if lookup(env, "XAI_API_KEY") != "" {
				s.AuthMethod = "xai.api_key"
			}
		}},
	// Antigravity's agy picks its credentials from the method it is
	// authenticated with, and reads the system prompt under its own key.
	agent.Antigravity: {Names: []string{"agy"}, Spec: acp.Spec{
		Tool: agent.Antigravity, Provider: "Google", Args: []string{"acp"}, AuthMethod: "oauth-personal",
		SystemPromptMeta: "antigravity"},
		configure: func(s *acp.Spec, env []string) {
			if lookup(env, "GEMINI_API_KEY") != "" {
				s.AuthMethod = "gemini-api-key"
			}
		}},
	// Kiro's default engine speaks another protocol; --auth-method=cli
	// keeps its tokens inside Kiro. Autopilot skips its prompts and its
	// review of a turn's changes.
	agent.Kiro: {Names: []string{"kiro-cli"}, Spec: acp.Spec{
		Tool: agent.Kiro, Provider: "Kiro", Args: []string{"acp", "--agent-engine=v3", "--auth-method=cli"},
		Options: map[string]string{"autopilot": "on"}}},
	agent.Muse: {Names: []string{"muse"}, Spec: acp.Spec{
		Tool: agent.Muse, Provider: "Meta", Args: []string{"acp"}}},
	// fx's automatic review was called code before auto; it keeps the
	// user's own MCP servers only when asked.
	agent.Fx: {Names: []string{"fx"}, Spec: acp.Spec{
		Tool: agent.Fx, Provider: "fx", Args: []string{"acp"}, Modes: []string{"auto", "code"},
		SessionMeta: map[string]any{"fx": map[string]any{"profileMcpServers": true}}}},
}

// Locate finds tool's CLI on the login PATH, as agent.LocateScript does.
func Locate(tool agent.Tool, env []string) (*acp.Driver, error) {
	cli, ok := CLIs[tool]
	if !ok {
		return nil, fmt.Errorf("%s does not serve ACP", tool.Name())
	}
	for _, name := range cli.Names {
		if path, args := agent.LocateScript(name, env); path != "" {
			spec := cli.Spec
			if cli.configure != nil {
				cli.configure(&spec, env)
			}
			return &acp.Driver{Spec: spec, Path: path, PathArgs: args, Env: env}, nil
		}
	}
	return nil, fmt.Errorf("%s was not found on PATH", cli.Names[0])
}

// lookup returns a variable of env, or of this process without one.
func lookup(env []string, name string) string {
	if env == nil {
		return os.Getenv(name)
	}
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], name+"="); ok {
			return v
		}
	}
	return ""
}
