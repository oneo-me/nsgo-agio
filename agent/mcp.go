package agent

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
)

// MCPServer is a Model Context Protocol server offered to an agent for one
// session. Command, with Args and Env added to the agent's environment,
// starts it on the agent's machine to talk over stdio; otherwise the agent
// reaches URL over streamable HTTP, sending Headers with each request.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// mcpName is what every agent accepts: names become configuration keys and
// parts of tool names.
var mcpName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Check reports why the server cannot be offered.
func (m MCPServer) Check() error {
	if !mcpName.MatchString(m.Name) {
		return fmt.Errorf("MCP server name must be letters, digits, - and _: %q", m.Name)
	}
	if (m.Command == "") == (m.URL == "") {
		return fmt.Errorf("MCP server %s needs either a command or a URL", m.Name)
	}
	if m.URL != "" && (len(m.Args) > 0 || len(m.Env) > 0) {
		return fmt.Errorf("MCP server %s has arguments or environment but no command", m.Name)
	}
	if m.Command != "" && len(m.Headers) > 0 {
		return fmt.Errorf("MCP server %s has headers but no URL", m.Name)
	}
	return nil
}

// HTTP reports whether the agent reaches the server over HTTP.
func (m MCPServer) HTTP() bool { return m.URL != "" }

// MCPConfig is servers in the mcpServers form Claude Code's --mcp-config
// reads, keyed by name.
func MCPConfig(servers []MCPServer) map[string]any {
	config := map[string]any{}
	for _, m := range servers {
		entry := map[string]any{}
		if m.HTTP() {
			entry["type"], entry["url"] = "http", m.URL
			if len(m.Headers) > 0 {
				entry["headers"] = m.Headers
			}
		} else {
			entry["type"], entry["command"], entry["args"] = "stdio", m.Command, append([]string{}, m.Args...)
			if len(m.Env) > 0 {
				entry["env"] = m.Env
			}
		}
		config[m.Name] = entry
	}
	return config
}

// JoinMCPServers lists servers by name in order of first appearance, a
// later list's server replacing an earlier one of the same name, after
// checking each.
func JoinMCPServers(lists ...[]MCPServer) ([]MCPServer, error) {
	var servers []MCPServer
	for _, list := range lists {
		for _, m := range list {
			if err := m.Check(); err != nil {
				return nil, err
			}
			m.Args, m.Env, m.Headers = slices.Clone(m.Args), maps.Clone(m.Env), maps.Clone(m.Headers)
			if i := slices.IndexFunc(servers, func(s MCPServer) bool { return s.Name == m.Name }); i >= 0 {
				servers[i] = m
			} else {
				servers = append(servers, m)
			}
		}
	}
	return servers, nil
}

// ErrMCPHTTP is returned by agents that reach MCP servers only over stdio.
var ErrMCPHTTP = errors.New("the agent cannot reach MCP servers over HTTP")
