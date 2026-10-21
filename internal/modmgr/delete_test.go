package modmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteModRemovesInstalledDirectories(t *testing.T) {
	for _, tc := range []struct {
		name   string
		info   bool
		copies bool
	}{
		{name: "incomplete_directory"},
		{name: "duplicate_valid_copies", info: true, copies: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cluster := filepath.Join(root, "Cluster_1")
			game := filepath.Join(root, "game")
			ugc := filepath.Join(root, "ugc")
			mgr := NewManager(cluster, game, ugc, []string{"Master", "Caves"})
			id := "workshop-1185229307"
			paths := []string{filepath.Join(ugc, "content", "322330", "1185229307")}
			if tc.copies {
				paths = append(paths,
					filepath.Join(ugc, "steamapps", "workshop", "content", "322330", "1185229307"),
					filepath.Join(game, "mods", id),
					filepath.Join(game, "mods", "workshop_1185229307"),
				)
			}
			for _, path := range paths {
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatal(err)
				}
				if tc.info {
					if err := os.WriteFile(filepath.Join(path, "modinfo.lua"), []byte(`name = "Test"`), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			other := filepath.Join(game, "mods", "other-mod")
			if err := os.MkdirAll(other, 0755); err != nil {
				t.Fatal(err)
			}
			o := NewOverrides()
			o.Order = []string{id, "other-mod"}
			o.Enabled[id], o.Enabled["other-mod"] = true, true
			o.Config[id] = map[string]interface{}{"test": true}
			if err := mgr.SaveOverrides(o); err != nil {
				t.Fatal(err)
			}
			installed, err := mgr.InstalledMods()
			if err != nil || !installed[id] {
				t.Fatalf("fixture must be installed: %v, %v", installed, err)
			}
			if err := mgr.DeleteMod(id); err != nil {
				t.Fatal(err)
			}
			for _, path := range paths {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("deleted mod directory remains: %s (%v)", path, err)
				}
			}
			installed, err = mgr.InstalledMods()
			if err != nil || installed[id] {
				t.Errorf("deleted mod still listed installed: %v, %v", installed, err)
			}
			if !dirExists(other) {
				t.Error("unrelated mod was removed")
			}
			for _, shard := range []string{"Master", "Caves"} {
				data, err := os.ReadFile(filepath.Join(cluster, shard, "modoverrides.lua"))
				if err != nil {
					t.Fatal(err)
				}
				got := ParseOverrides(string(data))
				if _, exists := got.Enabled[id]; exists {
					t.Errorf("deleted mod remains in %s", shard)
				}
				if !got.Enabled["other-mod"] {
					t.Errorf("unrelated override removed in %s", shard)
				}
			}
		})
	}
}

func TestDeleteModLocalAndMissing(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(filepath.Join(root, "cluster"), filepath.Join(root, "game"), filepath.Join(root, "ugc"), []string{"Master"})
	path := filepath.Join(mgr.ModsDir(), "local-test")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteMod("local-test"); err != nil {
		t.Fatal(err)
	}
	if dirExists(path) {
		t.Fatal("local mod directory remains")
	}
	if err := mgr.DeleteMod("workshop-1185229307"); err != nil {
		t.Fatalf("missing mods should still allow override cleanup: %v", err)
	}
}

func TestDeleteModRejectsUnsafeIDs(t *testing.T) {
	for _, id := range []string{"", ".", "..", "...", "local-mod.", "local-mod ", "../outside", `..\outside`, "C:outside", "workshop-", "workshop-.."} {
		t.Run(id, func(t *testing.T) {
			root := t.TempDir()
			mgr := NewManager(filepath.Join(root, "cluster"), filepath.Join(root, "game"), filepath.Join(root, "ugc"), []string{"Master"})
			if err := mgr.DeleteMod(id); err == nil {
				t.Errorf("unsafe ID accepted: %q", id)
			}
		})
	}
}
