package main

import "github.com/nsgo-dev/nsgo/core"

// grainProgram scatters light and dark specks, one a pixel, which keep their
// places as the window resizes: a film grain over the window's chrome.
var grainProgram = core.MustCompileShader(`package main

var Amount float

// hash is Dave Hoskins's hash without sine, whose precision draws lines.
func hash(p vec2) float {
	p3 := fract(vec3(p.x, p.y, p.x) * 0.1031)
	p3 += dot(p3, p3.yzx+33.33)
	return fract((p3.x + p3.y) * p3.z)
}

func Fragment(dst vec4, pos vec2) vec4 {
	n := hash(floor(dst.xy))
	a := Amount * abs(n-0.5) * 2
	if n > 0.5 {
		return vec4(a, a, a, a)
	}
	return vec4(0, 0, 0, a)
}
`)

// The grain's strength the settings start from, and the opacity of its
// strongest specks at 100%.
const (
	defaultGrain = 40
	grainOpacity = 0.16
)

// grainAmount is the grain's strength from 1 to 100.
func (s settings) grainAmount() int {
	if s.GrainAmount <= 0 {
		return defaultGrain
	}
	return min(s.GrainAmount, 100)
}

// grain is the window's grain while the settings turn it on, for
// ui.SidebarShell's Backdrop.
func (a *app) grain() []core.Node {
	return []core.Node{core.Show{When: func() bool { return a.get().Grain }, Then: func(cx *core.Scope) core.Node {
		return core.Shader{Program: grainProgram, TestID: "grain",
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			UniformsFunc: func() core.Uniforms {
				return core.Uniforms{"Amount": float64(a.get().grainAmount()) / 100 * grainOpacity}
			}}
	}}}
}
