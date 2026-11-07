package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/roomprofile"
)

func waitMigration(t *testing.T, s *Server, id string) migrationJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/migration/job?id="+id, nil, nil)
		if body["code"] != float64(200) {
			t.Fatal(body)
		}
		b, _ := json.Marshal(body["data"])
		var job migrationJob
		if err := json.Unmarshal(b, &job); err != nil {
			t.Fatal(err)
		}
		if job.State != "working" {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("迁移任务超时")
	return migrationJob{}
}
func uploadMigration(t *testing.T, s *Server, archive []byte) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/migration/upload", bytes.NewReader(archive))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var body struct {
		Code    int
		Data    struct{ ID string }
		Message string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 {
		t.Fatal(body.Message)
	}
	return body.Data.ID
}
func TestMigrationAPIExportsPreviewsAndImportsAsNewRooms(t *testing.T) {
	s, rm := modShardFixture(t)
	defer s.StopMigrations()
	s.rooms.SetClusterRoot(config.ClusterRoot(s.rootDir, s.confDir))
	rm.Token, rm.Password = "KEEP-TOKEN", "KEEP-PASSWORD"
	rm.Shards[1].Disabled = true
	rm, err := s.rooms.Update(rm)
	if err != nil {
		t.Fatal(err)
	}
	if err := room.Generate(s.rootDir, s.confDir, rm); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Caves", "save", "session", "session", "0000000002")
	if err := config.WriteFileAtomic(path, []byte("saved-world")); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteFileAtomic(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "adminlist.txt"), []byte("KU_admin")); err != nil {
		t.Fatal(err)
	}
	profile, err := roomprofile.Capture(rm.ClusterDir(s.rootDir, s.confDir), rm, "迁移方案")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := roomprofile.New(filepath.Join(s.rootDir, "profiles")).Save(profile); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodPost, "/api/migration/export", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	id := body["data"].(map[string]any)["id"].(string)
	job := waitMigration(t, s, id)
	if job.State != "ready" {
		t.Fatal(job.Error)
	}
	b, _ := json.Marshal(job.Preview)
	if strings.Contains(string(b), "KEEP-") {
		t.Fatal("预览暴露了凭据")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/migration/download?id="+id, nil))
	if rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatal(rec.Body.String())
	}
	uploadID := uploadMigration(t, s, rec.Body.Bytes())
	preview := waitMigration(t, s, uploadID)
	if preview.State != "ready" || len(preview.Preview.Rooms) != 1 || preview.Preview.Profiles != 1 {
		t.Fatal(preview)
	}
	archivedPlan := s.automation.Snapshot().Config
	localPlan := archivedPlan
	localPlan.Keep = 7
	if err := s.automation.Configure(localPlan, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, body = doJSON(t, s.Handler(), http.MethodPost, "/api/migration/apply", map[string]any{"id": uploadID, "applyPlan": false}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	result := waitMigration(t, s, uploadID)
	if result.State != "ready" || len(result.RoomIDs) != 1 {
		t.Fatal(result)
	}
	created, _ := s.rooms.Get(result.RoomIDs[0])
	if s.automation.Snapshot().Config != localPlan {
		t.Fatal("默认覆盖了本机维护计划")
	}
	if created.ID == rm.ID || created.ClusterKey == rm.ClusterKey || created.MasterPort == rm.MasterPort || created.Token != rm.Token || created.Password != rm.Password || !created.Shards[1].Disabled {
		t.Fatal(created)
	}
	for _, rel := range []string{"adminlist.txt", "Caves/save/session/session/0000000002", "Master/modoverrides.lua", "Caves/modoverrides.lua"} {
		a, err := os.ReadFile(filepath.Join(created.ClusterDir(s.rootDir, s.confDir), rel))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), rel))
		if err != nil || !bytes.Equal(a, b) {
			t.Fatal(rel, err)
		}
	}
	_, body = doJSON(t, s.Handler(), http.MethodPost, "/api/migration/apply", map[string]any{"id": uploadID}, nil)
	if body["code"] == float64(200) {
		t.Fatal("同一任务重复导入")
	}
	second := uploadMigration(t, s, rec.Body.Bytes())
	if job := waitMigration(t, s, second); job.State != "ready" {
		t.Fatal(job)
	}
	_, body = doJSON(t, s.Handler(), http.MethodPost, "/api/migration/apply", map[string]any{"id": second, "applyPlan": true}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	if job := waitMigration(t, s, second); job.State != "ready" {
		t.Fatal(job)
	}
	if s.automation.Snapshot().Config != archivedPlan {
		t.Fatal("未应用明确选中的维护计划")
	}
}
func TestMigrationRollsBackPreparedDirectories(t *testing.T) {
	s, rm := modShardFixture(t)
	defer s.StopMigrations()
	s.rooms.SetClusterRoot(config.ClusterRoot(s.rootDir, s.confDir))
	if err := room.Generate(s.rootDir, s.confDir, rm); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodPost, "/api/migration/export", nil, nil)
	id := body["data"].(map[string]any)["id"].(string)
	if job := waitMigration(t, s, id); job.State != "ready" {
		t.Fatal(job)
	}
	s.migrations.mu.Lock()
	source := s.migrations.items[id].dir
	s.migrations.mu.Unlock()
	archive, err := os.ReadFile(filepath.Join(source, "migration.zip"))
	if err != nil {
		t.Fatal(err)
	}
	uploadID := uploadMigration(t, s, archive)
	if job := waitMigration(t, s, uploadID); job.State != "ready" {
		t.Fatal(job)
	}
	s.migrations.mu.Lock()
	j := s.migrations.items[uploadID]
	s.migrations.mu.Unlock()
	// A missing staged source simulates an I/O failure after the fresh target directory is created.
	if err := os.Rename(filepath.Join(j.dir, "payload", "rooms"), filepath.Join(j.dir, "unavailable")); err != nil {
		t.Fatal(err)
	}
	_, err = s.applyMigration(j, false)
	if err == nil {
		t.Fatal("缺少源文件却导入成功")
	}
	if len(s.rooms.List()) != 1 {
		t.Fatal("失败时发布了房间")
	}
	if _, err := os.Stat(config.ClusterDir(s.rootDir, s.confDir, 2)); !os.IsNotExist(err) {
		t.Fatal("未清理新目录", err)
	}
	if _, err := os.Stat(rm.ClusterDir(s.rootDir, s.confDir)); err != nil {
		t.Fatal("改变原房间", err)
	}
}
func TestMigrationAndMapRequireAuthenticationAndValidateShard(t *testing.T) {
	s := newAuthTestServer(t, "password")
	for _, path := range []string{"/api/migration/export", "/api/migration/upload", "/api/migration/apply", "/api/migration/job?id=x", "/api/migration/download?id=x", "/api/map?room=1&shard=Master"} {
		rec, _ := doJSON(t, s.Handler(), http.MethodPost, path, nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatal(path, rec.Code)
		}
	}
	s, rm := modShardFixture(t)
	for _, shard := range []string{"..%2f..%2foutside", "Unknown"} {
		_, body := doJSON(t, s.Handler(), http.MethodGet, fmt.Sprintf("/api/map?room=%d&shard=%s", rm.ID, shard), nil, nil)
		if body["code"] == float64(200) {
			t.Fatal(body)
		}
	}
}

