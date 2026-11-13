//go:build !windows

package childproc

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// started is a shell run by Start whose background child's PID it printed.
type started struct {
	cmd    *exec.Cmd
	stdin  io.Closer
	pid    int
	exited chan error
}

// startShell runs script, which prints its background child's PID first.
func startShell(t *testing.T, script string) started {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", script)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- Wait(cmd) }()
	t.Cleanup(func() {
		stdin.Close()
		syscall.Kill(pid, syscall.SIGKILL)
	})
	return started{cmd, stdin, pid, exited}
}

// gone waits for pid to disappear; an orphan is reaped by init shortly
// after it dies.
func gone(pid int) bool {
	for range 100 {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestTerminateStopsDescendants(t *testing.T) {
	s := startShell(t, "sleep 60 >/dev/null & echo $!; wait")
	Terminate(s.cmd, 2*time.Second)
	select {
	case <-s.exited:
	case <-time.After(time.Second):
		t.Fatal("leader was not reaped")
	}
	if !gone(s.pid) {
		t.Fatalf("descendant %d survived", s.pid)
	}
}

func TestTerminateKillsAfterGrace(t *testing.T) {
	s := startShell(t, "trap '' TERM; sleep 60 >/dev/null & echo $!; while :; do sleep 1; done")
	start := time.Now()
	Terminate(s.cmd, 300*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("terminate took %v", elapsed)
	}
	if !gone(s.pid) {
		t.Fatalf("descendant %d survived", s.pid)
	}
}

func TestWaitCleansUpAfterExitedLeader(t *testing.T) {
	s := startShell(t, "sleep 60 >/dev/null & echo $!; read x")
	// Closing stdin ends the leader, like an agent asked to shut down.
	s.stdin.Close()
	<-s.exited
	// No explicit Terminate: an unexpectedly exited agent is removed from
	// the UI's sessions, so quit can no longer close it through that path.
	if !gone(s.pid) {
		t.Fatalf("descendant %d survived", s.pid)
	}
}

func TestShutdownTerminatesAndRefuses(t *testing.T) {
	t.Cleanup(func() {
		mu.Lock()
		shutdown = false
		mu.Unlock()
	})
	s := startShell(t, "sleep 60 >/dev/null & echo $!; wait")
	Shutdown(time.Second)
	<-s.exited
	if !gone(s.pid) {
		t.Fatalf("descendant %d survived", s.pid)
	}
	if err := Start(exec.Command("/bin/true")); !errors.Is(err, ErrShutdown) {
		t.Fatalf("start after shutdown: %v", err)
	}
}

func TestStartSetsPWDFromDir(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", `printf %s "$PWD"`)
	cmd.Dir, cmd.Env = dir, []string{"PWD=/", "PATH=/usr/bin:/bin"}
	var out strings.Builder
	cmd.Stdout = &out
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	if err := Wait(cmd); err != nil {
		t.Fatal(err)
	}
	if out.String() != dir {
		t.Fatalf("PWD = %q, want %q", out.String(), dir)
	}
}
