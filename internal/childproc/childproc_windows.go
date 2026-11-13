//go:build windows

package childproc

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// HideConsole prevents a background command from opening a console window.
// It preserves other process attributes and leaves redirected stdio intact.
// HideWindow also hides the launched program's first window, so use it only
// for console programs: a GUI application the user opens, such as the file
// manager, must keep its window.
func HideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// HideWindow alone can still let a console briefly appear. Prevent its
	// creation as well; these commands communicate through pipes, not a TTY.
	const createNoWindow = 0x08000000 // CREATE_NO_WINDOW
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

func setGroup(*exec.Cmd) {}

// terminate ends the process tree; Windows has no graceful signal for
// console processes started without a console, and a tree can no longer be
// found once its root has exited.
func terminate(cmd *exec.Cmd, done <-chan struct{}, grace time.Duration) {
	select {
	case <-done:
		return
	default:
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	HideConsole(kill)
	kill.Run()
	select {
	case <-done:
	case <-time.After(grace):
		cmd.Process.Kill()
		reaped(done)
	}
}
