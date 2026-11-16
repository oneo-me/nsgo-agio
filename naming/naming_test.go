package naming

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuggest(t *testing.T) {
	var got struct {
		Model    string
		Messages []struct{ Role, Content string }
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		reply := "```json\n{\"title\": \"“修复登录跳转”\"}\n```"
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": reply}}}})
	}))
	defer server.Close()
	p := Provider{BaseURL: server.URL + "/v1/", APIKey: " secret ", Model: " small ", TitleLanguage: "zh-Hans"}
	title, err := p.Suggest(context.Background(), "登录后跳转错误，请修复")
	if err != nil {
		t.Fatal(err)
	}
	if title != "修复登录跳转" {
		t.Fatalf("title %q", title)
	}
	if got.Model != "small" || len(got.Messages) != 2 ||
		got.Messages[0].Role != "system" || got.Messages[0].Content != instructions+"\nRequired title language: Simplified Chinese." ||
		got.Messages[1].Role != "user" || got.Messages[1].Content != "登录后跳转错误，请修复" {
		t.Fatalf("%+v", got)
	}
}

func TestSuggestErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("authorization sent without a key")
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "invalid key"}}`))
	}))
	defer server.Close()
	_, err := Provider{BaseURL: server.URL + "/chat/completions", Model: "m"}.Suggest(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatal(err)
	}
	if _, err := (Provider{}).Suggest(context.Background(), "x"); err == nil {
		t.Fatal("unconfigured provider requested a title")
	}
	for _, reply := range []string{"I cannot name this", `{"title": " "}`} {
		if _, err := parse(reply); err == nil {
			t.Errorf("%q accepted", reply)
		}
	}
}

func TestEndpoint(t *testing.T) {
	for base, want := range map[string]string{
		"":                                   "https://api.openai.com/v1/chat/completions",
		"http://localhost:11434/v1":          "http://localhost:11434/v1/chat/completions",
		"https://x.test/v1/chat/completions": "https://x.test/v1/chat/completions",
	} {
		if got := (Provider{BaseURL: base}).Endpoint(); got != want {
			t.Errorf("%q: %q", base, got)
		}
	}
	if got := shorten(strings.Repeat("a", 70) + "\nmore"); len([]rune(got)) != 61 {
		t.Errorf("long title %q", got)
	}
}
