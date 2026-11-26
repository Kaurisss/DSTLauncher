package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogReturnsRequestedLineCount(t *testing.T) {
	logDir := t.TempDir()
	manager := NewManager(t.TempDir(), t.TempDir(), logDir)
	path := manager.logPathFor("Cluster_1", "Master")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\nfive\n"), 0600); err != nil {
		t.Fatal(err)
	}

	lines, err := manager.ReadLog("Cluster_1", "Master", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(lines, ","); got != "four,five" {
		t.Fatalf("ReadLog() = %q, want %q", got, "four,five")
	}
}

func TestCompactLogFileKeepsRecentLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("old-1\nold-2\nnew-1\nnew-2\n"); err != nil {
		t.Fatal(err)
	}
	if file, err = compactLogFile(file, 20); err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "new-2") || strings.Contains(string(data), "old-1") {
		t.Fatalf("压缩后的日志内容不正确: %q", data)
	}
}
