package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dst-windows/internal/room"
	"dst-windows/internal/server"
)

func TestPlayersRequireAuthAndExplicitRoom(t *testing.T) {
	protected := newAuthTestServer(t, "secret").Handler()
	for _, path := range []string{"/api/players?room=1", "/api/players/action?room=1"} {
		rec, _ := doJSON(t, protected, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("玩家接口应受鉴权保护: %s, %d", path, rec.Code)
		}
	}
	srv := newAuthTestServer(t, "")
	rm, err := srv.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	srv.mgr.Register(server.NewCluster(rm))
	h := srv.Handler()
	_, body := doJSON(t, h, http.MethodGet, "/api/players", nil, nil)
	if body["code"] == float64(200) {
		t.Fatal("查询不应隐式选择首个房间")
	}
	rec, body := doJSON(t, h, http.MethodGet, "/api/players?room=1", nil, nil)
	if body["code"] != float64(200) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("停止房间仍应返回不可缓存的空列表: %v", body)
	}
	data := body["data"].(map[string]interface{})
	if players, ok := data["players"].([]interface{}); !ok || len(players) != 0 {
		t.Fatalf("空列表应编码为数组: %v", data)
	}
	rec, body = doJSON(t, h, http.MethodGet, "/api/players?room=1&shard=Master", nil, nil)
	if body["code"] != float64(200) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("带合法 shard 参数查询停止房间应返回空列表: %v", body)
	}
	_, body = doJSON(t, h, http.MethodGet, "/api/players?room=1&shard=NoSuchShard", nil, nil)
	if body["code"] == float64(200) {
		t.Fatalf("非法 shard 参数应返回错误: %v", body)
	}
	rec, _ = doJSON(t, h, http.MethodPost, "/api/players?room=1", nil, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatal("列表只接受 GET")
	}
	rec, _ = doJSON(t, h, http.MethodGet, "/api/players/action?room=1", nil, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatal("玩家操作只能使用 POST")
	}
}

func TestPlayerActionRejectsUnsafeAndAmbiguousRequests(t *testing.T) {
	srv := newAuthTestServer(t, "")
	rm, err := srv.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	srv.mgr.Register(server.NewCluster(rm))
	h := srv.Handler()
	for _, tc := range []struct{ path, body string }{
		{"/api/players/action", `{"action":"ban","userid":"KU_test","shard":"Master"}`},
		{"/api/players/action?room=1", `{"action":"kick","userid":"KU_test","shard":"Master","cmd":"print('unsafe')"}`},
		{"/api/players/action?room=1", `{"action":"kick"} {"action":"ban"}`},
		{"/api/players/action?room=1", `{"action":"kick","userid":"KU_';os.exit()","shard":"Master"}`},
		{"/api/players/action?room=1", `{"action":"teleport","userid":"KU_test","shard":"../../Other","target":"KU_other"}`},
		{"/api/players/action?room=999", `{"action":"kick","userid":"KU_test","shard":"Master"}`},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
		var response map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response["code"] == float64(200) {
			t.Fatalf("非法玩家请求被接受: %s %s", tc.path, tc.body)
		}
	}
}