func TestMigrationShutdownCancelsAnIncompleteUpload(t *testing.T) {
	s := newAuthTestServer(t, "")
	reader, writer := io.Pipe()
	defer writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/migration/upload", reader)
	done := make(chan struct{})
	go func() { defer close(done); s.Handler().ServeHTTP(httptest.NewRecorder(), req) }()
	if _, err := writer.Write([]byte("partial zip")); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() { s.StopMigrations(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("取消迁移时未中断上传")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("上传处理未退出")
	}
}

func TestMapAPIReadsSerializedWorldData(t *testing.T) {
	s, rm := modShardFixture(t)
	text := `local savedata = {}
local tablefunctions = {}
tablefunctions["map_fn"] = function()
return {width=2,height=2,tiles="TILE_FIXTURE",world_tile_map={GRASS=7,OCEAN_COASTAL=201}}
end
savedata["map"] = tablefunctions["map_fn"]()
return savedata` + "\x00"
	text = strings.Replace(text, "TILE_FIXTURE", base64.StdEncoding.EncodeToString([]byte{'V', 'R', 'S', 'N', 0, 1, 0, 0, 0, 7, 0, 201, 0, 7, 0, 201, 0}), 1)
	path := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Master", "save", "session", "WORLD_FIXTURE", "0000000001")
	if err := config.WriteFileAtomic(path, []byte(text)); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, fmt.Sprintf("/api/map?room=%d&shard=Master", rm.ID), nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	data := body["data"].(map[string]any)
	if data["kind"] != "terrain" || data["terrain"] == nil || data["snapshot"] != "0000000001" {
		t.Fatal(data)
	}
}
