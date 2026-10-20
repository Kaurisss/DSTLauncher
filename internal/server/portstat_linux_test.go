//go:build linux

package server

import (
	"net"
	"os"
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
	if portOwnedBy(port, self+1) {
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
