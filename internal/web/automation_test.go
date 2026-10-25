package web

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dst-windows/internal/automation"
	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/server"
)

func TestAutomationAPIValidationPersistenceAndAuth(t *testing.T) {
	s := newAuthTestServer(t, "")
	h := s.Handler()
	_, body := doJSON(t, h, http.MethodGet, "/api/automation", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	c := automation.Config{BackupHours: 2, Keep: 5, RestartAt: "04:00", UpdateMinutes: 30, UpdateMode: "idle"}
	_, body = doJSON(t, h, http.MethodPut, "/api/automation", c, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	reloaded, err := automation.New(filepath.Join(s.rootDir, "automation.json"))
	if err != nil || reloaded.Snapshot().Config != c {
		t.Fatal(reloaded, err)
	}
	c.Keep = 0
	_, body = doJSON(t, h, http.MethodPut, "/api/automation", c, nil)
	if body["code"] == float64(200) || s.automation.Snapshot().Config.Keep != 5 {
		t.Fatal(body)
	}
	locked := newAuthTestServer(t, "secret123")
	for _, path := range []string{"/api/automation", "/api/worldstate"} {
		rec, _ := doJSON(t, locked.Handler(), http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s 未鉴权", path)
		}
	}
}

func TestMaintenanceExcludesMutationsButKeepsStatusReadable(t *testing.T) {
	s := newAuthTestServer(t, "")
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	for _, path := range []string{"/api/start", "/api/stop", "/api/restart", "/api/cmd", "/api/mods/config", "/api/backups/create", "/api/backups/restore"} {
		_, body := doJSON(t, s.Handler(), http.MethodGet, path, nil, nil)
		if body["message"] != "自动维护正在执行，请完成后再操作" {
			t.Fatalf("%s: %v", path, body)
		}
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/automation", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
}

func TestAutomaticBackupAndWorldStateOffline(t *testing.T) {
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	s.mgr.Register(server.NewCluster(rm))
	dir := filepath.Join(config.ClusterDir(s.rootDir, s.confDir, rm.ID), rm.MasterShard(), "save", "session", "AAA")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0000000001.meta"), []byte(`return {clock={cycles=15},seasons={season="winter",remainingdaysinseason=0}}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := s.runMaintenance(context.Background(), "backup", automation.Config{Keep: 2})
	if err != nil || result != "已备份 1 个房间" {
		t.Fatal(result, err)
	}
	items, err := s.bakFor(rm).List()
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/worldstate", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	rows := body["data"].([]interface{})
	found := false
	for _, row := range rows {
		item := row.(map[string]interface{})
		if item["shard"] == rm.MasterShard() {
			state := item["state"].(map[string]interface{})
			if state["day"] != float64(16) || state["remainingDays"] != float64(0) {
				t.Fatal(state)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("未返回主世界")
	}
}

func TestTargetsNeverStartStoppedWorlds(t *testing.T) {
	got, err := maintenanceTargets([]server.Status{{Shard: "Master", State: "running"}, {Shard: "Caves", State: "stopped"}})
	if err != nil || len(got) != 1 || got[0].Shard != "Master" {
		t.Fatal(got, err)
	}
	if _, err := maintenanceTargets([]server.Status{{State: "starting"}}); err == nil {
		t.Fatal("启停中的世界必须阻止维护")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if pause(ctx, time.Hour) == nil {
		t.Fatal("退出时倒计时未取消")
	}
}
