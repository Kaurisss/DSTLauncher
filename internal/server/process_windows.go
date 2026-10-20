//go:build windows

package server

import (
	"os/exec"
	"syscall"
)

const defaultExeName = "dontstarve_dedicated_server_nullrenderer.exe"

// configureChildProcess keeps DST detached from the panel's console.
func configureChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// forceStopChild preserves the existing Windows fallback: terminate the server
// process after its graceful shutdown deadline has elapsed.
func forceStopChild(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
