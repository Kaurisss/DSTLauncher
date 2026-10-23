package saveimport

import (
	"os"
	"path/filepath"
	"testing"

	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/worldsettings"
)

// 存档里的 leveldataoverride.lua 必须原样落到目标分片。
//
// 它带着 location（shipwrecked / volcanoworld）等完整世界定义，
// 只转换成 worldgenoverride 的话模组世界的维度信息会丢，面板会退回 forest。
func TestWriteIntoKeepsLevelDataOverride(t *testing.T) {
	const islandLevelData = `return {
  id="SURVIVAL_SHIPWRECKED_CLASSIC",
  location="shipwrecked",
  overrides={ bamboo="default" },
}
`
	clusterDir := t.TempDir()
	writeFile(t, filepath.Join(clusterDir, "cluster.ini"), "[MISC]\ncluster_name = x\n")
	writeFile(t, filepath.Join(clusterDir, "Master", "server.ini"), "[SHARD]\nid = 1\nis_master = true\nname = Master\n")
	writeFile(t, filepath.Join(clusterDir, "Master", "save", "shardindex"), "x")
	writeFile(t, filepath.Join(clusterDir, "Master", "leveldataoverride.lua"), islandLevelData)

	p, err := Parse(clusterDir)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}

	root := t.TempDir()
	rm := &room.Room{ID: 1, Shards: []room.Shard{
		{Name: "Master", WorldType: room.WorldTypeForest, GameID: 1, IsMaster: true},
	}}
	if _, err := p.WriteInto(root, "DST", rm); err != nil {
		t.Fatalf("WriteInto 失败: %v", err)
	}

	dst := worldsettings.LevelDataPath(config.ClusterDir(root, "DST", rm.ID), "Master")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("leveldataoverride.lua 没有被导入: %v", err)
	}
	if string(got) != islandLevelData {
		t.Errorf("内容被改动了:\n%s", got)
	}
	// 同一分片不能再留 worldgenoverride：游戏会把它 merge 在 leveldata 上面
	if _, err := os.Stat(worldsettings.FilePath(config.ClusterDir(root, "DST", rm.ID), "Master")); err == nil {
		t.Error("有 leveldataoverride 时不应再写 worldgenoverride")
	}
}

// 存档没带 leveldataoverride 时沿用 worldgenoverride 转换，旧房间遗留的
// leveldataoverride 必须清掉，否则会盖住新导入的世界数据。
func TestWriteIntoDropsStaleLevelData(t *testing.T) {
	clusterDir := t.TempDir()
	writeFile(t, filepath.Join(clusterDir, "cluster.ini"), "[MISC]\ncluster_name = x\n")
	writeFile(t, filepath.Join(clusterDir, "Master", "server.ini"), "[SHARD]\nid = 1\nis_master = true\nname = Master\n")
	writeFile(t, filepath.Join(clusterDir, "Master", "save", "shardindex"), "x")
	writeFile(t, filepath.Join(clusterDir, "Master", "worldgenoverride.lua"),
		"return {\n\toverride_enabled = true,\n\tworldgen_preset = \"SURVIVAL_TOGETHER\",\n\toverrides = { day = \"long\" },\n}\n")

	p, err := Parse(clusterDir)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}

	root := t.TempDir()
	rm := &room.Room{ID: 2, Shards: []room.Shard{
		{Name: "Master", WorldType: room.WorldTypeForest, GameID: 1, IsMaster: true},
	}}
	target := config.ClusterDir(root, "DST", rm.ID)
	writeFile(t, worldsettings.LevelDataPath(target, "Master"), "return { location=\"cave\", overrides={} }\n")

	if _, err := p.WriteInto(root, "DST", rm); err != nil {
		t.Fatalf("WriteInto 失败: %v", err)
	}
	if _, err := os.Stat(worldsettings.LevelDataPath(target, "Master")); !os.IsNotExist(err) {
		t.Error("存档没有 leveldataoverride 时应清掉目标目录里的旧文件")
	}
	ov, exists, err := worldsettings.ReadFile(target, "Master", room.WorldTypeForest)
	if err != nil || !exists {
		t.Fatalf("worldgenoverride 没有导入: exists=%v err=%v", exists, err)
	}
	if ov.Overrides["day"] != "long" {
		t.Errorf("世界设置丢失: %+v", ov.Overrides)
	}
}
