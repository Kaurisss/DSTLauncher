//go:build linux

package server

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestConfigureChildProcessCreatesSeparateProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $! > \"$1\"; wait", "sh", pidFile)
	configureChildProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试进程失败: %v", err)
	}
	defer func() {
		_ = forceStopChild(cmd)
		_ = cmd.Wait()
	}()

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("获取进程组失败: %v", err)
	}
	if pgid != cmd.Process.Pid {
		t.Fatalf("进程组 ID = %d，want %d", pgid, cmd.Process.Pid)
	}

	childPID := waitForPID(t, pidFile)
	if err := forceStopChild(cmd); err != nil {
		t.Fatalf("终止进程组失败: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Error("被 SIGKILL 终止的进程应返回非 nil 错误")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("子进程 %d 在终止其进程组后仍存活", childPID)
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(string(data[:len(data)-1]))
			if parseErr != nil {
				t.Fatalf("解析子进程 PID 失败: %v", parseErr)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待子进程 PID 超时")
	return 0
}
