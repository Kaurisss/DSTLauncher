package backup

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreFailurePreservesOriginalCluster(t *testing.T) {
	m := NewManager(t.TempDir(), "DST", 1)
	if err := os.MkdirAll(m.clusterDir, 0700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(m.clusterDir, "original-save")
	if err := os.WriteFile(original, []byte("keep-original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(m.backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(m.backupDir, "corrupt.zip"))
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, name := range []string{"Master/save/session/WORLD/0000000001", "../outside"} {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte("bad-save"))
	}
	_ = w.Close()
	_ = f.Close()
	if err = m.Restore("corrupt.zip"); err == nil {
		t.Fatal("接受了越界压缩包")
	}
	b, err := os.ReadFile(original)
	if err != nil || string(b) != "keep-original" {
		t.Fatal("失败恢复改变了原存档", err)
	}
}

func TestInspectAllShardsWorldMetadataAndPlayerFiles(t *testing.T) {
	m := NewManager(t.TempDir(), "DST", 1)
	write := func(rel, body string) {
		path := filepath.Join(m.clusterDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("cluster.ini", "[NETWORK]\ncluster_name=Test")
	for _, shard := range []string{"Master", "Caves", "Island"} {
		write(shard+"/save/session/WORLD/0000000002", "saved-world")
		write(shard+"/save/session/WORLD/0000000002.meta", `KLEI     1 return {clock={cycles=9},seasons={season="autumn",remainingdaysinseason=12}}`)
		write(shard+"/save/session/WORLD/PLAYER_FIXTURE/0000000002", "saved-player")
		write(shard+"/modoverrides.lua", "return {}")
		write(shard+"/worldgenoverride.lua", "return {}")
	}
	item, err := m.Create()
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(t.TempDir(), "payload")
	report, err := m.Stage(context.Background(), item.Name, stage, []string{"Master", "Caves", "Island", "Volcano"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !report.CanRestore || len(report.Shards) != 4 || len(report.SHA256) != 64 || report.Files != 16 {
		t.Fatal(report)
	}
	for _, shard := range report.Shards {
		if shard.Name == "Volcano" {
			continue
		}
		if shard.Snapshots != 1 || shard.PlayerDirectories != 1 || shard.State == nil || *shard.State.Day != 10 || shard.State.Season != "autumn" || !shard.ModsConfig || !shard.WorldConfig || shard.State.SavedAt.Before(time.Now().Add(-time.Hour)) {
			t.Fatal(shard)
		}
	}
	if !strings.Contains(strings.Join(report.Warnings, ","), "Volcano") {
		t.Fatal(report.Warnings)
	}
	write("Master/save/session/WORLD/0000000002", "new-live-world")
	previous, err := m.RestoreStaged(context.Background(), item.Name, stage, report.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(m.clusterDir, "Master/save/session/WORLD/0000000002"))
	if string(data) != "saved-world" {
		t.Fatal(string(data))
	}
	data, _ = os.ReadFile(filepath.Join(previous, "Master/save/session/WORLD/0000000002"))
	if string(data) != "new-live-world" {
		t.Fatal("未保留恢复前存档")
	}
}
func TestChangedArchiveCannotRestoreAnEarlierInspection(t *testing.T) {
	m := NewManager(t.TempDir(), "DST", 1)
	for rel, body := range map[string]string{"cluster.ini": "[NETWORK]", "Master/save/session/WORLD/0000000001": "world"} {
		path := filepath.Join(m.clusterDir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0700)
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	item, err := m.Create()
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(t.TempDir(), "payload")
	report, err := m.Stage(context.Background(), item.Name, stage, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(item.Path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("changed"))
	_ = f.Close()
	if _, err = m.RestoreStaged(context.Background(), item.Name, stage, report.SHA256); err == nil {
		t.Fatal("接受了检查后被修改的 ZIP")
	}
	if _, err = os.Stat(m.clusterDir); err != nil {
		t.Fatal("改变了原存档")
	}
}
func TestInspectRejectsDuplicateTraversalSymlinkAndCRCFailures(t *testing.T) {
	for _, kind := range []string{"duplicate", "case", "traversal", "device", "symlink", "crc", "limit"} {
		t.Run(kind, func(t *testing.T) {
			m := NewManager(t.TempDir(), "DST", 1)
			_ = os.MkdirAll(m.backupDir, 0700)
			f, err := os.Create(filepath.Join(m.backupDir, "bad.zip"))
			if err != nil {
				t.Fatal(err)
			}
			w := zip.NewWriter(f)
			if kind == "crc" || kind == "limit" {
				size := uint64(3)
				if kind == "limit" {
					size = uint64(MaxRestoreBytes) + 1
				}
				h := &zip.FileHeader{Name: "cluster.ini", Method: zip.Store, CRC32: 123, CompressedSize64: 3, UncompressedSize64: size}
				entry, err := w.CreateRaw(h)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = entry.Write([]byte("abc"))
			} else {
				name := "cluster.ini"
				entry, _ := w.Create(name)
				_, _ = entry.Write([]byte("[NETWORK]"))
				switch kind {
				case "case":
					name = "CLUSTER.INI"
				case "traversal":
					name = "../outside"
				case "device":
					name = "Master/CON.txt"
				case "symlink":
					name = "Master/link"
				}
				if kind == "symlink" {
					h := &zip.FileHeader{Name: name}
					h.SetMode(os.ModeSymlink | 0777)
					entry, _ = w.CreateHeader(h)
				} else {
					entry, _ = w.Create(name)
				}
				_, _ = entry.Write([]byte("data"))
			}
			_ = w.Close()
			_ = f.Close()
			if _, err = m.Stage(context.Background(), "bad.zip", t.TempDir(), nil, nil); err == nil {
				t.Fatal("接受了损坏或越界备份")
			}
		})
	}
}
