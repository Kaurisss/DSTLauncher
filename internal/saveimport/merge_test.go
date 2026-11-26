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

func TestWriteIntoImportsUngeneratedShardDefinitions(t *testing.T) {
	clusterDir := t.TempDir()
	writeFile(t, filepath.Join(clusterDir, "cluster.ini"), "[MISC]\ncluster_name = x\n")
	definitions := map[string]string{
		"Master":  `return { location="forest", overrides={} }`,
		"Caves":   `return { location="cave", overrides={} }`,
		"Island":  `return { id="SURVIVAL_SHIPWRECKED_CLASSIC", location="shipwrecked", overrides={bamboo="default"} }`,
		"Volcano": `return { id="SURVIVAL_VOLCANO_CLASSIC", location="volcanoworld", overrides={boons="never"} }`,
	}
	for name, definition := range definitions {
		writeFile(t, filepath.Join(clusterDir, name, "server.ini"), "[SHARD]\nname = "+name+"\n")
		writeFile(t, filepath.Join(clusterDir, name, levelDataOverrideName), definition)
		if name == "Master" || name == "Caves" {
			writeFile(t, filepath.Join(clusterDir, name, "save", "shardindex"), name)
		}
	}
	p, err := Parse(clusterDir)
	if err != nil {
		t.Fatal(err)
	}
	rm := MergeInto(p, nil)
	rm.ID = 3
	rm.Normalize()
	root := t.TempDir()
	target := config.ClusterDir(root, "DST", rm.ID)
	// 覆盖到尚未生成的房间时也要清掉普通地表预设。
	if err := room.Generate(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	if _, err := p.WriteInto(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	if err := room.Generate(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	for name, want := range definitions {
		got, err := os.ReadFile(worldsettings.LevelDataPath(target, name))
		if err != nil || string(got) != want {
			t.Errorf("%s 世界定义未原样保留: %q, err=%v", name, got, err)
		}
		if _, err := os.Stat(worldsettings.FilePath(target, name)); !os.IsNotExist(err) {
			t.Errorf("%s 不应残留 worldgenoverride: %v", name, err)
		}
	}
}

func TestWriteIntoPreservesExistingSaveWithoutSourceSave(t *testing.T) {
	clusterDir := t.TempDir()
	writeFile(t, filepath.Join(clusterDir, "cluster.ini"), "[MISC]\ncluster_name = x\n")
	writeFile(t, filepath.Join(clusterDir, "Master", "server.ini"), "[SHARD]\nis_master = true\n")
	writeFile(t, filepath.Join(clusterDir, "Master", levelDataOverrideName), `return { location="shipwrecked", overrides={} }`)
	p, err := Parse(clusterDir)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rm := MergeInto(p, nil)
	rm.ID = 1
	target := config.ShardDir(root, "DST", rm.ID, "Master")
	want := map[string]string{
		filepath.Join("save", "shardindex"): "existing world",
		levelDataOverrideName:               `return { location="forest", overrides={} }`,
		worldsettings.OverrideFileName:      `return { override_enabled=true, overrides={day="long"} }`,
	}
	for name, content := range want {
		writeFile(t, filepath.Join(target, name), content)
	}
	if _, err := p.WriteInto(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(target, name))
		if err != nil || string(got) != content {
			t.Errorf("已有存档的 %s 被修改: %q, err=%v", name, got, err)
		}
	}
}

func TestMergeIntoKeepsSourceWorldTypeWhenOverwritingRoom(t *testing.T) {
	source := &Parsed{
		Room: &room.Room{
			Shards: []room.Shard{{
				Name:      "Island",
				WorldType: room.WorldTypeShipwrecked,
			}},
		},
	}
	target := &room.Room{
		ID: 7,
		Shards: []room.Shard{{
			Name:               "Island",
			WorldType:          room.WorldTypeForest,
			GameID:             3,
			ServerPort:         11003,
			MasterServerPort:   31003,
			AuthenticationPort: 41003,
		}},
	}

	merged := MergeInto(source, target)
	if len(merged.Shards) != 1 || merged.Shards[0].WorldType != room.WorldTypeShipwrecked {
		t.Fatalf("覆盖导入不应丢掉源世界类型: %+v", merged.Shards)
	}
	if merged.Shards[0].GameID != target.Shards[0].GameID ||
		merged.Shards[0].ServerPort != target.Shards[0].ServerPort {
		t.Fatalf("覆盖导入不应改动目标分片机器字段: %+v", merged.Shards[0])
	}
}
