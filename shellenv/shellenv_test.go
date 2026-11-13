package shellenv

import (
	"slices"
	"testing"
)

func TestWithDesktopAddsTheSessionDisplay(t *testing.T) {
	session := func() []string {
		return []string{
			"HOME=/home/me",
			"DISPLAY=:0",
			"WAYLAND_DISPLAY=wayland-0",
			"XAUTHORITY=/run/user/1000/.mutter-Xwaylandauth.ABC",
			"XDG_RUNTIME_DIR=/run/user/1000",
			"DBUS_SESSION_BUS_ADDRESS=$'unix:path=/run/user/1000/bus'",
		}
	}
	got := withDesktop([]string{"PATH=/usr/bin", "XDG_RUNTIME_DIR=/run/user/1001"}, session)
	want := []string{
		"PATH=/usr/bin",
		"XDG_RUNTIME_DIR=/run/user/1001",
		"DISPLAY=:0",
		"WAYLAND_DISPLAY=wayland-0",
		"XAUTHORITY=/run/user/1000/.mutter-Xwaylandauth.ABC",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("withDesktop = %q, want %q", got, want)
	}
}

func TestWithDesktopKeepsAnEnvironmentWithADisplay(t *testing.T) {
	session := func() []string { return []string{"DISPLAY=:0", "XAUTHORITY=/tmp/auth"} }
	env := []string{"WAYLAND_DISPLAY=wayland-1"}
	if got := withDesktop(env, session); !slices.Equal(got, env) {
		t.Fatalf("withDesktop = %q, want %q", got, env)
	}
	// Without a desktop session, nothing is added.
	none := func() []string { return []string{"XDG_RUNTIME_DIR=/run/user/1000"} }
	if got := withDesktop([]string{"PATH=/usr/bin"}, none); !slices.Equal(got, []string{"PATH=/usr/bin"}) {
		t.Fatalf("withDesktop without a session = %q", got)
	}
}
