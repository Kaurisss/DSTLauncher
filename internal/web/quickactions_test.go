package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuickEndpointRejectsMissingScopeUnknownActionsAndMalformedBodies(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Surface"}, nil)
	path := "/api/quick-action?room=" + itoa(rm.ID)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", path, ""},
		{"POST", "/api/quick-action", `{"action":"save"}`},
		{"POST", path, `{"action":"announce","message":"welcome","cmd":"os.exit()"}`},
		{"POST", path, `{"action":"save"} {}`},
		{"POST", path, `{"action":"announce","message":"a\nb"}`},
		{"POST", path, `{"action":"unknown"}`},
		{"POST", path, `{"action":"save"}`},
	} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if tc.method == "GET" {
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatal("GET mutated state")
			}
			continue
		}
		if strings.Contains(rec.Body.String(), `"code":200`) {
			t.Fatalf("invalid/no-running request passed: %s", rec.Body.String())
		}
	}
	s.envPassword = "test-password"
	rec, _ := doJSON(t, s.Handler(), "POST", path, map[string]string{"action": "save"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("shortcut endpoint not authenticated")
	}
}
