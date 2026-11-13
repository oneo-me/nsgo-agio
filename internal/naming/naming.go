// Package naming asks a model behind an OpenAI-compatible chat completions
// endpoint for a new task's title.
package naming

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
)

// DefaultBaseURL is used when a provider leaves its base URL empty.
const DefaultBaseURL = "https://api.openai.com/v1"

// maxPrompt bounds the task text sent for naming, in runes.
const maxPrompt = 4000

const instructions = `You name coding tasks. Given the user's first message to a coding agent, reply with only a JSON object of the form {"title": "..."}.

The title is the shortest natural title that still identifies the work, in the requested title language, regardless of the message's language or the examples below. Prefer a concrete action and its key object; keep technical names and qualifiers needed to distinguish the task. Aim for 6-12 characters in Chinese or 3-6 words in English, and stay within 50 characters. These are targets, not padding requirements: use fewer words when clear, and allow extra length when essential to preserve meaning. Summarize the main change rather than repeating the request or listing subtasks. Drop politeness, background, explanations, redundant modifiers and generic phrases such as "work on", "help with" or "进行优化". Avoid vague titles such as "优化功能" or "Update code". Rewrite concisely instead of cutting off words or adding an ellipsis. No quotes or trailing punctuation.

Examples of titles:
- "优化标题生成算法，应该在能体现工作的情况下，简短一些" -> "精简任务标题"
- "请帮我修复用户登录之后跳转到了错误页面的问题" -> "修复登录跳转"
- "希望在设置页面中增加一个可以切换深色模式的选项" -> "添加深色模式开关"
- "Please investigate and fix the login redirect that sends users to the wrong page" -> "Fix login redirect"
- "Add retries when the API returns HTTP 429 without retrying other errors" -> "Retry HTTP 429 responses"`

// Provider is an OpenAI-compatible endpoint and the model used for naming.
// An empty APIKey sends no authorization, for local servers.
type Provider struct {
	BaseURL, APIKey, Model string
	// TitleLanguage is zh-Hans or en, the default.
	TitleLanguage string
	// Client defaults to http.DefaultClient; callers bound requests with
	// their context.
	Client *http.Client
}

// Configured reports whether naming should be requested at all.
func (p Provider) Configured() bool { return strings.TrimSpace(p.Model) != "" }

// Endpoint is the chat completions URL. A base URL already naming the
// endpoint is accepted as is.
func (p Provider) Endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

// Suggest asks the model to title a task described by prompt.
func (p Provider) Suggest(ctx context.Context, prompt string) (string, error) {
	if !p.Configured() {
		return "", errors.New("no naming model is configured")
	}
	if r := []rune(strings.TrimSpace(prompt)); len(r) > maxPrompt {
		prompt = string(r[:maxPrompt])
	}
	language := "English"
	if p.TitleLanguage == "zh-Hans" {
		language = "Simplified Chinese"
	}
	body, err := json.Marshal(map[string]any{
		"model": strings.TrimSpace(p.Model),
		"messages": []map[string]string{
			{"role": "system", "content": instructions + "\nRequired title language: " + language + "."},
			{"role": "user", "content": prompt},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(p.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var reply struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeErr := json.Unmarshal(data, &reply)
	if resp.StatusCode/100 != 2 {
		if decodeErr == nil && reply.Error != nil && reply.Error.Message != "" {
			return "", fmt.Errorf("%s: %s", resp.Status, reply.Error.Message)
		}
		return "", errors.New(resp.Status)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("unexpected response: %w", decodeErr)
	}
	if reply.Error != nil && reply.Error.Message != "" {
		return "", errors.New(reply.Error.Message)
	}
	if len(reply.Choices) == 0 {
		return "", errors.New("the response has no choices")
	}
	return parse(reply.Choices[0].Message.Content)
}

// parse reads the JSON object in content, tolerating code fences and text
// around it, as models do not all support a JSON response format.
func parse(content string) (string, error) {
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return "", fmt.Errorf("the reply is not JSON: %q", content)
	}
	var v struct{ Title string }
	if err := json.Unmarshal([]byte(content[start:end+1]), &v); err != nil {
		return "", fmt.Errorf("the reply is not JSON: %w", err)
	}
	title := shorten(strings.TrimFunc(v.Title, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("\"'`“”‘’「」", r)
	}))
	if title == "" {
		return "", errors.New("the reply has no title")
	}
	return title, nil
}

// shorten keeps the first nonempty line of text, at most 60 runes, as the
// host titles a task from its first message.
func shorten(text string) string {
	for line := range strings.Lines(text) {
		if line = strings.TrimSpace(line); line != "" {
			if r := []rune(line); len(r) > 60 {
				return strings.TrimSpace(string(r[:60])) + "…"
			}
			return line
		}
	}
	return ""
}
