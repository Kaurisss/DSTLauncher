package steamcmd

import (
	"path/filepath"
	"testing"
)

func TestWorkshopDirFollowsSelectedSteamLibrary(t *testing.T) {
	work := t.TempDir()
	manager := NewManager(work, ArchAuto)
	for _, library := range []string{"Steam", "第二个 Steam 库"} {
		root := filepath.Join(t.TempDir(), library, "steamapps", "common", "Don't Starve Together")
		_, executable := serverPaths(root, Arch64)
		mustWriteFile(t, executable)
		if _, err := manager.Configure(filepath.Dir(executable), Arch64, "test"); err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(filepath.Dir(filepath.Dir(root)), "workshop")
		if got := manager.WorkshopDir(); got != want {
			t.Fatalf("工坊应随安装库切换: got %s, want %s", got, want)
		}
	}
}

func TestWorkshopDirStandaloneFallback(t *testing.T) {
	work := t.TempDir()
	manager := NewManager(work, ArchAuto)
	_, executable := serverPaths(ownRoot(work), Arch64)
	mustWriteFile(t, executable)
	if _, err := manager.Configure(ownRoot(work), Arch64, "test"); err != nil {
		t.Fatal(err)
	}
	if got := manager.WorkshopDir(); got != filepath.Join(work, "ugc_mods") {
		t.Fatalf("面板自带安装应保留独立缓存: %s", got)
	}
	root := t.TempDir()
	_, executable = serverPaths(root, Arch64)
	mustWriteFile(t, executable)
	if _, err := manager.Configure(root, Arch64, "test"); err != nil {
		t.Fatal(err)
	}
	if got := manager.WorkshopDir(); got != filepath.Join(work, "ugc_mods") {
		t.Fatalf("无 Steam 库布局的独立安装应保留缓存: %s", got)
	}
}
