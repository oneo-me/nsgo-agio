//go:build windows

package childproc_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/oneo-me/nsgo-agio/agent/claude"
	"github.com/oneo-me/nsgo-agio/agent/codex"
	"github.com/oneo-me/nsgo-agio/agent/pi"
	"github.com/oneo-me/nsgo-agio/internal/childproc"
)

const consoleHelperEnv = "AGIO_TEST_CONSOLE_HELPER"

// The test executable doubles as a console CLI, without installing an agent
// or touching the user's agent configuration.
func TestMain(m *testing.M) {
	if os.Getenv(consoleHelperEnv) == "1" {
		os.Exit(consoleHelper())
	}
	os.Exit(m.Run())
}

func consoleHelper() int {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	window, _, _ := kernel.NewProc("GetConsoleWindow").Call()
	codepage, _, _ := kernel.NewProc("GetConsoleCP").Call()
	if window != 0 || codepage != 0 {
		fmt.Fprintln(os.Stderr, "unexpected console")
		return 1
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("1.2.3")
		return 0
	}
	fmt.Println("ready")
	if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Fprintln(os.Stderr, "diagnostic")
	return 0
}

func consoleHelperPath(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHideConsolePreservesAttributes(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			cmd := &exec.Cmd{}
			var original *syscall.SysProcAttr
			if existing {
				original = &syscall.SysProcAttr{
					CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
					CmdLine:       "custom command line",
					Token:         syscall.Token(123),
				}
				cmd.SysProcAttr = original
			}
			childproc.HideConsole(cmd)
			childproc.HideConsole(cmd)      // Reapplying must be harmless.
			wantFlags := uint32(0x08000000) // CREATE_NO_WINDOW
			if existing {
				wantFlags |= syscall.CREATE_NEW_PROCESS_GROUP
				if cmd.SysProcAttr != original || original.CmdLine != "custom command line" || original.Token != 123 {
					t.Fatalf("attributes replaced: %+v", cmd.SysProcAttr)
				}
			}
			if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags != wantFlags {
				t.Fatalf("console attributes: %+v", cmd.SysProcAttr)
			}
		})
	}
}

func TestBackgroundCommandPipes(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprintf("tracked=%v", tracked), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, consoleHelperPath(t))
			cmd.Env = append(os.Environ(), consoleHelperEnv+"=1")
			cmd.Stdin = strings.NewReader("request\n")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			var err error
			if tracked {
				// Start must apply the policy even when the caller did not.
				if err = childproc.Start(cmd); err == nil {
					err = childproc.Wait(cmd)
				}
			} else {
				childproc.HideConsole(cmd)
				err = cmd.Run()
			}
			if err != nil || stdout.String() != "ready\nrequest\n" || stderr.String() != "diagnostic\n" {
				t.Fatalf("run: %v; stdout=%q stderr=%q", err, &stdout, &stderr)
			}
		})
	}
}

func TestBackgroundCommandStopping(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprintf("tracked=%v", tracked), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, consoleHelperPath(t))
			cmd.Env = append(os.Environ(), consoleHelperEnv+"=1")
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			start, wait := cmd.Start, cmd.Wait
			if tracked {
				start = func() error { return childproc.Start(cmd) }
				wait = func() error { return childproc.Wait(cmd) }
			} else {
				childproc.HideConsole(cmd)
			}
			if err := start(); err != nil {
				t.Fatal(err)
			}
			// Read before Wait, as required by StdoutPipe. The helper blocks on
			// stdin until cancelled or terminated, not until a timer expires.
			line, readErr := bufio.NewReader(stdout).ReadString('\n')
			done := make(chan error, 1)
			go func() { done <- wait() }()
			if tracked {
				childproc.Terminate(cmd, time.Second)
			} else {
				cancel()
			}
			err = <-done
			if readErr != nil || line != "ready\n" || err == nil || ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("stop: %v; ready=%q (%v); stderr=%q; context=%v", err, line, readErr, &stderr, ctx.Err())
			}
		})
	}
}

func TestAgentVersionProbesWithoutConsole(t *testing.T) {
	path := consoleHelperPath(t)
	env := append(os.Environ(), consoleHelperEnv+"=1")
	probes := map[string]func(context.Context) (string, error){
		"pi":     (pi.Command{Path: path, Env: env}).Version,
		"codex":  (&codex.Driver{Path: path, Env: env}).Version,
		"claude": (&claude.Driver{Path: path, Env: env}).Version,
	}
	for name, probe := range probes {
		t.Run(name, func(t *testing.T) {
			version, err := probe(context.Background())
			if err != nil || version != "1.2.3" {
				t.Fatalf("version=%q, error=%v", version, err)
			}
		})
	}
}
