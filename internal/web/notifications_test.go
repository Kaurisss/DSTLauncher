package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dst-windows/internal/automation"
	"dst-windows/internal/config"
	"dst-windows/internal/notify"
	"dst-windows/internal/room"
)

func TestNotificationAPIAndRealBackupFailures(t *testing.T) {
	got := make(chan notify.Event, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event notify.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Error(err)
		}
		got <- event
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := room.Generate(s.rootDir, s.confDir, rm); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	_, body := doJSON(t, h, http.MethodPut, "/api/notifications", map[string]any{"enabled": true, "url": receiver.URL, "secret": "secret-value", "events": []string{"backup_failed"}, "rooms": []int{rm.ID}}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	b, _ := json.Marshal(body)
	if strings.Contains(string(b), "secret-value") || !body["data"].(map[string]any)["secretSet"].(bool) {
		t.Fatal(body)
	}
	stop := s.StartNotifications()
	defer stop()
	_, body = doJSON(t, h, http.MethodGet, "/api/backups/create?room=1", nil, nil)
	if body["code"] == float64(200) {
		t.Fatal("没有存档却备份成功")
	}
	select {
	case event := <-got:
		if event.Event != "backup_failed" || event.RoomID != rm.ID {
			t.Fatal(event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("手动备份失败未通知")
	}
	if err := config.WriteFileAtomic(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Master", "save", "world"), []byte("saved world")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.rootDir, "backups"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = s.runMaintenanceWithNotification(context.Background(), "backup", automation.Config{Keep: 2})
	if err == nil {
		t.Fatal("备份目录不可用却未报错")
	}
	select {
	case event := <-got:
		if event.Event != "backup_failed" || event.RoomID != 0 {
			t.Fatal(event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("自动备份失败未通知")
	}
	_, body = doJSON(t, h, http.MethodPost, "/api/notifications/test", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	select {
	case event := <-got:
		if event.Event != "test" {
			t.Fatal(event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("未收到测试通知")
	}
}
