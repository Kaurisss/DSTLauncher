package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatEndpointScopedHistoryAndAuthenticated(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface", "Below"}, nil)
	dir := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Below")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server_chat_log.txt"), []byte("[00:00:01]: [Say] (KU_A-1) A: hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := "/api/chat-log?room=" + itoa(rm.ID) + "&shard=Below"
	rec, body := doJSON(t, s.Handler(), "GET", path, nil, nil)
	if body["code"] != float64(200) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(body)
	}
	data := body["data"].(map[string]any)
	if !data["exists"].(bool) || len(data["entries"].([]any)) != 1 {
		t.Fatal(data)
	}
	for _, bad := range []string{"/api/chat-log", path + "&limit=0", path + "&kind=unknown", path + "&page=0", path + "&page=-1", path + "&page=abc", "/api/chat-log?room=" + itoa(rm.ID) + "&shard=../../outside"} {
		_, body := doJSON(t, s.Handler(), "GET", bad, nil, nil)
		if body["code"] == float64(200) {
			t.Fatalf("invalid scope/query accepted: %s", bad)
		}
	}
	rec, _ = doJSON(t, s.Handler(), "POST", path, nil, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatal("unsupported method accepted")
	}
	s.envPassword = "test-password"
	rec, _ = doJSON(t, s.Handler(), "GET", path, nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("chat content exposed without auth")
	}
	rec, _ = doJSON(t, s.Handler(), "DELETE", path, map[string]any{"ids": []int64{1}}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("unauthenticated archive deletion accepted")
	}
}

func TestPlayerHistoryDeletionPreservesGameLogsAndRequiresExplicitScope(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface", "Below"}, nil)
	dir := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Below")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	original := "[00:00:01]: [Join Announcement] 玩家 A\n[00:00:02]: [Death Announcement] 玩家 A was killed by a hound.\n"
	if err := os.WriteFile(filepath.Join(dir, "server_chat_log.txt"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server_log.txt"), []byte("[00:00:01]: Client authenticated: (KU_A) 玩家 A\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := "/api/chat-log?room=" + itoa(rm.ID) + "&shard=Below"
	_, body := doJSON(t, s.Handler(), "GET", path+"&page=1&limit=1&kind=death&search=KU_A", nil, nil)
	data, ok := body["data"].(map[string]any)
	if body["code"] != float64(200) || !ok || data["total"] != float64(1) || data["pageSize"] != float64(1) {
		t.Fatal(body)
	}
	entry := data["entries"].([]any)[0].(map[string]any)
	if entry["userid"] != "KU_A" || entry["kind"] != "death" {
		t.Fatal(entry)
	}
	for _, bad := range []struct {
		path string
		body any
	}{
		{"/api/chat-log", map[string]any{"ids": []int64{2}}},
		{"/api/chat-log?room=" + itoa(rm.ID), map[string]any{"ids": []int64{2}}},
		{path, map[string]any{"ids": []int64{0}}},
		{path, map[string]any{"ids": []int64{2}, "unknown": true}},
	} {
		_, body := doJSON(t, s.Handler(), "DELETE", bad.path, bad.body, nil)
		if body["code"] == float64(200) {
			t.Fatalf("accepted deletion: %+v", bad)
		}
	}
	_, body = doJSON(t, s.Handler(), "DELETE", path, map[string]any{"ids": []int64{2}}, nil)
	if body["code"] != float64(200) || body["data"].(map[string]any)["deleted"] != float64(1) {
		t.Fatal(body)
	}
	_, body = doJSON(t, s.Handler(), "GET", path+"&page=1&limit=10", nil, nil)
	if body["data"].(map[string]any)["total"] != float64(1) {
		t.Fatal(body)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "server_chat_log.txt"))
	if err != nil || string(raw) != original {
		t.Fatal("archive deletion modified the original game log", err)
	}
}

func TestPlayerHistoryCollectionRunsWithoutOpeningTheLogPage(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface"}, nil)
	dir := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Surface")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "server_chat_log.txt")
	if err := os.WriteFile(path, []byte("[00:00:01]: [Announcement] captured without browser\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stop := s.StartPlayerLogs()
	stop()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), "GET", "/api/chat-log?room="+itoa(rm.ID)+"&shard=Surface&page=1&limit=10", nil, nil)
	if body["code"] != float64(200) || body["data"].(map[string]any)["total"] != float64(1) {
		t.Fatal("background collector did not preserve history", body)
	}
}

func TestChatLogStreamNDJSONParityAndAuth(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface"}, nil)
	dir := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Surface")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "[00:00:01]: [Say] (KU_A) A: msg-1\n" +
		"[00:00:02]: [Say] (KU_B) B: msg-2\n" +
		"[00:00:03]: [Join Announcement] C\n" +
		"[00:00:04]: [Say] (KU_A) A: msg-3\n"
	if err := os.WriteFile(filepath.Join(dir, "server_chat_log.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	basePath := "/api/chat-log?room=" + itoa(rm.ID) + "&shard=Surface&kind=all&page=1&limit=2"

	// Classic JSON response for parity check.
	_, jsonBody := doJSON(t, s.Handler(), "GET", basePath, nil, nil)
	jsonData := jsonBody["data"].(map[string]any)
	jsonEntries := jsonData["entries"].([]any)
	if len(jsonEntries) != 2 || jsonData["total"] != float64(4) {
		t.Fatalf("unexpected json baseline: %+v", jsonData)
	}

	// NDJSON stream response.
	req := httptest.NewRequest(http.MethodGet, basePath+"&stream=1", nil)
	req.Header.Set("Accept", "application/x-ndjson, application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-ndjson") {
		t.Fatalf("expected ndjson content-type, got %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", rec.Header().Get("Cache-Control"))
	}

	var events []map[string]any
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("invalid ndjson line %q: %v", line, err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 { // 1 meta + 2 entries + 1 done
		t.Fatalf("expected 4 ndjson events, got %d: %+v", len(events), events)
	}
	if events[0]["type"] != "meta" || events[0]["total"] != float64(4) || events[0]["count"] != float64(2) || events[0]["exists"] != true {
		t.Fatalf("unexpected meta event: %+v", events[0])
	}
	if events[1]["type"] != "entry" || events[2]["type"] != "entry" || events[3]["type"] != "done" {
		t.Fatalf("unexpected event sequence: %+v", events)
	}
	for i := range 2 {
		wantID := jsonEntries[i].(map[string]any)["id"]
		gotID := events[i+1]["entry"].(map[string]any)["id"]
		if wantID != gotID {
			t.Fatalf("entry %d id mismatch: want %v got %v", i, wantID, gotID)
		}
	}

	// Auth still blocks stream mode.
	s.envPassword = "secret"
	req = httptest.NewRequest(http.MethodGet, basePath+"&stream=1", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}
