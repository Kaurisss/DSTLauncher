//go:build linux

package hoststats

import "testing"

func TestParseMemory(t *testing.T) {
	got, err := parseMemory("MemTotal: 8192 kB\nMemFree: 100 kB\nMemAvailable: 2048 kB\n")
	if err != nil || got.TotalBytes != 8192*1024 || got.AvailableBytes != 2048*1024 {
		t.Fatalf("应读取可回收内存而非仅 MemFree: %+v, %v", got, err)
	}
	for _, data := range []string{
		"", "MemTotal: 8192 kB\n", "MemTotal: 0 kB\nMemAvailable: 0 kB",
		"MemTotal: 1 kB\nMemAvailable: 2 kB", "MemTotal: nope kB",
		"MemTotal: 8192 MB", "MemTotal: 18446744073709551615 kB",
	} {
		if _, err := parseMemory(data); err == nil {
			t.Errorf("应拒绝不完整或无效数据: %q", data)
		}
	}
	if got, err := parseMemory("MemTotal: 8192 kB\nMemAvailable: 0 kB"); err != nil || got.AvailableBytes != 0 {
		t.Fatalf("真正耗尽内存不应算采样失败: %+v, %v", got, err)
	}
}
