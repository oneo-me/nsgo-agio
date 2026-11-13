// Package childproc configures background commands and tracks long-lived
// child processes in their own process groups, so stopping one also stops
// the processes it started and quitting leaves none of them behind. It only
// ever signals processes it started and their descendants.
package childproc

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ErrShutdown reports a start requested after Shutdown.
var ErrShutdown = errors.New("application is quitting")

// entry is one started process; done closes once Wait has reaped it.
type entry struct {
	cmd  *exec.Cmd
	done chan struct{}
}

var (
	mu       sync.Mutex
	shutdown bool
	live     = map[*exec.Cmd]*entry{}
)

// Start starts cmd without a console window in a new process group and
// tracks it until Wait returns. It refuses once Shutdown has begun, so late
// starts cannot outlive quit.
func Start(cmd *exec.Cmd) error {
	HideConsole(cmd)
	SetPWD(cmd)
	setGroup(cmd)
	mu.Lock()
	defer mu.Unlock()
	if shutdown {
		return ErrShutdown
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	live[cmd] = &entry{cmd: cmd, done: make(chan struct{})}
	return nil
}

// SetPWD points an explicit environment's PWD at cmd.Dir. Go only does this
// when Env is nil, so commands given Agio's environment would otherwise
// inherit its PWD, such as / when opened from Finder.
func SetPWD(cmd *exec.Cmd) {
	if cmd.Env != nil && cmd.Dir != "" {
		cmd.Env = WithPWD(cmd.Env, cmd.Dir)
	}
}

// WithPWD returns a copy of env whose PWD is the absolute form of dir.
// Windows does not use PWD, so env is returned unchanged there.
func WithPWD(env []string, dir string) []string {
	if runtime.GOOS == "windows" {
		return env
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "PWD=") {
			out = append(out, kv)
		}
	}
	return append(out, "PWD="+dir)
}

// Wait reaps cmd and cleans up its remaining group before untracking it.
// An agent can exit without Close ever being called (for example, after a
// crash); its tools must not disappear from shutdown tracking with it.
func Wait(cmd *exec.Cmd) error {
	err := cmd.Wait()
	mu.Lock()
	e := live[cmd]
	if e != nil {
		close(e.done)
	}
	mu.Unlock()
	if e != nil {
		terminate(cmd, e.done, time.Second)
		mu.Lock()
		delete(live, cmd)
		mu.Unlock()
	}
	return err
}

// Terminate stops cmd's process group: it asks every member to exit, then
// kills the group after grace. It also stops descendants left behind by a
// leader that has already exited. It returns once the leader is reaped, or
// shortly after the kill if a process outside the group holds its output.
func Terminate(cmd *exec.Cmd, grace time.Duration) {
	if cmd.Process == nil {
		return
	}
	mu.Lock()
	e := live[cmd]
	mu.Unlock()
	done := make(chan struct{})
	if e != nil {
		done = e.done
	} else {
		close(done)
	}
	terminate(cmd, done, grace)
}

// Shutdown refuses later starts and terminates every tracked process group
// in parallel, waiting up to about grace. Callers close processes gracefully
// first; this catches whatever is still running, such as agents that were
// starting when the application quit.
func Shutdown(grace time.Duration) {
	mu.Lock()
	shutdown = true
	cmds := make([]*exec.Cmd, 0, len(live))
	for cmd := range live {
		cmds = append(cmds, cmd)
	}
	mu.Unlock()
	var wg sync.WaitGroup
	for _, cmd := range cmds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Terminate(cmd, grace)
		}()
	}
	wg.Wait()
}

// reaped waits a short while for done after a kill, without hanging when a
// process outside the group keeps the output open.
func reaped(done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}
