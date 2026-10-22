//go:build linux

package server

import (
	"net"
	"os"
	"os/exec"
	"testing"
)

func TestPortOwnedBySelf(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("监听 UDP 失败: %v", err)
	}
	defer conn.Close() //nolint:errcheck

	port := conn.LocalAddr().(*net.UDPAddr).Port
	self := os.Getpid()
	if !portOwnedBy(port, self) {
		t.Fatalf("端口 %d 已绑定，却没有识别为当前进程所有", port)
	}
	// 相邻 PID 可能是当前进程的线程 ID，同样持有该 socket；用独立进程校验。
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = other.Process.Kill()
		_ = other.Wait()
	})
	if portOwnedBy(port, other.Process.Pid) {
		t.Error("不应把当前进程的 UDP 端口归属给其它 PID")
	}
}

func TestPortOwnedByRejectsInvalidArgs(t *testing.T) {
	if portOwnedBy(0, os.Getpid()) {
		t.Error("端口 0 不应判定为已监听")
	}
	if portOwnedBy(65536, os.Getpid()) {
		t.Error("超出范围的端口不应判定为已监听")
	}
	if portOwnedBy(11000, 0) {
		t.Error("PID 0 不应判定为已监听")
	}
}
