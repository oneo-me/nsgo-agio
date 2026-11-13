//go:build !windows

package childproc

import (
	"os/exec"
	"syscall"
	"time"
)

// HideConsole is a no-op on platforms where child processes do not create
// console windows.
func HideConsole(*exec.Cmd) {}

func setGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// terminate signals the group whose ID is the leader's PID. The ID stays
// reserved while any member is alive, so signals reach only that group; once
// it is empty the kernel reports ESRCH.
func terminate(cmd *exec.Cmd, done <-chan struct{}, grace time.Duration) {
	pgid := cmd.Process.Pid
	if syscall.Kill(-pgid, syscall.SIGTERM) != nil {
		reaped(done)
		return
	}
	deadline := time.After(grace)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			syscall.Kill(-pgid, syscall.SIGKILL)
			reaped(done)
			return
		case <-tick.C:
		}
		select {
		case <-done:
			if syscall.Kill(-pgid, 0) != nil {
				return
			}
		default:
		}
	}
}
