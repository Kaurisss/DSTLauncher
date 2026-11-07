package main

import (
	"net"
	"testing"
)

func TestPanelURL(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want string
	}{
		{"127.0.0.1", "http://127.0.0.1:16800"},
		{"0.0.0.0", "http://127.0.0.1:16800"},
		{"::", "http://[::1]:16800"},
		{"::1", "http://[::1]:16800"},
		{"192.168.1.2", "http://192.168.1.2:16800"},
	} {
		t.Run(tc.ip, func(t *testing.T) {
			addr := &net.TCPAddr{IP: net.ParseIP(tc.ip), Port: 16800}
			if got := panelURL(addr); got != tc.want {
				t.Fatalf("panelURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
