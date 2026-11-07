package worldsettings

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// Opt-in: verify actual world snapshots read-only, without storing saves in fixtures.
func TestRealWorldTileMaps(t *testing.T) {
	root := os.Getenv("DST_REAL_SAVE")
	if root == "" {
		t.Skip("设置 DST_REAL_SAVE 指向 Cluster_N 以验证实际世界快照")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths, err := filepath.Glob(filepath.Join(root, entry.Name(), "save", "session", "*", "*"))
		if err != nil {
			t.Fatal(err)
		}
		hasWorld := false
		for _, path := range paths {
			if worldSnapshotName.MatchString(filepath.Base(path)) {
				info, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				hasWorld = hasWorld || info.Mode().IsRegular()
			}
		}
		if !hasWorld {
			continue
		}
		m, err := ReadSavedMap(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(entry.Name(), err)
		}
		if m.Terrain == nil || m.Kind != "terrain" {
			t.Fatal(entry.Name(), "未读取地块地图")
		}
		raw, err := base64.StdEncoding.DecodeString(m.Terrain.Tiles)
		if err != nil || len(raw) != 2*m.Width*m.Height {
			t.Fatal("地块载荷不符", err)
		}
		t.Logf("%s: %dx%d, %d named tiles, %d landmarks", entry.Name(), m.Width, m.Height, m.Width*m.Height, len(m.Landmarks))
		checked++
	}
	if checked == 0 {
		t.Fatal("未找到世界快照")
	}
}
