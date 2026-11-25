package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicIPEndpointCachesAndRefreshes(t *testing.T) {
	s, _ := newWorldConfigServer(t, []string{"Master"}, nil)
	calls := 0
	s.publicIPFetcher = func(context.Context) publicIPState {
		calls++
		if calls == 1 {
			return publicIPState{IP: "203.0.113.10", Source: "stub", FetchedAt: time.Now().UTC()}
		}
		return publicIPState{IP: "203.0.113.20", Source: "stub", FetchedAt: time.Now().UTC()}
	}

	rec, _ := doJSON(t, s.Handler(), http.MethodGet, "/api/network/public-ip", nil, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"203.0.113.10"`) {
		t.Fatalf("unexpected first response: %d %s", rec.Code, rec.Body.String())
	}
	// 第二次应命中缓存
	rec, _ = doJSON(t, s.Handler(), http.MethodGet, "/api/network/public-ip", nil, nil)
	if calls != 1 || !strings.Contains(rec.Body.String(), `"203.0.113.10"`) {
		t.Fatalf("expected cache hit (calls=%d): %s", calls, rec.Body.String())
	}
	// refresh=1 应强制重新探测
	rec, _ = doJSON(t, s.Handler(), http.MethodGet, "/api/network/public-ip?refresh=1", nil, nil)
	if calls != 2 || !strings.Contains(rec.Body.String(), `"203.0.113.20"`) {
		t.Fatalf("expected refresh (calls=%d): %s", calls, rec.Body.String())
	}

	// 非 GET 拒绝
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/network/public-ip", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestRoomConnectUpdateAndPreservation(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Master"}, nil)
	path := "/api/rooms/connect?room=" + itoa(rm.ID)

	// 更新自定义直连地址（含按分片映射端口）
	rec, _ := doJSON(t, s.Handler(), http.MethodPut, path, map[string]any{
		"connectIp":    "play.example.com",
		"connectPort":  25565,
		"connectPorts": map[string]int{"Master": 25565, "Caves": 25566},
	}, nil)
	if !strings.Contains(rec.Body.String(), `"code":200`) || !strings.Contains(rec.Body.String(), `"play.example.com"`) || !strings.Contains(rec.Body.String(), `"Caves":25566`) {
		t.Fatalf("update connect failed: %s", rec.Body.String())
	}

	// 普通编辑未回传 connectIp/connectPort/connectPorts 时应保留原值
	rmCopy, ok := s.rooms.Get(rm.ID)
	if !ok {
		t.Fatal("room disappeared")
	}
	rmCopy.ConnectIP = ""
	rmCopy.ConnectPort = 0
	rmCopy.ConnectPorts = nil
	rmCopy.Name = "改名后房间"
	updated, err := s.rooms.Update(rmCopy)
	if err != nil {
		t.Fatalf("room update failed: %v", err)
	}
	if updated.ConnectIP != "play.example.com" || updated.ConnectPort != 25565 || updated.ConnectPorts["Caves"] != 25566 {
		t.Fatalf("custom connect lost after room update: %+v", updated)
	}

	// 非法引号/端口应被拒绝
	for _, bad := range []map[string]any{
		{"connectIp": `1.2.3.4");os.exit()`, "connectPort": 10999},
		{"connectIp": "1.2.3.4", "connectPort": 70000},
	} {
		badRec, _ := doJSON(t, s.Handler(), http.MethodPut, path, bad, nil)
		if strings.Contains(badRec.Body.String(), `"code":200`) {
			t.Fatalf("invalid connect payload accepted: %s", badRec.Body.String())
		}
	}

	// 传空值可清除自定义直连
	clearRec, _ := doJSON(t, s.Handler(), http.MethodPut, path, map[string]any{
		"connectIp":   "",
		"connectPort": 0,
	}, nil)
	if !strings.Contains(clearRec.Body.String(), `"code":200`) {
		t.Fatalf("clear connect failed: %s", clearRec.Body.String())
	}
	cleared, _ := s.rooms.Get(rm.ID)
	if cleared.ConnectIP != "" || cleared.ConnectPort != 0 {
		t.Fatalf("connect not cleared: %+v", cleared)
	}
}

func TestRegenerateQuickActionValidation(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Master"}, nil)
	path := "/api/quick-action?room=" + itoa(rm.ID)
	// 带 message 应拒绝，未运行分片应返回错误而非 200
	rec, _ := doJSON(t, s.Handler(), http.MethodPost, path, map[string]string{
		"action":  "regenerate",
		"message": "unexpected",
	}, nil)
	if strings.Contains(rec.Body.String(), `"code":200`) {
		t.Fatalf("regenerate with message should fail: %s", rec.Body.String())
	}
	rec, _ = doJSON(t, s.Handler(), http.MethodPost, path, map[string]string{
		"action": "regenerate",
	}, nil)
	if !strings.Contains(rec.Body.String(), "请先启动主分片") {
		t.Fatalf("expected master-not-running error, got: %s", rec.Body.String())
	}
}
