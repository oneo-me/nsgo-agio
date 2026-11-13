package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/internal/naming"
)

// namingTimeout bounds how long a task waits for its title.
const namingTimeout = 20 * time.Second

// namer titles each new task with the naming model once its first message
// is sent. The host has titled it with that message by then, which a task
// keeps without a naming model, when naming fails or once its agent
// renamed it.
type namer struct {
	b  agio.Backend
	st *store[settings]
	// lang is the interface's language.
	lang func() string

	mu sync.Mutex
	// seen are the conversations named or resumed.
	seen map[string]bool
	// failure is why naming last failed, empty once it succeeds.
	failure string
	changed func()
}

// newNamer leaves the conversations b already has, the tasks resumed, as
// they are.
func newNamer(b agio.Backend, st *store[settings], lang func() string) *namer {
	n := &namer{b: b, st: st, lang: lang, seen: map[string]bool{}}
	for _, c := range b.State().Conversations {
		n.seen[c.ID] = true
	}
	return n
}

// provider is the naming model chosen, in the titles' language.
func (n *namer) provider() naming.Provider {
	s := n.st.get()
	language := s.TitleLanguage
	if language == "" {
		language = n.lang()
	}
	if strings.HasPrefix(language, "zh") {
		language = "zh-Hans"
	} else {
		language = "en"
	}
	p := naming.Provider{TitleLanguage: language}
	if provider, model, ok := s.providerModel(s.NamingModel); ok {
		p.BaseURL, p.APIKey, p.Model = provider.BaseURL, provider.APIKey, model.Model
	}
	return p
}

// look starts naming the conversations whose first message was sent since
// it last looked.
func (n *namer) look(s agio.State) {
	provider := n.provider()
	n.mu.Lock()
	defer n.mu.Unlock()
	seen := map[string]bool{}
	for _, c := range s.Conversations {
		if n.seen[c.ID] || len(c.Turns) == 0 {
			seen[c.ID] = n.seen[c.ID]
			continue
		}
		seen[c.ID] = true
		if provider.Configured() {
			go n.name(provider, c.ID, c.Title, c.Turns[0].Prompt)
		}
	}
	n.seen = seen
}

// name asks for a title, which replaces the one taken from the first
// message unless the conversation was renamed since.
func (n *namer) name(provider naming.Provider, id, title, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), namingTimeout)
	defer cancel()
	named, err := provider.Suggest(ctx, prompt)
	if err == nil {
		if c := n.b.State().Conversation(id); c != nil && c.Title == title {
			err = n.b.Choose(id, agio.Choice{Title: &named})
		}
	}
	n.mu.Lock()
	var changed func()
	if provider == n.provider() {
		failure := ""
		if err != nil {
			failure = err.Error()
		}
		if failure != n.failure {
			n.failure, changed = failure, n.changed
		}
	}
	n.mu.Unlock()
	if changed != nil {
		changed()
	}
}

// lastFailure is why naming last failed, empty once it succeeds.
func (n *namer) lastFailure() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.failure
}

// onChange has fn called, on any goroutine, when lastFailure changes.
func (n *namer) onChange(fn func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.changed = fn
}

// namingSettings is the Naming page: the titles' language and the model
// naming tasks, which reports why naming last failed.
func (a *app) namingSettings() ui.SettingsCategory {
	m := a.msgs()
	description := m.NamingModelDescription()
	if failure := a.namer.lastFailure(); failure != "" {
		description += " " + m.NamingFailed(failure)
	}
	return ui.SettingsCategory{
		ID: "naming", Title: m.Naming(), Icon: lucide.Sparkles,
		Groups: []ui.SettingsGroup{{
			ID: "naming", Items: []ui.SettingsItem{{
				ID: "title-language", Title: m.TitleLanguage(), Description: m.TitleLanguageDescription(),
				Keywords: []string{"naming", "title", "language", "English", "中文", "命名", "标题", "语言"},
				Control: a.selectChoice(func(s *settings) *string { return &s.TitleLanguage }, m.TitleLanguage(), []string{"", "en", "zh-Hans"}, func(v string) string {
					if v == "" {
						return m.TitleLanguageInterface()
					}
					return a.choiceName(v)
				}),
			}, {
				ID: "model", Title: m.NamingModel(), Description: description,
				Keywords: []string{"naming", "title", "model", "命名", "标题", "模型"},
				Control:  core.Dynamic(func(*core.Scope) core.Node { return a.namingModelSetting() }),
			}},
		}},
	}
}

// namingModelSetting chooses among the providers' models with an ID, or
// none. It follows the settings, as their models' IDs are typed elsewhere.
func (a *app) namingModelSetting() core.Node {
	m := a.msgs()
	s := a.get()
	options := []ui.Option[modelReference]{{Label: m.NamingModelNone()}}
	for _, p := range s.Providers {
		for _, pm := range p.Models {
			if model := strings.TrimSpace(pm.Model); model != "" {
				options = append(options, ui.Option[modelReference]{
					Value: modelReference{Provider: p.ID, Model: pm.ID}, Label: a.providerName(p) + " · " + model})
			}
		}
	}
	return ui.Select[modelReference]{Options: options, Label: m.NamingModelChoice(), Style: core.Style{Width: core.Pt(192), MaxWidth: core.Pct(100)},
		Value:    func() modelReference { return a.get().NamingModel },
		OnChange: func(ref modelReference) { a.change(func(s *settings) { s.NamingModel = ref }) }}
}
