package backup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutomaticRetentionPreservesManualBackupsAndAllShards(t *testing.T) {
	m := NewManager(t.TempDir(), "DST", 1)
	for _, shard := range []string{"Surface", "Below"} {
		path := filepath.Join(m.clusterDir, shard, "save", "world")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(shard), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manual, err := m.Create()
	if err != nil {
		t.Fatal(err)
	}
	var latest *BackupItem
	for i := 0; i < 4; i++ {
		latest, err = m.CreateAutomatic(2)
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := m.List()
	if err != nil || len(items) != 3 {
		t.Fatal(items, err)
	}
	if _, err := os.Stat(manual.Path); err != nil {
		t.Fatal("手动备份被误删", err)
	}
	if err := m.Clean(); err != nil {
		t.Fatal(err)
	}
	auto := 0
	for _, i := range items {
		if strings.Contains(i.Name, "_auto_") {
			auto++
		}
	}
	if auto != 2 {
		t.Fatal(items)
	}
	z, err := zip.OpenReader(latest.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 2 {
		t.Fatal("未包含所有分片", z.File)
	}
	if _, err := m.CreateAutomatic(0); err == nil {
		t.Fatal("零份数不能删除所有备份")
	}
}

func TestFailedAutomaticBackupDoesNotPrune(t *testing.T) {
	m := NewManager(t.TempDir(), "DST", 2)
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(m.backupDir, "Cluster_2_auto_old.zip")
	if err := os.WriteFile(old, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateAutomatic(1); err == nil {
		t.Fatal("无存档不应创建备份")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("失败不能清理旧备份")
	}
}
