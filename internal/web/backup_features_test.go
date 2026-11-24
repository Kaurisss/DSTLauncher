package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dst-windows/internal/room"
)

func backupFeatureRoom(t *testing.T) (*Server, *room.Room) {
	t.Helper()
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if err = room.Generate(s.rootDir, s.confDir, rm); err != nil {
		t.Fatal(err)
	}
	tiles := base64.StdEncoding.EncodeToString([]byte{'V', 'R', 'S', 'N', 0, 1, 0, 0, 0, 7, 0, 201, 0, 7, 0, 201, 0})
	files := map[string]string{
		"Master/save/session/WORLD/0000000001":                `KLEI     1 return {map={width=2,height=2,tiles="` + tiles + `",world_tile_map={GRASS=7,OCEAN_COASTAL=201}},ents={pigking={{x=0,z=0}}}}`,
		"Master/save/session/WORLD/0000000001.meta":           `KLEI     1 return {clock={cycles=21},seasons={season="autumn",remainingdaysinseason=3}}`,
		"Master/save/session/WORLD/PLAYER_FIXTURE/0000000001": "saved-player",
	}
	for rel, body := range files {
		file := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), rel)
		if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return s, rm
}
func waitBackupCheck(t *testing.T, s *Server, id string) backupCheck {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/backups/check?id="+id, nil, nil)
		if body["code"] != float64(200) {
			t.Fatal(body)
		}
		b, _ := json.Marshal(body["data"])
		var result backupCheck
		if err := json.Unmarshal(b, &result); err != nil {
			t.Fatal(err)
		}
		if result.State != "checking" && result.State != "restoring" {
			return result
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("检查未完成")
	return backupCheck{}
}
func TestBackupChecksMapAndConfirmedRestoreKeepPreviousWorld(t *testing.T) {
	s, rm := backupFeatureRoom(t)
	defer s.StopBackupChecks()
	_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/backups/create?room=1", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	name := body["data"].(map[string]any)["name"].(string)
	_, body = doJSON(t, s.Handler(), http.MethodPost, "/api/backups/check?room=1", map[string]string{"name": name}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	id := body["data"].(map[string]any)["id"].(string)
	checked := waitBackupCheck(t, s, id)
	if checked.State != "ready" || !checked.Report.CanRestore || checked.Report.Files == 0 {
		t.Fatal(checked)
	}
	_, body = doJSON(t, s.Handler(), http.MethodGet, "/api/backups/check/map?id="+id+"&shard=Master", nil, nil)
	if body["code"] != float64(200) || body["data"].(map[string]any)["kind"] != "terrain" {
		t.Fatal(body)
	}
	_, body = doJSON(t, s.Handler(), http.MethodGet, "/api/backups/check/map?id="+id+"&shard=..%2foutside", nil, nil)
	if body["code"] == float64(200) {
		t.Fatal("越界读取了暂存目录")
	}
	_, body = doJSON(t, s.Handler(), http.MethodPost, "/api/backups/restore?room=1", map[string]string{"id": id, "sha256": "incorrect"}, nil)
	if body["code"] == float64(200) {
		t.Fatal("接受错误摘要")
	}
	current := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Master/save/session/WORLD/0000000001")
	if err := os.WriteFile(current, []byte("new-live-world"), 0600); err != nil {
		t.Fatal(err)
	}
	_, body = doJSON(t, s.Handler(), http.MethodPost, "/api/backups/restore?room=1", map[string]string{"id": id, "sha256": checked.Report.SHA256}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	result := waitBackupCheck(t, s, id)
	if result.State != "restored" {
		t.Fatal(result.Error)
	}
	b, err := os.ReadFile(current)
	if err != nil || string(b) == "new-live-world" {
		t.Fatal("未恢复", err)
	}
	b, err = os.ReadFile(filepath.Join(result.PreviousDirectory, "Master/save/session/WORLD/0000000001"))
	if err != nil || string(b) != "new-live-world" {
		t.Fatal("未保留恢复前存档", err)
	}
	_, body = doJSON(t, s.Handler(), http.MethodDelete, "/api/backups/check?id="+id, nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	if _, err = os.Stat(current); err != nil {
		t.Fatal("清理检查删除了已恢复存档", err)
	}
}
func TestBackupRemoteSettingsQueueManualAndAutomaticBackups(t *testing.T) {
	s, _ := backupFeatureRoom(t)
	stop := s.StartBackupRemote()
	defer stop()
	destination := t.TempDir()
	_, body := doJSON(t, s.Handler(), http.MethodPut, "/api/backup-remote", map[string]any{"enabled": true, "kind": "directory", "directory": destination, "keep": 3, "password": "HIDDEN-PASSWORD"}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	data, _ := json.Marshal(body)
	if strings.Contains(string(data), "HIDDEN-PASSWORD") {
		t.Fatal("密码回传")
	}
	_, body = doJSON(t, s.Handler(), http.MethodPost, "/api/backup-remote/test", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	_, body = doJSON(t, s.Handler(), http.MethodGet, "/api/backups/create?room=1", nil, nil)
	if body["code"] != float64(200) || body["data"].(map[string]any)["syncQueued"] != true {
		t.Fatal(body)
	}
	if _, err := s.runMaintenanceWithNotification(context.Background(), "backup", s.automation.Snapshot().Config); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := s.backupRemote.Snapshot()
		if len(snapshot.Results) == 2 && snapshot.Results[0].State == "synced" && snapshot.Results[1].State == "synced" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("手动/自动备份没有完成后台同步", s.backupRemote.Snapshot().Results)
}
func TestBackupFeatureEndpointsRequireAuthentication(t *testing.T) {
	s := newAuthTestServer(t, "password")
	for _, endpoint := range []string{"/api/backup-remote", "/api/backup-remote/test", "/api/backup-remote/sync?room=1", "/api/backups/check?room=1", "/api/backups/check/map?id=x", "/api/backups/restore?room=1"} {
		rec, _ := doJSON(t, s.Handler(), http.MethodPost, endpoint, map[string]any{}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatal(fmt.Sprint(endpoint, rec.Code))
		}
	}
}
