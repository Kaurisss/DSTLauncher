package worldsettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocationFromFilesPrefersLevelDataThenWorldgenPreset(t *testing.T) {
	cluster := t.TempDir()
	shard := "Island"
	dir := filepath.Join(cluster, shard)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, OverrideFileName), []byte(
		"return { worldgen_preset=\"SHIPWRECKED_ENDLESS\", overrides={} }",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, found, err := LocationFromFiles(cluster, shard, LocationForest); err != nil {
		t.Fatal(err)
	} else if got != LocationShipwrecked || !found {
		t.Fatalf("worldgen 预设识别错误: location=%q found=%v", got, found)
	}

	if err := os.WriteFile(filepath.Join(dir, LevelDataFileName), []byte(
		"return { location=\"volcanoworld\", overrides={} }",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, found, err := LocationFromFiles(cluster, shard, LocationForest); err != nil {
		t.Fatal(err)
	} else if got != LocationVolcano || !found {
		t.Fatalf("leveldata 位置优先级错误: location=%q found=%v", got, found)
	}
}
