// Package pi integrates pi coding agent 1.0 and later through its RPC mode.
// It locates the installed CLI, runs one RPC process per conversation and
// reads saved session files. It owns no UI state; callers deliver events to
// their own thread.
package pi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

// MinimumMajor is the oldest supported pi major version.
const MinimumMajor = 1

// ErrNotFound reports that no pi executable is on the login PATH.
var ErrNotFound = errors.New("pi was not found on PATH")

// Command is how to launch pi: Path with leading Args, in Env.
type Command struct {
	Path string
	Args []string
	Env  []string
}

// Locate finds pi as agent.LocateScript does.
func Locate(env []string) (Command, error) {
	path, args := agent.LocateScript("pi", env)
	if path == "" {
		return Command{}, ErrNotFound
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// Exec builds an exec.Cmd running pi with args.
func (c Command) Exec(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Path, append(append([]string{}, c.Args...), args...)...)
	childproc.HideConsole(cmd)
	cmd.Env = append(append([]string{}, c.Env...), "PI_SKIP_VERSION_CHECK=1")
	return cmd
}

// Version runs `pi --version` and returns its version string.
func (c Command) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var out, stderr bytes.Buffer
	cmd := c.Exec(ctx, "--version")
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	version := versionPattern.FindString(out.String())
	if version == "" {
		return "", fmt.Errorf("unrecognized pi version %q", strings.TrimSpace(out.String()))
	}
	return version, nil
}

var versionPattern = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// Supported reports whether version is pi 1.0 or later.
func Supported(version string) bool {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	return err == nil && n >= MinimumMajor
}
