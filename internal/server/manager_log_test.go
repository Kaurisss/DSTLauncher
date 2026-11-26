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
