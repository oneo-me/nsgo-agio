package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/oneo-me/nsgo-agio/agent"
)

// CallTool calls a tool of the HTTP server m as an agent does, such as for
// an application's tests standing in for one, and returns the text it read
// back and whether the call failed.
func CallTool(ctx context.Context, m agent.MCPServer, name string, arguments any) (string, bool, error) {
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := post(ctx, m, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result); err != nil {
		return "", false, err
	}
	var text string
	for _, c := range result.Content {
		text += c.Text
	}
	return text, result.IsError, nil
}

// post sends a request to m and decodes its result into v.
func post(ctx context.Context, m agent.MCPServer, method string, params, v any) error {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, h := range m.Headers {
		req.Header.Set(k, h)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", method, res.Status)
	}
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		return err
	}
	if reply.Error != nil {
		return reply.Error
	}
	return json.Unmarshal(reply.Result, v)
}
