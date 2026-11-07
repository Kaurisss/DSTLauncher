package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/roomprofile"
	"dst-windows/internal/server"
)

func TestProfileAPICopyImportExportAndIsolation(t *testing.T) {
	s, rm := modShardFixture(t)
	s.rooms.SetClusterRoot(config.ClusterRoot(s.rootDir, s.confDir))
	rm.Token, rm.Password = "PRIVATE-TOKEN", "PRIVATE-PASSWORD"
	rm.Shards = append(rm.Shards, room.Shard{Name: "Island", WorldType: room.WorldTypeForest}, room.Shard{Name: "Volcano", WorldType: room.WorldTypeForest})
	rm.Shards[1].Disabled = true
	var err error
	rm, err = s.rooms.Update(rm)
	if err != nil {
		t.Fatal(err)
	}
	if err := room.Generate(s.rootDir, s.confDir, rm); err != nil {
		t.Fatal(err)
	}
	for shard, location := range map[string]string{"Island": "shipwrecked", "Volcano": "volcanoworld"} {
		if err := os.Remove(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), shard, "worldgenoverride.lua")); err != nil {
			t.Fatal(err)
		}
		if err := config.WriteFileAtomic(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), shard, "leveldataoverride.lua"), []byte(`return {location="`+location+`",overrides={task_set="islandadventures"}}`)); err != nil {
			t.Fatal(err)
		}
		if err := config.WriteFileAtomic(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), shard, "modoverrides.lua"), []byte(`return {["workshop-987654"]={enabled=true}}`)); err != nil {
			t.Fatal(err)
		}
	}
	world := `return {override_enabled=true,worldgen_preset="SURVIVAL_TOGETHER",overrides={season_start="autumn"}}`
	if err := config.WriteFileAtomic(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Master", "worldgenoverride.lua"), []byte(world)); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteFileAtomic(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Master", "save", "world"), []byte("SAVE-NOT-TO-COPY")); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	_, saved := doJSON(t, h, http.MethodPost, "/api/profiles", map[string]any{"room": rm.ID, "name": "朋友联机"}, nil)
	if saved["code"] != float64(200) {
		t.Fatal(saved)
	}
	id := saved["data"].(map[string]any)["id"].(string)
	export := httptest.NewRecorder()
	h.ServeHTTP(export, httptest.NewRequest(http.MethodGet, "/api/profiles?id="+id, nil))
	if strings.Contains(export.Body.String(), "PRIVATE-") || strings.Contains(export.Body.String(), "SAVE-NOT-TO-COPY") {
		t.Fatal("导出包含凭据或存档")
	}
	var portable roomprofile.Profile
	if err := json.Unmarshal(export.Body.Bytes(), &portable); err != nil {
		t.Fatal(err)
	}
	_, imported := doJSON(t, h, http.MethodPost, "/api/profiles/import", portable, nil)
	if imported["code"] != float64(200) {
		t.Fatal(imported)
	}
	for _, path := range []string{"/api/profiles/apply", "/api/rooms/copy"} {
		_, body := doJSON(t, h, http.MethodPost, path, map[string]any{"id": id, "room": rm.ID, "name": "新房间"}, nil)
		if body["code"] != float64(200) {
			t.Fatal(body)
		}
		createdID := int(body["data"].(map[string]any)["id"].(float64))
		created, _ := s.rooms.Get(createdID)
		if created.Token != "" || created.Password != "" || created.ClusterKey == rm.ClusterKey || created.Shards[0].ServerPort == rm.Shards[0].ServerPort {
			t.Fatal("新房间未隔离", created)
		}
		if len(created.Shards) != 4 || !created.Shards[1].Disabled {
			t.Fatal("未保留四世界和停用状态")
		}
		for _, shard := range []string{"Island", "Volcano"} {
			if _, err := os.Stat(filepath.Join(created.ClusterDir(s.rootDir, s.confDir), shard, "worldgenoverride.lua")); !os.IsNotExist(err) {
				t.Fatal("向已有海难世界定义叠加了地表默认设置", shard, err)
			}
			actual, _ := os.ReadFile(filepath.Join(created.ClusterDir(s.rootDir, s.confDir), shard, "leveldataoverride.lua"))
			source, _ := os.ReadFile(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), shard, "leveldataoverride.lua"))
			if string(actual) != string(source) {
				t.Fatal("丢失海难世界配置")
			}
		}
		for _, shard := range rm.Shards {
			rel := filepath.Join(shard.Name, "modoverrides.lua")
			a, err := os.ReadFile(filepath.Join(created.ClusterDir(s.rootDir, s.confDir), rel))
			if err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), rel))
			if string(a) != string(b) {
				t.Fatal("分片模组配置被混合")
			}
		}
		b, _ := os.ReadFile(filepath.Join(created.ClusterDir(s.rootDir, s.confDir), "Master", "worldgenoverride.lua"))
		if string(b) != world {
			t.Fatal("丢失世界配置")
		}
		if created.ExistsSave(s.rootDir, s.confDir) {
			t.Fatal("复制了运行存档")
		}
	}
	_, deleted := doJSON(t, h, http.MethodDelete, "/api/profiles?id="+id, nil, nil)
	if deleted["code"] != float64(200) {
		t.Fatal(deleted)
	}
	_, bad := doJSON(t, h, http.MethodGet, "/api/profiles?id=..%2foutside", nil, nil)
	if bad["code"] == float64(200) {
		t.Fatal(bad)
	}
}

func TestNewFeatureAPIsRequireAuthAndRespectMaintenance(t *testing.T) {
	locked := newAuthTestServer(t, "secret123")
	for _, endpoint := range []string{"/api/profiles", "/api/profiles/import", "/api/profiles/apply", "/api/rooms/copy", "/api/diagnostics?room=1", "/api/notifications", "/api/notifications/test"} {
		rec, _ := doJSON(t, locked.Handler(), http.MethodPost, endpoint, map[string]any{}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatal(endpoint, rec.Code)
		}
	}
	s := newAuthTestServer(t, "")
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	for _, endpoint := range []string{"/api/profiles", "/api/profiles/import", "/api/profiles/apply", "/api/rooms/copy"} {
		_, body := doJSON(t, s.Handler(), http.MethodPost, endpoint, map[string]any{}, nil)
		if body["message"] != "自动维护正在执行，请完成后再操作" {
			t.Fatal(endpoint, body)
		}
	}
}

func TestDiagnosticAPIIncludesEvidenceAndRedactsToken(t *testing.T) {
	s, rm := modShardFixture(t)
	rm.Token = "PRIVATE-TOKEN"
	if _, err := s.rooms.Update(rm); err != nil {
		t.Fatal(err)
	}
	s.mgr.Register(server.NewCluster(rm))
	if err := s.mgr.StartShard(rm.Key(), "Master"); err == nil {
		t.Fatal("未安装服务端却启动成功")
	}
	if err := config.WriteFileAtomic(filepath.Join(s.rootDir, fmt.Sprintf("%s_Master.log", rm.Key())), []byte("Invalid server token PRIVATE-TOKEN\nLUA ERROR stack traceback:\nmods/workshop-100/modmain.lua:23: bad value\n")); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/diagnostics?room="+fmt.Sprint(rm.ID), nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	b, _ := json.Marshal(body)
	if strings.Contains(string(b), "PRIVATE-TOKEN") || !strings.Contains(string(b), "workshop-100") || !strings.Contains(string(b), "bad value") || !strings.Contains(string(b), "start_failed") {
		t.Fatal(string(b))
	}
}
