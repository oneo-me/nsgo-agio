package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/transcript"
)

// titledBackend is a Backend with conversations that Choose renames.
type titledBackend struct {
	agio.Backend
	mu sync.Mutex
	s  agio.State
}

func (b *titledBackend) State() agio.State {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.s
	s.Conversations = nil
	for _, c := range b.s.Conversations {
		conv := *c
		s.Conversations = append(s.Conversations, &conv)
	}
	return s
}

func (b *titledBackend) Choose(id string, ch agio.Choice) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c := b.s.Conversation(id); c != nil && ch.Title != nil {
		c.Title = *ch.Title
	}
	return nil
}

func (b *titledBackend) add(id, prompt string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.s.Conversations = append(b.s.Conversations, &agio.Conversation{ID: id, Title: prompt,
		Turns: []*transcript.Turn{transcript.New(id, prompt, nil, time.Now())}})
}

func TestNewTasksAreNamed(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		prompts = append(prompts, body.Messages[1].Content)
		if fail {
			http.Error(w, `{"error": {"message": "invalid key"}}`, http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"修复登录跳转\"}"}}]}`))
	}))
	defer server.Close()
	b := &titledBackend{}
	b.add("resumed", "an earlier task")
	st := &store[settings]{s: settings{Providers: []modelProvider{{ID: "p", BaseURL: server.URL, Models: []providerModel{{ID: "m", Model: "small"}}}}}}
	n := newNamer(b, st, func() string { return "zh-Hans" })

	// Without a naming model, a task keeps its first message.
	b.add("plain", "fix the login redirect")
	n.look(b.State())
	st.update(func(s *settings) { s.NamingModel = modelReference{Provider: "p", Model: "m"} })
	b.add("named", "登录后跳转错误，请修复")
	n.look(b.State())
	n.look(b.State())
	waitUntil(t, func() bool { return b.State().Conversation("named").Title == "修复登录跳转" })
	if s := b.State(); s.Conversation("resumed").Title != "an earlier task" || s.Conversation("plain").Title != "fix the login redirect" {
		t.Fatalf("renamed tasks that were not new: %+v %+v", s.Conversation("resumed"), s.Conversation("plain"))
	}
	mu.Lock()
	if len(prompts) != 1 || prompts[0] != "登录后跳转错误，请修复" {
		t.Fatalf("asked to name %q", prompts)
	}
	fail = true
	mu.Unlock()

	b.add("failed", "add dark mode")
	n.look(b.State())
	waitUntil(t, func() bool { return n.lastFailure() != "" })
	if b.State().Conversation("failed").Title != "add dark mode" {
		t.Fatal("a failed naming changed the title")
	}
}

// waitUntil waits for ok to hold.
func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("never happened")
		}
	}
}

func TestRemovingTheNamingModelClearsIt(t *testing.T) {
	s := settings{Providers: []modelProvider{{ID: "p", Models: []providerModel{{ID: "a"}, {ID: "b"}}}}}
	s.NamingModel = modelReference{Provider: "p", Model: "a"}
	s.removeProviderModel("p", "b")
	if s.NamingModel.Model != "a" {
		t.Fatal("removing another model cleared the naming model")
	}
	s.removeProviderModel("p", "a")
	if s.NamingModel != (modelReference{}) {
		t.Fatal("kept a removed naming model")
	}
	s.NamingModel = modelReference{Provider: "p", Model: "b"}
	s.removeProvider("p")
	if s.NamingModel != (modelReference{}) {
		t.Fatal("kept a removed provider's naming model")
	}
}
