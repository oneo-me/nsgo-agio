package main

import (
	"runtime"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// colorScheme is one of Agio's built-in looks for an appearance, which the
// settings choose apart for light and dark: a soft gradient over the
// window's chrome, cards of a quiet page color floating on it, the text
// and an accent.
type colorScheme struct {
	ID   string
	Dark bool
	// Plain leaves a window material showing as it is, untinted.
	Plain bool
	// Chrome are the gradient's colors, from the window's top left to its
	// bottom right.
	Chrome             []core.Color
	Page, Text, Accent core.Color
}

// The schemes, the first of each the default.
var (
	lightSchemes = []colorScheme{
		{ID: "mist", Plain: true, Chrome: rgb(0xe6ebf3, 0xeceef3, 0xf1eee8), Page: core.RGB(0xffffff), Text: core.RGB(0x1f2329), Accent: core.RGB(0x2f6bff)},
		{ID: "blossom", Chrome: rgb(0xf1dcef, 0xf5e6e6, 0xf4ead2), Page: core.RGB(0xfffcfb), Text: core.RGB(0x2b2327), Accent: core.RGB(0xc8417c)},
		{ID: "lagoon", Chrome: rgb(0xd5edf1, 0xdfeafb, 0xebe5fa), Page: core.RGB(0xfcfeff), Text: core.RGB(0x1b2a33), Accent: core.RGB(0x0b8bad)},
		{ID: "meadow", Chrome: rgb(0xdcefda, 0xe9f1d8, 0xf6edd5), Page: core.RGB(0xfefffb), Text: core.RGB(0x1f2a1f), Accent: core.RGB(0x2b9653)},
		{ID: "sunset", Chrome: rgb(0xfcdcc6, 0xf9d6de, 0xe6d7f4), Page: core.RGB(0xfffbf9), Text: core.RGB(0x2e2220), Accent: core.RGB(0xe8622a)},
		{ID: "citrus", Chrome: rgb(0xfbefb8, 0xeef3bf, 0xd5efcf), Page: core.RGB(0xfffef7), Text: core.RGB(0x2a2814), Accent: core.RGB(0xd08300)},
		{ID: "ocean", Chrome: rgb(0xc9e2fb, 0xd0eef4, 0xd9f4e6), Page: core.RGB(0xfbfdff), Text: core.RGB(0x182634), Accent: core.RGB(0x1677ff)},
		{ID: "lilac", Chrome: rgb(0xdcd3fa, 0xe8d5f6, 0xf5daec), Page: core.RGB(0xfdfbff), Text: core.RGB(0x261f38), Accent: core.RGB(0x7c4dff)},
	}
	darkSchemes = []colorScheme{
		{ID: "graphite", Dark: true, Plain: true, Chrome: rgb(0x17181c, 0x1c1d22, 0x202127), Page: core.RGB(0x26272d), Text: core.RGB(0xe8e8eb), Accent: core.RGB(0x5b95ff)},
		{ID: "aurora", Dark: true, Chrome: rgb(0x0d2629, 0x111c33, 0x221737), Page: core.RGB(0x1a2130), Text: core.RGB(0xe2ebf5), Accent: core.RGB(0x4fd1c5)},
		{ID: "dusk", Dark: true, Chrome: rgb(0x261533, 0x33182f, 0x36201c), Page: core.RGB(0x271f2c), Text: core.RGB(0xf1e6f2), Accent: core.RGB(0xe879b9)},
		{ID: "ember", Dark: true, Chrome: rgb(0x26130d, 0x2e170d, 0x251710), Page: core.RGB(0x2a1e19), Text: core.RGB(0xf6e5da), Accent: core.RGB(0xff8a4c)},
		{ID: "nebula", Dark: true, Chrome: rgb(0x191342, 0x29134f, 0x3a1241), Page: core.RGB(0x1e1a35), Text: core.RGB(0xece6ff), Accent: core.RGB(0x8b7bff)},
		{ID: "moss", Dark: true, Chrome: rgb(0x12251a, 0x1b2c15, 0x292b16), Page: core.RGB(0x1c2620), Text: core.RGB(0xe3eedf), Accent: core.RGB(0x8fce5c)},
		{ID: "crimson", Dark: true, Chrome: rgb(0x2c0d17, 0x360f1d, 0x2e1124), Page: core.RGB(0x2a1a20), Text: core.RGB(0xf6e3e8), Accent: core.RGB(0xff5c7a)},
		{ID: "ink", Dark: true, Chrome: rgb(0x0d1a26, 0x10232e, 0x0d2728), Page: core.RGB(0x18222b), Text: core.RGB(0xdceaf0), Accent: core.RGB(0x38bdf8)},
	}
)

func rgb(hex ...uint32) []core.Color {
	out := make([]core.Color, len(hex))
	for i, h := range hex {
		out[i] = core.RGB(h)
	}
	return out
}

// scheme is the scheme of schemes with id, else the first.
func scheme(schemes []colorScheme, id string) colorScheme {
	for _, s := range schemes {
		if s.ID == id {
			return s
		}
	}
	return schemes[0]
}

// theme is the app's theme of the schemes chosen for each appearance.
func theme(s settings) *core.Theme {
	t := core.DefaultTheme()
	t.Light = scheme(lightSchemes, s.LightScheme).palette()
	t.Dark = scheme(darkSchemes, s.DarkScheme).palette()
	return t
}

// chrome paints the window's chrome with the gradient of the scheme
// shown, for ui.SidebarShell's ChromeStyle; it follows the appearance and
// the settings. Over a window material the gradient tints the blur
// showing through it, which grays it, so it takes more of the accent
// there, and a plain scheme leaves the blur as it is; else it is opaque.
func (a *app) chrome() core.Style {
	s := a.get()
	sc := scheme(lightSchemes, s.LightScheme)
	if a.cx.Dark() {
		sc = scheme(darkSchemes, s.DarkScheme)
	}
	tint, accent, fill := float32(1), float32(0), sc.Chrome[0]
	if desktop.ShowsMaterial(a.cx) {
		if sc.Plain {
			return core.Style{}
		}
		tint, accent, fill = 0.6, 0.16, 0
		if sc.Dark {
			tint, accent = 0.7, 0.12
		}
	}
	stops := make([]core.ColorStop, len(sc.Chrome))
	for i, c := range sc.Chrome {
		stops[i] = core.ColorStop{Color: ui.Alpha(ui.Mix(c, sc.Accent, accent), tint), At: core.Pct(float32(100 * i / max(len(sc.Chrome)-1, 1)))}
	}
	return core.Style{Background: fill,
		Gradients: []core.Gradient{{Kind: core.GradientLinear, ToCorner: core.ToBottomRight, Stops: stops}}}
}

// windowMaterial is the translucent blur behind the window: macOS's
// sidebar vibrancy, and Acrylic elsewhere, as Mica shows no blur.
func windowMaterial() core.WindowMaterial {
	if runtime.GOOS == "darwin" {
		return core.WindowMaterialSidebar
	}
	return core.WindowMaterialAcrylic
}

// palette is the scheme over NSGo's default palette of its appearance.
// Insets and fields are the page tinted with the chrome; the accent darkens
// under the pointer in light and lightens in dark.
func (sc colorScheme) palette() core.Palette {
	text, page, accent := sc.Text, sc.Page, sc.Accent
	mid := sc.Chrome[len(sc.Chrome)/2]
	black, white := core.RGB(0x000000), core.RGB(0xffffff)
	p := core.DefaultTheme().Light
	surface, raised, field := page, page, page
	inset := ui.Mix(page, mid, 0.55)
	hover, pressed := ui.Mix(accent, black, 0.1), ui.Mix(accent, black, 0.2)
	onAccent, scrollbar := white, float32(0.30)
	if sc.Dark {
		p = core.DefaultTheme().Dark
		surface, raised, field = ui.Mix(page, text, 0.04), ui.Mix(page, text, 0.08), ui.Mix(page, black, 0.15)
		inset = ui.Mix(page, mid, 0.5)
		hover, pressed = ui.Mix(accent, white, 0.12), ui.Mix(accent, black, 0.12)
		onAccent, scrollbar = ui.Mix(page, black, 0.5), 0.35
	}
	p.Background, p.Surface = page, surface
	p.SurfaceHover, p.SurfacePressed = text.Faded(0.07), text.Faded(0.12)
	p.Border, p.Text, p.TextMuted = text.Faded(0.14), text, text.Faded(0.58)
	p.Accent, p.AccentHover, p.AccentPressed, p.AccentText = accent, hover, pressed, onAccent
	p.Selection, p.Focus, p.Scrollbar = ui.Alpha(accent, 0.24), ui.Alpha(accent, 0.6), text.Faded(scrollbar)
	for name, c := range map[string]core.Color{
		"inset": inset, "raised": raised, "field": field, "fieldBorder": text.Faded(0.14),
		"line": text.Faded(0.10), "faint": text.Faded(0.38), "accentFill": accent,
		"light": text, "onLight": page, "code": text,
	} {
		p.SetColor(name, c)
	}
	return p
}

// schemeSetting chooses one of schemes, named label, into the setting
// field returns, each shown as squares of its common colors and tested as
// id and its own. The theme holds both appearances' schemes, so a choice
// shows at once in its appearance and waits for it in the other.
func (a *app) schemeSetting(id, label string, field func(*settings) *string, schemes []colorScheme) core.Node {
	chosen := func() string { s := a.get(); return scheme(schemes, *field(&s)).ID }
	var options []core.Node
	for _, sc := range schemes {
		options = append(options, a.schemeOption(id+"-"+sc.ID, sc, func() bool { return chosen() == sc.ID }, func() {
			a.change(func(s *settings) { *field(s) = sc.ID })
			a.cx.SetTheme(theme(a.st.get()))
		}))
	}
	return base.RadioGroup{Label: label, Children: options,
		Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 4}}
}

