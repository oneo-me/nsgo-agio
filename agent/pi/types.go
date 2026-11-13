package pi

import (
	"encoding/json"
	"strings"

	"github.com/oneo-me/nsgo-agio/agent"
)

// Block is one message content block: text, thinking, toolCall or image.
type Block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Data      string          `json:"data,omitempty"`
	MimeType  string          `json:"mimeType,omitempty"`
}

// Content is a string or a list of blocks on the wire.
type Content []Block

func (c *Content) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*c = Content{{Type: "text", Text: text}}
		return nil
	}
	var blocks []Block
	if err := json.Unmarshal(data, &blocks); err != nil {
		return err
	}
	*c = blocks
	return nil
}

// Text joins the text blocks.
func (c Content) Text() string {
	var parts []string
	for _, b := range c {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// Usage is provider-reported token usage.
type Usage struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	CacheRead   int64 `json:"cacheRead"`
	CacheWrite  int64 `json:"cacheWrite"`
	TotalTokens int64 `json:"totalTokens"`
}

// ContextTokens follows pi's footer: total tokens, or the sum of components
// when a provider leaves the total empty.
func (u *Usage) ContextTokens() int64 {
	if u == nil {
		return 0
	}
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.Input + u.Output + u.CacheRead + u.CacheWrite
}

// Message is an AgentMessage. Unknown roles are kept and ignored by callers.
type Message struct {
	Role         string  `json:"role"`
	Content      Content `json:"content"`
	StopReason   string  `json:"stopReason,omitempty"`
	ErrorMessage string  `json:"errorMessage,omitempty"`
	Usage        *Usage  `json:"usage,omitempty"`
	Timestamp    int64   `json:"timestamp,omitempty"`
	ToolCallID   string  `json:"toolCallId,omitempty"`
	ToolName     string  `json:"toolName,omitempty"`
	IsError      bool    `json:"isError,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	Model        string  `json:"model,omitempty"`
}

// StreamEvent is a delta-only assistantMessageEvent.
type StreamEvent struct {
	Type         string          `json:"type"`
	ContentIndex int             `json:"contentIndex"`
	Delta        string          `json:"delta,omitempty"`
	Content      string          `json:"content,omitempty"`
	ID           string          `json:"id,omitempty"`
	ToolName     string          `json:"toolName,omitempty"`
	ToolCall     *Block          `json:"toolCall,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Error        json.RawMessage `json:"error,omitempty"`
}

// ToolResult is a tool's partial or final result.
type ToolResult struct {
	Content Content `json:"content"`
}

// Event is one session event or extension request from stdout. Fields are
// populated according to Type; see pi's JSON event stream reference.
type Event struct {
	Type                  string          `json:"type"`
	Message               *Message        `json:"message,omitempty"`
	Usage                 *Usage          `json:"usage,omitempty"`
	AssistantMessageEvent *StreamEvent    `json:"assistantMessageEvent,omitempty"`
	ToolCallID            string          `json:"toolCallId,omitempty"`
	ToolName              string          `json:"toolName,omitempty"`
	Args                  json.RawMessage `json:"args,omitempty"`
	PartialResult         *ToolResult     `json:"partialResult,omitempty"`
	Result                *ToolResult     `json:"result,omitempty"`
	IsError               bool            `json:"isError,omitempty"`
	Aborted               bool            `json:"aborted,omitempty"`
	Name                  *string         `json:"name,omitempty"`
	Level                 string          `json:"level,omitempty"`
	Success               bool            `json:"success,omitempty"`
	FinalError            string          `json:"finalError,omitempty"`
	ErrorMessage          string          `json:"errorMessage,omitempty"`
	Error                 string          `json:"error,omitempty"`
	ID                    string          `json:"id,omitempty"`
	Method                string          `json:"method,omitempty"`
}

// Model is a configured model from get_state or get_available_models.
type Model struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Provider         string             `json:"provider"`
	Reasoning        bool               `json:"reasoning"`
	Input            []string           `json:"input"`
	ContextWindow    int64              `json:"contextWindow"`
	ThinkingLevelMap map[string]*string `json:"thinkingLevelMap,omitempty"`
}

// ThinkingLevels lists every level in pi's order.
var ThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// Levels returns the thinking levels this model accepts, as pi does: a level
// mapped to null is unsupported, xhigh and max need a mapping, and other
// unmapped levels use the provider's default. A model without reasoning, or
// without a level that keeps it thinking, has none.
func (m Model) Levels() []string {
	if !m.Reasoning {
		return nil
	}
	var levels []string
	for _, level := range ThinkingLevels {
		if agent.ThinkingOff(level) {
			continue
		}
		v, ok := m.ThinkingLevelMap[level]
		if ok && v == nil || !ok && (level == "xhigh" || level == "max") {
			continue
		}
		levels = append(levels, level)
	}
	return levels
}

// AcceptsImages reports whether the model takes image input.
func (m Model) AcceptsImages() bool {
	for _, input := range m.Input {
		if input == "image" {
			return true
		}
	}
	return false
}

// Key identifies a model as provider/id.
func (m Model) Key() string { return m.Provider + "/" + m.ID }

// State is the get_state response.
type State struct {
	Model         *Model `json:"model,omitempty"`
	ThinkingLevel string `json:"thinkingLevel"`
	IsStreaming   bool   `json:"isStreaming"`
	SessionFile   string `json:"sessionFile"`
	SessionID     string `json:"sessionId"`
	SessionName   string `json:"sessionName"`
}

// Stats is the get_session_stats response.
type Stats struct {
	ContextUsage *struct {
		Tokens        *int64 `json:"tokens"`
		ContextWindow int64  `json:"contextWindow"`
	} `json:"contextUsage,omitempty"`
}

// Image is prompt image content.
type Image struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}
