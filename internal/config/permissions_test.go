package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHardenPrivateFilesRepairsExistingTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
	}
	root := t.TempDir()
	backupDir := filepath.Join(root, "backups")
	file := filepath.Join(backupDir, "backup.zip")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("backup"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := HardenPrivateFiles(file, backupDir); err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0600 || dirInfo.Mode().Perm() != 0700 {
		t.Fatalf("权限未收紧: file=%o dir=%o", fileInfo.Mode().Perm(), dirInfo.Mode().Perm())
	}
}
