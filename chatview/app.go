package chatview

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/chatview/locales"
)

// App is a window of every conversation of Model in a sidebar beside the
// selected one, with a button starting a new one in Dir, a directory on
// the backend's computer, or where the backend chooses when it is empty.
// It starts a conversation when there is none.
type App struct {
	Model *Model
	Dir   string
	// Header ends the window's header, such as the app's own actions.
	Header []core.Node
	TestID string
}

// Render implements core.Node.
func (a App) Render(cx *core.Scope) core.Node {
	m := a.Model
	msgs := locales.For(cx)
	colors := ui.ColorsOf(cx)
	selected := core.NewSignal(cx, "")
	open := core.NewSignal(cx, true)
	width := core.NewSignal(cx, core.Points(240))
	creating := false
	create := func() {
		if creating {
			return
		}
		creating = true
		m.Create(agio.Create{Dir: a.Dir}, func(id string) {
			creating = false
			selected.Set(id)
		})
	}
	// The selection follows the conversations: the first one when none is
	// selected or it went, and a new one when there are none.
	cx.Effect(func() {
		s := m.State()
		if s.Conversation(selected.Get()) != nil {
			return
		}
		cx.Untrack(func() {
			switch {
			case len(s.Conversations) > 0:
				selected.Set(s.Conversations[0].ID)
			case s.Err == "" && !creating:
				create()
			}
		})
	})
	title := live(m, func() string {
		if c := m.State().Conversation(selected.Get()); c != nil && c.Title != "" {
			return c.Title
		}
		return msgs.Untitled()
	})
	sections := live(m, func() []ui.SidebarSection {
		var items []ui.SidebarItem
		for _, c := range m.State().Conversations {
			id, text, icon := c.ID, c.Title, lucide.MessageSquare
			if text == "" {
				text = msgs.Untitled()
			}
			if c.Running || c.Starting {
				icon = lucide.LoaderCircle
			}
			items = append(items, ui.SidebarItem{ID: id, Text: text, Icon: icon, TestID: "conversation-" + id,
				Menu: []core.Node{ui.MenuItem{Text: msgs.RemoveConversation(), Icon: lucide.Trash, Variant: ui.MenuItemDanger,
					OnSelect: func() { m.Do(func(b agio.Backend) error { return b.Remove(id) }, nil) }}}})
		}
		return []ui.SidebarSection{{Title: msgs.Conversations(), Items: items}}
	})
	return ui.SidebarShell{TestID: a.TestID,
		Open: open.Get, OnOpenChange: open.Set, ToggleLabel: msgs.ToggleSidebar(),
		Width: width.Get, OnWidthChange: width.Set, ResizeLabel: msgs.ResizeSidebar(),
		Sidebar: []core.Node{
			ui.SidebarButton{Text: msgs.NewConversation(), Icon: lucide.SquarePen, OnPress: create, TestID: "new-conversation"},
			ui.Sidebar{SectionsFunc: sections, Selected: selected.Get, OnSelect: selected.Set, TestID: "conversations"},
		},
		Header: []core.Node{core.Text{ContentFunc: title, MaxLines: 1,
			Style:     core.Style{FontSize: 13, FontWeight: ui.Semibold, FlexShrink: 1, MinWidth: core.Pt(0)},
			StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} }}},
		Actions: a.Header,
		Content: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
			id := selected.Get()
			if id == "" {
				return core.Group(nil)
			}
			return Conversation{Model: m, ID: id, TestID: "conversation"}
		})},
	}
}
