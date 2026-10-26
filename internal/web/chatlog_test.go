package web

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestChatEndpointScopedReadOnlyAndAuthenticated(t *testing.T) {
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
	for _, bad := range []string{"/api/chat-log", path + "&limit=0", path + "&kind=unknown", "/api/chat-log?room=" + itoa(rm.ID) + "&shard=../../outside"} {
		_, body := doJSON(t, s.Handler(), "GET", bad, nil, nil)
		if body["code"] == float64(200) {
			t.Fatalf("invalid scope/query accepted: %s", bad)
		}
	}
	rec, _ = doJSON(t, s.Handler(), "DELETE", path, nil, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatal("chat API should be read only")
	}
	s.envPassword = "test-password"
	rec, _ = doJSON(t, s.Handler(), "GET", path, nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("chat content exposed without auth")
	}
}
