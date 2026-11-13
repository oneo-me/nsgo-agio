// Package shellenv reads the user's login-shell environment. Desktop apps
// started from Finder or a launcher inherit a minimal PATH, while pi, Git and
// their runtimes are normally configured in the login shell.
package shellenv

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"
)

// Load returns the login shell's environment, falling back to the process
// environment. It blocks for up to a few seconds; call it off the UI thread.
func Load() []string {
	env := login()
	if runtime.GOOS == "linux" {
		env = withDesktop(env, userManagerEnvironment)
	}
	return env
}

func login() []string {
	if runtime.GOOS == "windows" {
		return os.Environ()
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const marker = "__AGIO_ENV__"
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", "printf '"+marker+"'; env -0")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return os.Environ()
	}
	_, data, ok := bytes.Cut(out, []byte(marker))
	if !ok {
		return os.Environ()
	}
	return Merge(os.Environ(), strings.Split(strings.TrimRight(string(data), "\x00"), "\x00"))
}

// Merge overlays login values on the process environment.
func Merge(base, login []string) []string {
	values := map[string]string{}
	var order []string
	for _, list := range [][]string{base, login} {
		for _, kv := range list {
			k, _, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				continue
			}
			if _, seen := values[k]; !seen {
				order = append(order, k)
			}
			values[k] = kv
		}
	}
	env := make([]string, 0, len(order))
	for _, k := range order {
		env = append(env, values[k])
	}
	return env
}

// desktopKeys are what a graphical program needs to reach the desktop
// session.
var desktopKeys = []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"}

// withDesktop adds the signed-in desktop session's display, as session
// reports it, to an environment without one, such as an SSH session's, so
// the windows of graphical programs commands start show on that desktop.
// Values env already has stay.
func withDesktop(env []string, session func() []string) []string {
	has := map[string]bool{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		has[k] = v != ""
	}
	if has["DISPLAY"] || has["WAYLAND_DISPLAY"] {
		return env
	}
	var add []string
	display := false
	for _, kv := range session() {
		k, v, ok := strings.Cut(kv, "=")
		// systemd quotes values with special characters, which no display
		// variable needs.
		if !ok || v == "" || has[k] || !slices.Contains(desktopKeys, k) || strings.HasPrefix(v, "$'") {
			continue
		}
		add = append(add, kv)
		display = display || k == "DISPLAY" || k == "WAYLAND_DISPLAY"
	}
	if !display {
		return env
	}
	return append(slices.Clip(env), add...)
}

// userManagerEnvironment is the systemd user manager's environment, into
// which desktop sessions import their display.
func userManagerEnvironment() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "show-environment").Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}
