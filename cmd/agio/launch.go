package main

import (
	"log"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/oneo-me/nsgo-agio/internal/launchers"
)

// The size of the title bar's buttons and their icons.
const (
	titleSize = 32
	titleIcon = 16
)

// launcherGlyph is the generic icon of kind, which the title bar shows in
// place of the applications' own icons.
func launcherGlyph(kind string) func() *core.SVG {
	switch kind {
	case launchers.Terminal:
		return lucide.SquareTerminal
	case launchers.Editor:
		return lucide.CodeXml
	case launchers.Git:
		return lucide.GitBranch
	}
	return lucide.Folder
}

// chosenLauncher resolves the setting for kind. The file manager has none:
// it is the platform's own.
func (a *app) chosenLauncher(s settings, kind string) *launchers.Installed {
	switch kind {
	case launchers.Editor:
		return launchers.Resolve(a.installed, kind, s.Editor)
	case launchers.Terminal:
		return launchers.Resolve(a.installed, kind, s.Terminal)
	case launchers.Git:
		return launchers.Resolve(a.installed, kind, s.GitTool)
	}
	return launchers.Resolve(a.installed, kind, "")
}

// launchButtons open dir in the file manager, a terminal, a development
// tool and a Git client, as the settings choose, with each kind's glyph.
func (a *app) launchButtons(dir string) []core.Node {
	if dir == "" {
		return nil
	}
	return []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
		s := a.get()
		m := a.msgs()
		var buttons []core.Node
		for _, kind := range launchers.Kinds {
			l := a.chosenLauncher(s, kind)
			if l == nil {
				continue
			}
			label := m.OpenIn(l.Name)
			buttons = append(buttons, ui.Tooltip{Text: label, Trigger: ui.IconButton{Icon: launcherGlyph(kind), Label: label,
				Size: titleSize, IconSize: titleIcon, Tint: ui.ColorsOf(cx).Text, TestID: "open-" + kind,
				OnPress: func() { launch(l, dir) }}})
		}
		return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter, Gap: 2}, Children: buttons}
	})}
}

// launch opens dir with l off the engine.
func launch(l *launchers.Installed, dir string) {
	cmd := l.Command(dir)
	go func() {
		if err := launchers.Run(cmd); err != nil {
			log.Printf("open %s in %s: %v", dir, l.Name, err)
		}
	}()
}
