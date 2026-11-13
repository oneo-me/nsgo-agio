package chatview

import (
	"runtime"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	menus "github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/chatview/locales"
)

// App is a window of every conversation of Model in a sidebar beside the
// selected one, with a button starting a new one and the settings. A new
// conversation works where the selected one does, else in Dir, a directory
// on the backend's computer, or where the backend chooses when it is empty.
// It starts a conversation when there is none.
//
// Name, when set, heads the menu bar on macOS: the app's menu with the
// settings, a File menu starting a conversation, and the platform's Edit
// and Window menus.
type App struct {
	Model *Model
	Dir   string
	Name  string
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
	settingsOpen := core.NewSignal(cx, false)
	dir := func() string {
		if c := m.State().Conversation(selected.Get()); c != nil && c.Dir != "" {
			return c.Dir
		}
		return a.Dir
	}
	creating := false
	// failed is the directory starting a conversation in failed, which is
	// not retried on its own.
	failed := ""
	create := func() {
		if creating {
			return
		}
		creating = true
		d := core.Untracked(cx, dir)
		var id string
		m.Do(func(b agio.Backend) (err error) {
			id, err = b.Create(agio.Create{Dir: d})
			return err
		}, func(err error) {
			creating = false
			if err != nil {
				failed = d
				return
			}
			failed = ""
			m.refresh()
			selected.Set(id)
		})
	}
	// The selection follows the conversations: the first one when none is
	// selected or it went, and a new one when there are none.
	cx.Effect(func() {
		s := m.State()
		list := s.Conversations
		if s.Conversation(selected.Get()) != nil {
			return
		}
		err, d := s.Err, a.Dir
		cx.Untrack(func() {
			switch {
			case len(list) > 0:
				selected.Set(list[0].ID)
			case err == "" && d != failed:
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
	newConversation := func() { failed = ""; create() }
	if a.Name != "" && runtime.GOOS == "darwin" {
		cx.Effect(func() {
			desktop.SetApplicationMenu(cx, appMenu(a.Name, locales.For(cx), menus.For(cx), newConversation, func() { settingsOpen.Set(true) }))
		})
	}
	shell := ui.SidebarShell{TestID: a.TestID,
		Open: open.Get, OnOpenChange: open.Set, ToggleLabel: msgs.ToggleSidebar(),
		Width: width.Get, OnWidthChange: width.Set, ResizeLabel: msgs.ResizeSidebar(),
		Sidebar: []core.Node{
			ui.SidebarButton{Text: msgs.NewConversation(), Icon: lucide.SquarePen, TestID: "new-conversation",
				OnPress: newConversation},
			ui.Sidebar{SectionsFunc: sections, Selected: selected.Get, OnSelect: selected.Set, TestID: "conversations"},
		},
		SidebarFooter: []core.Node{ui.SidebarButton{Text: msgs.Settings(), Icon: lucide.Settings, TestID: "open-settings",
			OnPress: func() { settingsOpen.Set(true) }}},
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
	return core.Group([]core.Node{shell, a.settings(cx, settingsOpen)})
}

// appMenu is the menu bar in the language shown, of chatview's words and
// NSGo's; the platform labels the role items.
func appMenu(name string, msgs locales.Messages, words menus.Messages, newConversation, openSettings func()) []desktop.MenuItem {
	return []desktop.MenuItem{
		{Label: name, Items: []desktop.MenuItem{
			{Role: desktop.MenuAbout},
			{Separator: true},
			{Label: msgs.SettingsMenuItem(), Shortcut: "mod+,", OnSelect: openSettings},
			{Separator: true},
			{Role: desktop.MenuHide}, {Role: desktop.MenuHideOthers}, {Role: desktop.MenuShowAll},
			{Separator: true},
			{Role: desktop.MenuQuit},
		}},
		{Label: msgs.FileMenu(), Items: []desktop.MenuItem{
			{Label: msgs.NewConversation(), Shortcut: "mod+n", OnSelect: newConversation},
			{Separator: true},
			{Role: desktop.MenuClose},
		}},
		{Label: words.Edit(), Items: []desktop.MenuItem{
			{Role: desktop.MenuUndo}, {Role: desktop.MenuRedo},
			{Separator: true},
			{Role: desktop.MenuCut}, {Role: desktop.MenuCopy}, {Role: desktop.MenuPaste}, {Role: desktop.MenuSelectAll},
		}},
		{Label: words.Window(), Items: []desktop.MenuItem{
			{Role: desktop.MenuMinimize}, {Role: desktop.MenuZoom}, {Role: desktop.MenuToggleFullScreen},
		}},
	}
}
