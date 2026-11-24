package modmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSteamWorkshopReadAndMissingMods(t *testing.T) {
	root := t.TempDir()
	workshop := filepath.Join(root, "Steam 库", "steamapps", "workshop")
	game := filepath.Join(root, "game")
	cluster := filepath.Join(root, "Cluster_1")
	for _, id := range []string{"123", "456", ".dst-panel-backup-test"} {
		dir := filepath.Join(workshop, "content", "322330", id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if id != "456" {
			if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte("name = '中文工坊模组'\nversion = '1.2.3-beta'\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	manager := NewManager(cluster, game, workshop, []string{"Master"})
	overrides := NewOverrides()
	overrides.Order = []string{"workshop-123", "workshop-456"}
	overrides.Enabled["workshop-123"] = true
	overrides.Enabled["workshop-456"] = true
	if err := manager.SaveOverrides(overrides); err != nil {
		t.Fatal(err)
	}
	installed, err := manager.InstalledMods()
	if err != nil || len(installed) != 1 || !installed["workshop-123"] {
		t.Fatalf("只认完整的 Steam 工坊模组，不认临时或半下载目录: %v, %v", installed, err)
	}
	name, version, _ := manager.ModInfo("workshop-123")
	if name != "中文工坊模组" {
		t.Fatalf("应直接从 Steam 工坊读取元信息: %q", name)
	}
	if version != "1.2.3-beta" {
		t.Fatalf("应读取本地声明的版本: %q", version)
	}
	missing, err := manager.MissingMods()
	if err != nil || len(missing) != 1 || missing[0] != "workshop-456" {
		t.Fatalf("残缺下载应仍报缺失: %v, %v", missing, err)
	}
}

func TestInstalledModDirsUsesCandidatePriorityAndRequiresModInfo(t *testing.T) {
	root := t.TempDir()
	modsDir := filepath.Join(root, "game", "mods")
	manager := NewManager(filepath.Join(root, "cluster"), filepath.Join(root, "game"), "", []string{"Master"})

	for _, name := range []string{"workshop-123", "workshop_123", "workshop-456"} {
		if err := os.MkdirAll(filepath.Join(modsDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(modsDir, "workshop-123", "modinfo.lua"), []byte("name='hyphen'"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modsDir, "workshop_123", "modinfo.lua"), []byte("name='underscore'"), 0644); err != nil {
		t.Fatal(err)
	}

	installed, err := manager.InstalledModDirs()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := installed["workshop-123"], filepath.Join(modsDir, "workshop-123"); got != want {
		t.Fatalf("短横线目录应优先于下划线目录: got %q want %q", got, want)
	}
	if _, ok := installed["workshop-456"]; ok {
		t.Fatal("缺少 modinfo.lua 的目录不应被视为已安装")
	}
}
