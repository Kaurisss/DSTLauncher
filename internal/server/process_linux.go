//go:build linux

package server

import (
	"errors"
	"os/exec"
	"syscall"
)

const defaultExeName = "dontstarve_dedicated_server_nullrenderer"

// configureChildProcess gives each DST process its own process group. The group
// ID equals the child PID, so a timeout can reliably terminate the engine and
// any helper processes it spawned without touching the panel itself.
func configureChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// forceStopChild ends the complete child process group. os.Process.Kill only
// reaches the group leader on Linux, which can leave Steam or game children
// behind after a timed-out shutdown.
func forceStopChild(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
