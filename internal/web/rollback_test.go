package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dst-windows/internal/steamcmd"
)

func TestRollbackEndpointValidatesMethodRoomAndBody(t *testing.T) {
	api, rm := newWorldConfigServer(t, []string{"Master", "Caves"}, nil)
	api.steam = steamcmd.NewManager(t.TempDir(), "auto")
	for _, tc := range []struct{ method, query, body, message string }{
		{"GET", "?room=" + itoa(rm.ID), "", ""},
		{"POST", "", `{"count":1}`, "明确指定"},
		{"POST", "?room=99999", `{"count":1}`, "房间不存在"},
		{"POST", "?room=" + itoa(rm.ID), `{"count":1.5}`, "参数无效"},
		{"POST", "?room=" + itoa(rm.ID), `{"count":"1);os.exit()"}`, "参数无效"},
		{"POST", "?room=" + itoa(rm.ID), `{"count":1,"shard":"Caves"}`, "参数无效"},
		{"POST", "?room=" + itoa(rm.ID), `{"count":1} {}`, "一份参数"},
		{"POST", "?room=" + itoa(rm.ID), `{"count":0}`, "1-99"},
		{"POST", "?room=" + itoa(rm.ID), `{"count":100}`, "1-99"},
		{"POST", "?room=" + itoa(rm.ID), `{"count":1}`, "先启动"},
	} {
		t.Run(tc.method+tc.query+tc.body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			api.Handler().ServeHTTP(rec, httptest.NewRequest(tc.method, "/api/rollback"+tc.query, strings.NewReader(tc.body)))
			if tc.method != "POST" {
				if rec.Code != http.StatusMethodNotAllowed {
					t.Fatalf("GET 不能触发回档: %d", rec.Code)
				}
				return
			}
			var result struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code == 200 || !strings.Contains(result.Message, tc.message) {
				t.Fatalf("校验未生效: %s", rec.Body.String())
			}
		})
	}
}
