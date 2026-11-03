package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dst-windows/internal/room"
)

func TestPlayerListEndpointsPersistAndRefreshReservations(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface"}, nil)
	if err := room.Generate(s.rootDir, s.confDir, rm); err != nil {
		t.Fatal(err)
	}
	path := "/api/player-lists?room=" + itoa(rm.ID) + "&kind=whitelist"
	for _, method := range []string{http.MethodPost, http.MethodPost, http.MethodGet} {
		var input any
		if method == http.MethodPost {
			input = map[string]string{"kleiId": "ku_player-1"}
		}
		rec, body := doJSON(t, s.Handler(), method, path, input, nil)
		if body["code"] != float64(200) || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("request failed: %v", body)
		}
		data := body["data"].(map[string]any)
		if len(data["ids"].([]any)) != 1 || data["reservedSlots"] != float64(1) {
			t.Fatalf("reservation not updated: %v", data)
		}
	}
	b, err := os.ReadFile(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "cluster.ini"))
	if err != nil || !strings.Contains(string(b), "whitelist_slots = 1") {
		t.Fatalf("cluster config: %s, %v", b, err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodDelete, path, map[string]string{"kleiId": "KU_player-1"}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	b, _ = os.ReadFile(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "cluster.ini"))
	if !strings.Contains(string(b), "whitelist_slots = 0") {
		t.Fatal("removal did not refresh reservations")
	}
}

func TestPlayerListScopeValidationAuthAndMaintenance(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface"}, nil)
	h := s.Handler()
	base := "/api/player-lists?room=" + itoa(rm.ID)
	for _, tc := range []struct {
		method, path string
		input        any
	}{
		{"GET", "/api/player-lists?kind=blocklist", nil},
		{"GET", base + "&kind=../../secret", nil},
		{"POST", base + "&kind=blocklist", map[string]any{"kleiId": "KU_test", "other": true}},
		{"POST", base + "&kind=blocklist", map[string]string{"kleiId": "KU_bad\nKU_other"}},
	} {
		_, body := doJSON(t, h, tc.method, tc.path, tc.input, nil)
		if body["code"] == float64(200) {
			t.Fatalf("invalid request accepted: %+v", tc)
		}
	}
	s.maintenanceMu.Lock()
	_, body := doJSON(t, h, "POST", base+"&kind=blocklist", map[string]string{"kleiId": "KU_test"}, nil)
	s.maintenanceMu.Unlock()
	if body["code"] == float64(200) {
		t.Fatal("write accepted during maintenance")
	}
	s.envPassword = "test-password"
	rec, _ := doJSON(t, h, "GET", base+"&kind=blocklist", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("player lists exposed without auth")
	}
}

func TestPlayerListReportsSeenPlayersWithoutWritingNames(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface"}, nil)
	if err := room.Generate(s.rootDir, s.confDir, rm); err != nil {
		t.Fatal(err)
	}
	dir := rm.ClusterDir(s.rootDir, s.confDir)
	log := "[00:00:00]: Current time: Mon Jan  2 15:04:05 2006\n" +
		"[00:01:00]: Client authenticated: (KU_player-1) 张三\n" +
		"[00:02:00]: User ID KU_player-1 assigned ownership to entity 9 - willow\n"
	if err := os.MkdirAll(filepath.Join(dir, "Surface"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Surface", "server_log.txt"), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	path := "/api/player-lists?room=" + itoa(rm.ID) + "&kind=blocklist"
	_, body := doJSON(t, s.Handler(), http.MethodPost, path, map[string]string{"kleiId": "ku_player-1"}, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	data := body["data"].(map[string]any)
	face := data["seen"].(map[string]any)["ku_player-1"].(map[string]any)
	if face["id"] != "KU_player-1" || face["name"] != "张三" || face["prefab"] != "willow" {
		t.Fatalf("seen: %v", data["seen"])
	}
	raw, err := os.ReadFile(filepath.Join(dir, "blocklist.txt"))
	if err != nil || string(raw) != "KU_player-1\n" {
		t.Fatalf("list file: %q %v", raw, err)
	}
	_, body = doJSON(t, s.Handler(), http.MethodGet, "/api/admins?room="+itoa(rm.ID), nil, nil)
	adminFace := body["data"].(map[string]any)["seen"].(map[string]any)["ku_player-1"].(map[string]any)
	if adminFace["name"] != "张三" {
		t.Fatal(body)
	}
}