// schemeOption is a scheme's choice: its chrome's colors, page and
// accent as squares over its name, tinted with the accent shown while
// chosen.
func (a *app) schemeOption(testID string, sc colorScheme, checked func() bool, choose func()) core.Node {
	name := a.schemeName(sc.ID)
	edge := sc.Text.Faded(0.14)
	var squares []core.Node
	for _, c := range append(append([]core.Color{}, sc.Chrome...), sc.Page, sc.Accent) {
		squares = append(squares, core.View{Style: core.Style{Width: core.Pt(12), Height: core.Pt(12), Background: c,
			CornerRadius: 3, BorderWidth: 1, BorderColor: edge}})
	}
	return core.Func(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		return base.Radio{Label: name, TestID: testID, Checked: checked, OnChange: func(bool) { choose() },
			StyleFunc: func() core.Style {
				// The chosen option keeps a hover style of its own color, as a
				// view without one stops hearing the pointer leave and would
				// show it hovered once another is chosen.
				st := core.Style{Gap: 6, Padding: core.Symmetric(8, 10), AlignItems: core.AlignCenter, CornerRadius: ui.RadiusControl,
					Hover: &core.Style{Background: colors.Hover()}}
				if checked() {
					st.Background = ui.TintsOf(cx).Accent()
					st.Hover = &core.Style{Background: st.Background}
				}
				return st
			},
			Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, Gap: 3}, Children: squares},
				core.Text{Content: name, MaxLines: 1, Style: core.Style{FontSize: 12}, StyleFunc: func() core.Style {
					if checked() {
						return core.Style{Color: colors.Accent(), FontWeight: ui.Medium}
					}
					return core.Style{Color: colors.Muted()}
				}},
			}}
	})
}

// schemeName names a scheme in the language shown.
func (a *app) schemeName(id string) string {
	m := a.msgs()
	switch id {
	case "mist":
		return m.SchemeMist()
	case "blossom":
		return m.SchemeBlossom()
	case "lagoon":
		return m.SchemeLagoon()
	case "meadow":
		return m.SchemeMeadow()
	case "sunset":
		return m.SchemeSunset()
	case "graphite":
		return m.SchemeGraphite()
	case "aurora":
		return m.SchemeAurora()
	case "dusk":
		return m.SchemeDusk()
	case "ember":
		return m.SchemeEmber()
	case "citrus":
		return m.SchemeCitrus()
	case "ocean":
		return m.SchemeOcean()
	case "lilac":
		return m.SchemeLilac()
	case "nebula":
		return m.SchemeNebula()
	case "moss":
		return m.SchemeMoss()
	case "crimson":
		return m.SchemeCrimson()
	case "ink":
		return m.SchemeInk()
	}
	return id
}
