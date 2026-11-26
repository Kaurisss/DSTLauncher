package web

import (
	"net/http"
	"path/filepath"
	"testing"
)

func TestHostStatsWithoutRooms(t *testing.T) {
	srv := newAuthTestServer(t, "")
	rec, body := doJSON(t, srv.Handler(), http.MethodGet, "/api/host", nil, nil)
	if rec.Code != http.StatusOK || body["code"] != float64(200) {
		t.Fatalf("空房间列表也应能采样: %v", body)
	}
	data := body["data"].(map[string]interface{})
	if data["memory"] == nil || data["disk"] == nil || data["diskPath"] != filepath.Join(srv.rootDir, srv.confDir) {
		t.Fatalf("应采样实际存档盘: %v", data)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("资源采样不可缓存")
	}
}

func TestHostStatsRequiresAuthAndIsReadOnly(t *testing.T) {
	srv := newAuthTestServer(t, "secret123")
	h := srv.Handler()
	rec, _ := doJSON(t, h, http.MethodGet, "/api/host", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("资源接口应受鉴权保护: %d", rec.Code)
	}
	rec, body := doJSON(t, h, http.MethodPost, "/api/auth/login", map[string]string{"username": "admin", "password": "secret123"}, nil)
	if body["code"] != float64(200) {
		t.Fatalf("登录失败: %v", body)
	}
	cookies := rec.Result().Cookies()
	rec, body = doJSON(t, h, http.MethodGet, "/api/host", nil, cookies)
	if body["code"] != float64(200) {
		t.Fatalf("登录后应能采样: %v", body)
	}
	rec, _ = doJSON(t, h, http.MethodPut, "/api/host", nil, cookies)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("非 GET 请求应被拒绝: %d", rec.Code)
	}
}
