package steamcmd

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
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
	if err := manager.extractZip(context.Background(), archivePath, filepath.Join(t.TempDir(), "steamcmd")); err == nil {
		t.Fatal("extractZip accepted a path traversal entry")
	}
}

func TestExtractProgressOnlyAcceptsSteamCmdFormat(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"Update state (0x11) preallocating, progress: 0.00 (0 / 4490782099)", "预分配磁盘空间 0.00%"},
		{"Update state (0x61) downloading, progress: 41.63 (1869568807 / 4490782099)", "下载服务器文件 41.63%"},
		{"Update state (0x61) downloading, progress: 100.00 (4490782099 / 4490782099)", "下载服务器文件 100.00%"},
		// 噪音行必须忽略：曾因泛化 NN% 匹配把面板进度从 100% 打回 0%
		{"Preallocating 100%", ""},
		{"Memory Load: 39%", ""},
		// 校验进度明确标注阶段；未知阶段不猜测百分比。
		{"Update state (0x81) verifying install, progress: 4.05 (181928000 / 4490782099)", "校验服务器文件 4.05%"},
		{"Update state (0x61) DOWNLOADING, progress: 7 (314354746 / 4490782099)", "下载服务器文件 7%"},
		{"downloading progress: 50 (1 / 2)", ""},
		{"Update state (0x61) downloading, progress: 1..2 (1 / 2)", ""},
		{"Update state (0x0) unknown, progress: 0.00 (0 / 0)", ""},
		{"Unloading Steam API...", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := extractProgress(c.line); got != c.want {
			t.Errorf("extractProgress(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestInstallProgressReportsEachStage(t *testing.T) {
	manager := NewManager(t.TempDir(), ArchAuto)
	for _, step := range []struct {
		line string
		want string
	}{
		{"\x1b[0m Update state (0x11) preallocating, progress: 35.27 (1584028185 / 4490782099)", "预分配磁盘空间 35.27%"},
		{"Update state (0x61) downloading, progress: 2.50 (112269552 / 4490782099)", "下载服务器文件 2.50%"},
		{"Update state (0x81) verifying install, progress: 4.05 (181928000 / 4490782099)", "校验服务器文件 4.05%"},
		{"Memory Load: 39%", "校验服务器文件 4.05%"},
	} {
		manager.appendOutput(step.line)
		got, _, _ := manager.Progress()
		if got != step.want {
			t.Errorf("appendOutput(%q): progress = %q, want %q", step.line, got, step.want)
		}
	}
}

func TestCancelInstallRejectsIdle(t *testing.T) {
	manager := NewManager(t.TempDir(), ArchAuto)
	if err := manager.Cancel(); err == nil {
		t.Fatal("Cancel() succeeded when no installation was running")
	}
}

func TestCancelInstallAbortsSteamCmdDownload(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	manager := NewManager(t.TempDir(), ArchAuto)
	manager.archiveURL = srv.URL
	manager.Install()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SteamCMD download request")
	}

	if err := manager.Cancel(); err != nil {
		t.Fatalf("Cancel() failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for manager.IsInstalling() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if manager.IsInstalling() {
		t.Fatal("installation remained running after Cancel()")
	}
	if !manager.IsCanceled() {
		t.Fatal("IsCanceled() = false after Cancel()")
	}
	progress, errText, _ := manager.Progress()
	if progress != "已取消安装" {
		t.Errorf("progress = %q, want 已取消安装", progress)
	}
	if !strings.Contains(errText, "已取消安装") {
		t.Errorf("errText = %q, want cancellation message", errText)
	}
}

func TestCancelInstallTerminatesRunningSteamCmdProcess(t *testing.T) {
	workDir := t.TempDir()
	manager := NewManager(workDir, ArchAuto)
	mustWriteFile(t, filepath.Join(manager.steamCmdDir, steamCmdExecutableName()))

	started := make(chan struct{}, 1)
	manager.makeCmd = func(_ string, _ ...string) *exec.Cmd {
		select {
		case started <- struct{}{}:
		default:
		}
		if runtime.GOOS == "windows" {
			return exec.Command("ping", "-n", "30", "127.0.0.1")
		}
		return exec.Command("sleep", "30")
	}

	manager.Install()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SteamCMD subprocess to start")
	}
	// Wait until Manager records the running cmd.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		hasCmd := manager.cmd != nil
		manager.mu.RUnlock()
		if hasCmd {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := manager.Cancel(); err != nil {
		t.Fatalf("Cancel() failed: %v", err)
	}

	waitDeadline := time.Now().Add(5 * time.Second)
	for manager.IsInstalling() && time.Now().Before(waitDeadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if manager.IsInstalling() {
		t.Fatal("SteamCMD subprocess did not exit after Cancel()")
	}
	if !manager.IsCanceled() {
		t.Fatal("IsCanceled() = false after terminating SteamCMD subprocess")
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
