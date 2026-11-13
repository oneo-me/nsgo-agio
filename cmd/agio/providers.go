package main

import (
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// modelProvider is an OpenAI-compatible model service. IDs are stable
// local identities, not display names or remote model IDs. Text keeps its
// spacing while edited and is trimmed only when used.
type modelProvider struct {
	ID      string          `json:"id"`
	Name    string          `json:"name,omitempty"`
	BaseURL string          `json:"baseURL,omitempty"`
	APIKey  string          `json:"apiKey,omitempty"`
	Models  []providerModel `json:"models,omitempty"`
}

type providerModel struct {
	ID    string `json:"id"`
	Model string `json:"model,omitempty"`
}

// defaultBaseURL is the base URL a provider without one uses.
const defaultBaseURL = "https://api.openai.com/v1"

const (
	providerCategoryPrefix = "provider:"
	// noProvidersCategory stands in for the providers until one is added.
	noProvidersCategory = "providers"
)

func providerCategory(id string) string { return providerCategoryPrefix + id }

// editProvider copies both slice levels before editing, as the settings'
// copies share them.
func (s *settings) editProvider(id string, edit func(*modelProvider)) {
	providers := slices.Clone(s.Providers)
	for i := range providers {
		if providers[i].ID == id {
			providers[i].Models = slices.Clone(providers[i].Models)
			edit(&providers[i])
			s.Providers = providers
			return
		}
	}
}

func (s *settings) addProvider(name string) string {
	id := newID()
	s.Providers = append(slices.Clone(s.Providers), modelProvider{ID: id, Name: name})
	return id
}

func (s *settings) removeProvider(id string) {
	s.Providers = slices.DeleteFunc(slices.Clone(s.Providers), func(p modelProvider) bool { return p.ID == id })
}

func (s *settings) addProviderModel(id string) {
	s.editProvider(id, func(p *modelProvider) {
		p.Models = append(p.Models, providerModel{ID: newID()})
	})
}

func (s *settings) removeProviderModel(provider, model string) {
	s.editProvider(provider, func(p *modelProvider) {
		p.Models = slices.DeleteFunc(p.Models, func(m providerModel) bool { return m.ID == model })
	})
}

func (a *app) providerName(p modelProvider) string {
	if name := strings.TrimSpace(p.Name); name != "" {
		return name
	}
	return a.msgs().ProviderUnnamed()
}

// showSettingsPage goes on to a page of the settings.
func (a *app) showSettingsPage(category string) { a.category.Set(category) }

// providerCategories give each provider a page under Model services, or a
// note in the sidebar that none is added yet; the section's heading adds them.
func (a *app) providerCategories() []ui.SettingsCategory {
	m := a.msgs()
	section := m.ModelServices()
	providers := a.st.get().Providers
	if len(providers) == 0 {
		return []ui.SettingsCategory{{
			ID: noProvidersCategory, Title: m.ProviderEmpty(), Section: section,
			Placeholder: core.Func(func(cx *core.Scope) core.Node {
				t := ui.Text(m.ProviderEmpty(), 11.375, ui.Regular, ui.ColorsOf(cx).Muted)
				t.Style = t.Style.Merge(core.Style{FlexShrink: 1, MinWidth: core.Pt(0)})
				return t
			}),
		}}
	}
	if core.Untracked(a.cx, a.category.Get) == noProvidersCategory {
		a.showSettingsPage(providerCategory(providers[0].ID))
	}
	var categories []ui.SettingsCategory
	for _, p := range providers {
		categories = append(categories, ui.SettingsCategory{
			ID: providerCategory(p.ID), Title: a.providerName(p), Section: section, Icon: lucide.Cloud,
			Groups: a.providerGroups(p),
		})
	}
	return categories
}

// addProviderButton is the Model services heading's add button, listing the
// kinds of provider to add, which goes on to the page of the provider added.
func (a *app) addProviderButton(cx *core.Scope) core.Node {
	m := a.msgs()
	label := m.AddProvider()
	return ui.Menu{Trigger: ui.Button{Variant: ui.ButtonGhost, Icon: lucide.Plus, Label: label, Side: 20, IconSize: 11.375, Tint: ui.ColorsOf(cx).Muted}, Label: label, Children: []core.Node{
		ui.MenuItem{Text: m.ProviderType(), OnSelect: func() {
			var id string
			a.change(func(s *settings) { id = s.addProvider(m.ProviderType()) })
			a.showSettingsPage(providerCategory(id))
		}}}}
}

// providerGroups are a provider's connection, models and removal.
func (a *app) providerGroups(p modelProvider) []ui.SettingsGroup {
	m := a.msgs()
	edit := func(fn func(*modelProvider)) {
		a.change(func(s *settings) { s.editProvider(p.ID, fn) })
	}
	field := func(id, label, initial, placeholder string, secret bool, set func(*modelProvider, string)) ui.SettingsItem {
		return ui.SettingsItem{
			ID: id, Title: label,
			Keywords: []string{a.providerName(p), "OpenAI", "provider", "提供商"},
			Control: textInput(initial, label, placeholder, secret, func(value string) {
				edit(func(p *modelProvider) { set(p, value) })
			}),
		}
	}
	groups := []ui.SettingsGroup{{
		ID: "connection", Items: []ui.SettingsItem{
			field("name", m.ProviderName(), p.Name, m.ProviderNamePlaceholder(), false, func(p *modelProvider, v string) { p.Name = v }),
			field("url", m.ProviderBaseUrl(), p.BaseURL, defaultBaseURL, false, func(p *modelProvider, v string) { p.BaseURL = v }),
			field("key", m.ProviderApiKey(), p.APIKey, "sk-…", true, func(p *modelProvider, v string) { p.APIKey = v }),
		},
	}}
	models := ui.SettingsGroup{
		ID: "models", Title: m.ProviderModels(),
		Actions: []core.Node{ui.Button{Text: m.AddProviderModel(), Variant: ui.ButtonSecondary, OnPress: func() {
			a.change(func(s *settings) { s.addProviderModel(p.ID) })
		}}},
	}
	for i, pm := range p.Models {
		label := m.ProviderModelLabel(i + 1)
		models.Items = append(models.Items, ui.SettingsItem{
			ID: pm.ID, Title: label, Keywords: []string{pm.Model, a.providerName(p)},
			Control: core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, MinWidth: core.Pt(0)}, Children: []core.Node{
				textInput(pm.Model, label, m.ProviderModelPlaceholder(), false, func(value string) {
					edit(func(p *modelProvider) {
						for i := range p.Models {
							if p.Models[i].ID == pm.ID {
								p.Models[i].Model = value
							}
						}
					})
				}),
				ui.IconButton{Icon: lucide.Trash, Size: titleSize, IconSize: 16.25, Label: m.RemoveProviderModel(i + 1), OnPress: func() {
					a.change(func(s *settings) { s.removeProviderModel(p.ID, pm.ID) })
				}}}},
		})
	}
	if len(models.Items) == 0 {
		models.Items = []ui.SettingsItem{{ID: "empty", Title: m.ProviderModelsEmpty()}}
	}
	return append(groups, models, ui.SettingsGroup{
		ID: "remove", Items: []ui.SettingsItem{{
			ID: "remove", Title: m.RemoveProvider(), Description: m.RemoveProviderDescription(),
			Control: ui.Button{Text: m.RemoveProvider(), Variant: ui.ButtonSecondary, OnPress: func() { a.removeProvider(p.ID) }},
		}},
	})
}

// removeProvider removes a provider and goes on to the page of the one
// listed in its place, or else the last one.
func (a *app) removeProvider(id string) {
	var next string
	a.change(func(s *settings) {
		i := slices.IndexFunc(s.Providers, func(p modelProvider) bool { return p.ID == id })
		s.removeProvider(id)
		next = noProvidersCategory
		if n := len(s.Providers); n > 0 {
			next = providerCategory(s.Providers[min(max(i, 0), n-1)].ID)
		}
	})
	a.showSettingsPage(next)
}
