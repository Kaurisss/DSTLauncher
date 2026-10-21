package web

import (
	"dst-windows/internal/worldsettings"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOriginalLevelDataRejectsGenerationUpdates(t *testing.T) {
	for _, loc := range []string{"forest", "cave"} {
		t.Run(loc, func(t *testing.T) {
			before := "return { location='" + loc + "', overrides={world_size='default', weather='default'} }"
			api, rm := newWorldConfigServer(t, []string{"Master"}, map[string]string{"Master": before})
			code, _ := putWorldConfig(t, api, "/api/worldconfig", map[string]any{"room": rm.ID, "shard": "Master", "overrides": map[string]string{"world_size": "huge"}})
			if code == 200 {
				t.Error("已有原版存档不得写入生成项")
			}
			b, err := os.ReadFile(worldsettings.LevelDataPath(rm.ClusterDir(api.rootDir, api.confDir), "Master"))
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != before {
				t.Error("拒绝时应保持原文件不变")
			}
		})
	}
}

func TestOrdinarySaveKeepsEveryUnknownLiteral(t *testing.T) {
	const before = `return { override_enabled=true, overrides={weather="never", unknown_single='custom', unknown_bool=true, unknown_num=42, unknown_second="second", unknown_list={"a", "b"}, ["dotted.key"]="text with spaces, quotes \" and }"} }`
	api, rm := newWorldConfigServer(t, []string{"Master"}, nil)
	path := worldsettings.FilePath(rm.ClusterDir(api.rootDir, api.confDir), "Master")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(before), 0644); err != nil {
		t.Fatal(err)
	}
	view := getWorldConfig(t, api, itoa(rm.ID), "Master")
	for k, v := range map[string]string{"weather": "never", "unknown_single": "custom", "unknown_bool": "true", "unknown_num": "42", "unknown_second": "second"} {
		if view.Config.Overrides[k] != v {
			t.Errorf("读取丢失 %s: %q", k, view.Config.Overrides[k])
		}
	}
	view.Config.Overrides["weather"] = "often"
	code, msg := putWorldConfig(t, api, "/api/worldconfig?room="+itoa(rm.ID)+"&shard=Master", view.Config)
	if code != 200 {
		t.Fatalf("保留未知值时保存失败: %s", msg)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(before, `weather="never"`, `weather="often"`, 1)
	if string(b) != want {
		t.Errorf("未知键的内容或类型被改动:\ngot %s\nwant %s", b, want)
	}
	// 缺省提交也不能删除未知项。
	code, msg = putWorldConfig(t, api, "/api/worldconfig?room="+itoa(rm.ID)+"&shard=Master", map[string]any{"overrideEnabled": true, "overrides": map[string]string{"weather": "default"}})
	if code != 200 {
		t.Fatal(msg)
	}
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want = strings.Replace(before, `weather="never"`, `weather="default"`, 1)
	if string(b) != want {
		t.Errorf("部分提交删除了未点名值: %s", b)
	}
}

func TestOrdinaryGeneratedConfigRejectsGenerationChanges(t *testing.T) {
	api, rm := newWorldConfigServer(t, []string{"Master"}, nil)
	cluster := rm.ClusterDir(api.rootDir, api.confDir)
	original := &worldsettings.Override{Enabled: true, WorldGenPreset: "SURVIVAL_TOGETHER", Overrides: map[string]string{"world_size": "huge", "weather": "never"}}
	if err := worldsettings.WriteFile(cluster, "Master", original); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cluster, "Master", "save")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shardindex"), []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	path := worldsettings.FilePath(cluster, "Master")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"value", "preset"} {
		t.Run(kind, func(t *testing.T) {
			v := getWorldConfig(t, api, itoa(rm.ID), "Master")
			if !v.HasSave {
				t.Fatal("测试夹具应为已有存档")
			}
			if kind == "value" {
				v.Config.Overrides["world_size"] = "small"
			} else {
				v.Config.WorldGenPreset = "DST_CAVE"
			}
			code, _ := putWorldConfig(t, api, "/api/worldconfig?room="+itoa(rm.ID)+"&shard=Master", v.Config)
			if code == 200 {
				t.Error("已有存档生成参数/预设不可修改")
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != string(before) {
				t.Error("拒绝时仍然写入文件")
			}
		})
	}
	// 原生成值随规则一起提交，必须允许且原样保留。
	v := getWorldConfig(t, api, itoa(rm.ID), "Master")
	v.Config.Overrides["weather"] = "often"
	code, msg := putWorldConfig(t, api, "/api/worldconfig?room="+itoa(rm.ID)+"&shard=Master", v.Config)
	if code != 200 {
		t.Fatal(msg)
	}
	again := getWorldConfig(t, api, itoa(rm.ID), "Master")
	if again.Config.Overrides["world_size"] != "huge" {
		t.Fatal("原生成值没有保留")
	}
}
