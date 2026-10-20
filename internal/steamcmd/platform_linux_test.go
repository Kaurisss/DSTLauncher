//go:build linux

package steamcmd

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxLayoutUsesNativeExecutableNames(t *testing.T) {
	root := t.TempDir()
	_, executable := serverPaths(root, Arch64)
	if strings.HasSuffix(executable, ".exe") {
		t.Fatalf("Linux 服务端路径不应使用 .exe: %s", executable)
	}
	if filepath.Base(executable) != "dontstarve_dedicated_server_nullrenderer_x64" {
		t.Errorf("64 位服务端名 = %q", filepath.Base(executable))
	}
}

func TestExtractPlatformArchiveRejectsPathTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "steamcmd.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	payload := []byte("must not escape")
	if err := archive.WriteHeader(&tar.Header{Name: "../outside.txt", Mode: 0644, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(t.TempDir(), ArchAuto)
	if err := manager.extractPlatformArchive(archivePath, filepath.Join(t.TempDir(), "steamcmd")); err == nil {
		t.Fatal("Linux tar 解压接受了路径穿越条目")
	}
}
