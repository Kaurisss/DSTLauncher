package migration

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dst-windows/internal/automation"
	"dst-windows/internal/room"
)

func fixture(t *testing.T) (string, *Manifest) {
	t.Helper()
	root := t.TempDir()
	rm := room.Defaults()
	rm.ID = 7
	rm.Token = "server-token"
	rm.Password = "room-password"
	if err := room.Generate(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	save := filepath.Join(rm.ClusterDir(root, "DST"), "Caves", "save", "session", "world", "0000000010")
	if err := os.MkdirAll(filepath.Dir(save), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(save, []byte("world-save-data"), 0600); err != nil {
		t.Fatal(err)
	}
	modified := time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC)
	if err := os.Chtimes(save, modified, modified); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "migration.zip")
	m, err := Create(context.Background(), archive, Options{Root: root, ConfDir: "DST", Rooms: []*room.Room{rm}, Plan: automation.Config{Keep: 5, UpdateMode: "notify"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return archive, m
}
func TestArchiveRoundTripPreservesSaveTimeAndCredentials(t *testing.T) {
	archive, expected := fixture(t)
	dest := t.TempDir()
	m, err := Extract(context.Background(), archive, dest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Rooms[0].Token != "server-token" || m.Rooms[0].Password != "room-password" || len(m.Files) != len(expected.Files) {
		t.Fatal(m)
	}
	path := filepath.Join(dest, "rooms", "7", "Caves", "save", "session", "world", "0000000010")
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "world-save-data" {
		t.Fatal(string(b), err)
	}
	info, _ := os.Stat(path)
	if info.ModTime().Year() != 2026 || info.ModTime().Day() != 2 {
		t.Fatal(info.ModTime())
	}
}
func rewriteZIP(t *testing.T, archive string, mutate func(string, []byte) (string, []byte, bool), extra string) string {
	t.Helper()
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	dest := filepath.Join(t.TempDir(), "modified.zip")
	out, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(out)
	for _, file := range reader.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
		name, b, keep := mutate(file.Name, b)
		if !keep {
			continue
		}
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if extra != "" {
		w, err := writer.Create(extra)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("extra"))
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	return dest
}
func TestRejectCorruptAndUnsafeArchives(t *testing.T) {
	archive, _ := fixture(t)
	for _, kind := range []string{"checksum", "missing", "traversal", "case", "duplicate", "extra", "limit"} {
		t.Run(kind, func(t *testing.T) {
			extra := ""
			if kind == "extra" {
				extra = "unexpected.txt"
			}
			if kind == "case" {
				extra = "ROOMS/7/cluster.ini"
			}
			if kind == "duplicate" {
				extra = "rooms/7/cluster.ini"
			}
			modified := rewriteZIP(t, archive, func(name string, b []byte) (string, []byte, bool) {
				if kind == "checksum" && strings.HasSuffix(name, "0000000010") {
					return name, []byte("WORLD-save-data"), true
				}
				if kind == "missing" && strings.HasSuffix(name, "cluster.ini") {
					return name, b, false
				}
				if kind == "traversal" && strings.HasSuffix(name, "cluster.ini") {
					return "../outside", b, true
				}
				if kind == "limit" && name == "manifest.json" {
					var m Manifest
					_ = json.Unmarshal(b, &m)
					m.Files[0].Size = MaxUnpacked + 1
					b, _ = json.Marshal(m)
				}
				return name, b, true
			}, extra)
			if _, err := Extract(context.Background(), modified, t.TempDir(), nil); err == nil {
				t.Fatal("接受了非法 ZIP")
			}
		})
	}
}

func TestRejectZIPSymbolicLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symlink.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	h := &zip.FileHeader{Name: "rooms/7/link"}
	h.SetMode(os.ModeSymlink | 0777)
	entry, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("../../outside"))
	_ = w.Close()
	_ = f.Close()
	if _, err := Extract(context.Background(), path, t.TempDir(), nil); err == nil {
		t.Fatal("接受 ZIP 符号链接")
	}
}
func TestUnsafePathsAndCancellation(t *testing.T) {
	for _, p := range []string{"../a", "/a", "C:/a", "a\\b", "a/CON.txt", "a/LPT1", "a/name.", "a/name ", "a/b:stream", "a/../b", "a/\x00", "a/a?b"} {
		if safePath(p) == nil {
			t.Fatal(p)
		}
	}
	archive, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Extract(ctx, archive, t.TempDir(), nil); err == nil {
		t.Fatal("忽略取消")
	}
	root := t.TempDir()
	rm := room.Defaults()
	rm.ID = 1
	if err := room.Generate(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rm.ClusterDir(root, "DST"), "outside")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skip("符号链接权限不可用")
	}
	if _, err := Create(context.Background(), filepath.Join(t.TempDir(), "a.zip"), Options{Root: root, ConfDir: "DST", Rooms: []*room.Room{rm}, Plan: automation.Config{Keep: 5, UpdateMode: "notify"}}, nil); err == nil {
		t.Fatal("导出了符号链接")
	}
}
