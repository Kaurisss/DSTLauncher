package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dst-windows/internal/room"
	"dst-windows/internal/server"
	"dst-windows/internal/worldsettings"
)

const saveTargetMaster = `return {
  id="MASTER_SHIPWRECKED",
  location="shipwrecked",
  overrides={
    dragoonegg="default",
  },
}
`
const saveTargetVolcano = `return {
  id="VOLCANO_ONLY",
  location="volcanoworld",
  overrides={
    dragoonegg="default",
  },
}
`

func putWorldConfig(t *testing.T, api *Server, url string, payload any) (int, string) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body)))
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Code, result.Message
}

func TestWorldConfigSaveUsesBodyShardBeforeReading(t *testing.T) {
	api, rm := newWorldConfigServer(t, []string{"Master", "Caves"}, map[string]string{
		"Master": saveTargetMaster, "Caves": saveTargetVolcano,
	})
	code, msg := putWorldConfig(t, api, "/api/worldconfig?room="+itoa(rm.ID), map[string]any{
		"shard": "Caves", "overrides": map[string]string{"dragoonegg": "never"},
	})
	if code != 200 {
		t.Fatalf("保存失败: %s", msg)
	}
	for shard, want := range map[string]string{
		"Master": saveTargetMaster,
		"Caves":  strings.Replace(saveTargetVolcano, `dragoonegg="default"`, `dragoonegg="never"`, 1),
	} {
		got, err := os.ReadFile(worldsettings.LevelDataPath(rm.ClusterDir(api.rootDir, api.confDir), shard))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s 被写错，期望只改火山 dragoonegg:\n%s", shard, got)
		}
	}
}

func TestWorldConfigSaveRestoresRuleDefault(t *testing.T) {
	before := strings.Replace(saveTargetMaster, `dragoonegg="default"`, `dragoonegg="never"`, 1)
	api, rm := newWorldConfigServer(t, []string{"Master"}, map[string]string{"Master": before})
	code, msg := putWorldConfig(t, api, "/api/worldconfig", map[string]any{
		"room": rm.ID, "shard": "Master", "overrides": map[string]string{"dragoonegg": "default"},
	})
	if code != 200 {
		t.Fatalf("恢复默认失败: %s", msg)
	}
	view := getWorldConfig(t, api, itoa(rm.ID), "Master")
	if view.Config.Overrides["dragoonegg"] != "default" {
		t.Fatal("默认值没有持久化")
	}
	b, err := os.ReadFile(worldsettings.LevelDataPath(rm.ClusterDir(api.rootDir, api.confDir), "Master"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != saveTargetMaster {
		t.Fatal("恢复默认误改了其他字节")
	}
}

func TestWorldConfigSavePreservesOrdinaryOverrides(t *testing.T) {
	api, rm := newWorldConfigServer(t, []string{"Master"}, nil)
	original := &worldsettings.Override{
		Enabled: true, Overrides: map[string]string{"world_size": "huge", "weather": "never", "unknown_mod_key": "custom"},
	}
	cluster := rm.ClusterDir(api.rootDir, api.confDir)
	if err := worldsettings.WriteFile(cluster, "Master", original); err != nil {
		t.Fatal(err)
	}
	view := getWorldConfig(t, api, itoa(rm.ID), "Master")
	view.Config.Overrides["weather"] = "often"
	code, msg := putWorldConfig(t, api, "/api/worldconfig?room="+itoa(rm.ID)+"&shard=Master", view.Config)
	if code != 200 {
		t.Fatalf("保存失败: %s", msg)
	}
	again := getWorldConfig(t, api, itoa(rm.ID), "Master")
	for name, want := range map[string]string{"world_size": "huge", "weather": "often", "unknown_mod_key": "custom"} {
		if again.Config.Overrides[name] != want {
			t.Errorf("%s 没有保留: %v", name, again.Config.Overrides)
		}
	}
}

func TestWorldConfigSaveUsesBodyRoom(t *testing.T) {
	api, first := newWorldConfigServer(t, []string{"Master", "Caves"}, map[string]string{
		"Master": saveTargetMaster, "Caves": saveTargetVolcano,
	})
	second, err := api.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	api.mgr.Register(server.NewCluster(second))
	path := worldsettings.LevelDataPath(second.ClusterDir(api.rootDir, api.confDir), "Caves")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(saveTargetVolcano), 0o644); err != nil {
		t.Fatal(err)
	}
	code, msg := putWorldConfig(t, api, "/api/worldconfig", map[string]any{
		"room": second.ID, "shard": "Caves", "overrides": map[string]string{"dragoonegg": "never"},
	})
	if code != 200 {
		t.Fatalf("保存失败: %s", msg)
	}
	got := getWorldConfig(t, api, itoa(second.ID), "Caves")
	if got.Location != "volcanoworld" || got.Config.Overrides["dragoonegg"] != "never" {
		t.Fatalf("第二个房间保存结果错误: location=%s dragoonegg=%s", got.Location, got.Config.Overrides["dragoonegg"])
	}
	for shard, want := range map[string]string{"Master": saveTargetMaster, "Caves": saveTargetVolcano} {
		b, err := os.ReadFile(worldsettings.LevelDataPath(first.ClusterDir(api.rootDir, api.confDir), shard))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Errorf("错误改动第一个房间的 %s", shard)
		}
	}
}

func TestWorldConfigSaveRejectsConflictingTargets(t *testing.T) {
	api, rm := newWorldConfigServer(t, []string{"Master", "Caves"}, map[string]string{
		"Master": saveTargetMaster, "Caves": saveTargetVolcano,
	})
	for _, tc := range []struct {
		query string
		body  map[string]any
	}{
		{"?room=" + itoa(rm.ID) + "&shard=Master", map[string]any{"room": rm.ID, "shard": "Caves"}},
		{"?room=" + itoa(rm.ID), map[string]any{"room": rm.ID + 1, "shard": "Master"}},
	} {
		t.Run(tc.query+fmt.Sprint(tc.body), func(t *testing.T) {
			tc.body["overrides"] = map[string]string{"dragoonegg": "never"}
			code, _ := putWorldConfig(t, api, "/api/worldconfig"+tc.query, tc.body)
			if code == 200 {
				t.Error("互相冲突的目标必须拒绝")
			}
		})
	}
	for shard, want := range map[string]string{"Master": saveTargetMaster, "Caves": saveTargetVolcano} {
		b, err := os.ReadFile(worldsettings.LevelDataPath(rm.ClusterDir(api.rootDir, api.confDir), shard))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Errorf("拒绝请求却修改了 %s", shard)
		}
	}
}
