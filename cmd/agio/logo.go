package main

import (
	_ "embed"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The app icon's layers (assets/AppIcon.icon) in currentColor, cropped to
// the bubble.
var (
	//go:embed assets/LogoBubble.svg
	logoBubbleSVG []byte
	//go:embed assets/LogoPrompt.svg
	logoPromptSVG []byte

	logoBubble = core.MustParseSVG(logoBubbleSVG)
	logoPrompt = core.MustParseSVG(logoPromptSVG)
)

// logo is the app icon heading New task in the colors shown, without its
// tile: the bubble in the accent with the prompt on it, so it follows the
// color scheme and the appearance.
type logo struct{}

// Render implements core.Node.
func (logo) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	layer := func(svg *core.SVG, color func() core.Color) core.Node {
		return core.Icon{SVG: svg, Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0),
			Width: core.Pct(100), Height: core.Pct(100)}, StyleFunc: ui.Colored(color)}
	}
	return core.View{TestID: "logo", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
		Children: []core.Node{layer(logoBubble, colors.Accent), layer(logoPrompt, colors.OnAccent)}}
}
