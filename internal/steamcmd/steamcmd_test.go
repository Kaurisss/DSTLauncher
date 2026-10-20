package steamcmd

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeArch(t *testing.T) {
	cases := map[string]string{
		"":        ArchAuto,
		"auto":    ArchAuto,
		"AUTO":    ArchAuto,
		"64":      Arch64,
		" 32 ":    Arch32,
		"invalid": ArchAuto,
	}
	for input, want := range cases {
		if got := NormalizeArch(input); got != want {
			t.Errorf("NormalizeArch(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPickLayoutPrefers64BitAndAcceptsBinPaths(t *testing.T) {
	root := t.TempDir()
	_, exe32 := serverPaths(root, Arch32)
	_, exe64 := serverPaths(root, Arch64)
	mustWriteFile(t, exe32)
	mustWriteFile(t, exe64)

	layout, err := PickLayout(root, ArchAuto, "test")
	if err != nil {
		t.Fatalf("PickLayout(root) failed: %v", err)
	}
	if layout.Arch != Arch64 {
		t.Errorf("auto architecture = %s, want %s", layout.Arch, Arch64)
	}
	if layout.ServerDir != filepath.Join(root, "bin64") {
		t.Errorf("server directory = %q", layout.ServerDir)
	}

	layout, err = PickLayout(filepath.Join(root, "bin"), Arch32, "test")
	if err != nil {
		t.Fatalf("PickLayout(bin) failed: %v", err)
	}
	if layout.Root != root || layout.Arch != Arch32 {
		t.Errorf("bin path resolved to %+v, want root %q and %s", layout, root, Arch32)
	}
}

func TestPickLayoutRejectsMissingRequestedArchitecture(t *testing.T) {
	root := t.TempDir()
	_, exe32 := serverPaths(root, Arch32)
	mustWriteFile(t, exe32)

	if _, err := PickLayout(root, Arch64, "test"); err == nil {
		t.Fatal("PickLayout accepted missing 64-bit executable")
	}
	layout, err := PickLayout(root, ArchAuto, "test")
	if err != nil {
		t.Fatalf("PickLayout(auto) failed: %v", err)
	}
	if layout.Arch != Arch32 {
		t.Errorf("auto architecture = %s, want %s", layout.Arch, Arch32)
	}
}

func TestVDFLibraryPaths(t *testing.T) {
	vdf := `"libraryfolders"
{
    "0"
    {
        "path" "D:\\SteamLibrary"
    }
    "1" "E:\\Games\\Steam"
}`
	want := []string{`D:\SteamLibrary`, `E:\Games\Steam`}
	if got := vdfLibraryPaths(vdf); !reflect.DeepEqual(got, want) {
		t.Errorf("vdfLibraryPaths() = %#v, want %#v", got, want)
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "steamcmd.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(file)
	entry, err := zipWriter.Create("../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("must not escape")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(t.TempDir(), ArchAuto)
	if err := manager.extractZip(archivePath, filepath.Join(t.TempDir(), "steamcmd")); err == nil {
		t.Fatal("extractZip accepted a path traversal entry")
	}
}

func TestExtractPercentOnlyAcceptsSteamCmdFormat(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		// SteamCMD 的权威下载进度行
		{"Update state (0x61) downloading, progress: 41.63 (1869568807 / 4490782099)", "41.63%"},
		{"Update state (0x61) downloading, progress: 100.00 (4490782099 / 4490782099)", "100.00%"},
		// 噪音行必须忽略：曾因泛化 NN% 匹配把面板进度从 100% 打回 0%
		{"Preallocating 100%", ""},
		{"Memory Load: 39%", ""},
		// 下载完成后的其他阶段也打印 progress，必须忽略，
		// 否则面板会从 99% 回跳到 4%/38%/0%（实测过）
		{"Update state (0x81) verifying install, progress: 4.05 (181928000 / 4490782099)", ""},
		{"Update state (0x0) unknown, progress: 0.00 (0 / 0)", ""},
		{"Unloading Steam API...", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := extractPercent(c.line); got != c.want {
			t.Errorf("extractPercent(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
}
