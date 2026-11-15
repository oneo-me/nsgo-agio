package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/oneo-me/nsgo-agio/agent"
)

// mcpExtension writes a temporary extension registering servers for the
// session that loads it with -e, leaving pi's mcp.json files alone; a
// server of the same name there takes precedence. The caller removes the
// returned directory once the session ends.
func mcpExtension(servers []agent.MCPServer) (dir, file string, err error) {
	config := map[string]any{}
	for _, m := range servers {
		entry := map[string]any{}
		if m.HTTP() {
			entry["url"] = m.URL
			if len(m.Headers) > 0 {
				entry["headers"] = m.Headers
			}
		} else {
			entry["command"] = m.Command
			if len(m.Args) > 0 {
				entry["args"] = m.Args
			}
			if len(m.Env) > 0 {
				entry["env"] = m.Env
			}
		}
		config[m.Name] = entry
	}
	// JSON is a JavaScript expression.
	data, err := json.Marshal(config)
	if err != nil {
		return "", "", err
	}
	if dir, err = os.MkdirTemp("", agent.Client.Name+"-mcp-"); err != nil {
		return "", "", err
	}
	// /mcp shows the file as the servers' source.
	file = filepath.Join(dir, agent.Client.Name+"-mcp.js")
	source := fmt.Sprintf(`const servers = %s;
export default function (pi) {
  for (const [name, config] of Object.entries(servers)) pi.registerMcpServer(name, config);
}
`, data)
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		os.RemoveAll(dir)
		return "", "", err
	}
	return dir, file, nil
}
