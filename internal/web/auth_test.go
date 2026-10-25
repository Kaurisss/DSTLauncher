package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"dst-windows/internal/panelcfg"
	"dst-windows/internal/room"
	"dst-windows/internal/server"
	"dst-windows/internal/steamcmd"
)

func newAuthTestServer(t *testing.T, envPassword string) *Server {
	t.Helper()
	dir := t.TempDir()
	rooms, err := room.NewStore(filepath.Join(dir, "rooms.json"))
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(Options{
		Mgr:      server.NewManager(dir, dir, dir),
		Rooms:    rooms,
		Steam:    steamcmd.NewManager(dir, "auto"),
		StaticFS: fstest.MapFS{"index.html": {Data: []byte("ok")}},
		RootDir:  dir,
		ConfDir:  "DST",
		WorkDir:  dir,
		UgcDir:   dir,
		Version:  "test",
		Password: envPassword,
	})
}

func doJSON(t *testing.T, h http.Handler, method, path string, body interface{}, cookies []*http.Cookie) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestAuthLifecycleAndCookieInvalidation(t *testing.T) {
	srv := newAuthTestServer(t, "")
	h := srv.Handler()

	// 1. 默认未设密码，业务接口直接放行
	rec, body := doJSON(t, h, http.MethodGet, "/api/rooms", nil, nil)
	if rec.Code != http.StatusOK || body["code"] != float64(200) {
		t.Fatalf("未设密码时应放行: status=%d body=%v", rec.Code, body)
	}

	// 2. 设置新密码，返回有效会话 Cookie
	rec, body = doJSON(t, h, http.MethodPut, "/api/auth/password", map[string]string{
		"newPassword": "secret123",
	}, nil)
	if body["code"] != float64(200) {
		t.Fatalf("首次设置密码失败: %v", body)
	}
	cookies1 := rec.Result().Cookies()
	if len(cookies1) == 0 {
		t.Fatal("设置密码后应下发会话 Cookie")
	}
	if cookies1[0].SameSite != http.SameSiteStrictMode || !cookies1[0].HttpOnly {
		t.Fatalf("会话 Cookie 必须限制跨站发送并禁止脚本读取: %+v", cookies1[0])
	}

	// 3. 无 Cookie 访问受保护接口应返回 401，但 /healthz 仍为 200
	rec, body = doJSON(t, h, http.MethodGet, "/api/rooms", nil, nil)
	if rec.Code != http.StatusUnauthorized || body["code"] != float64(401) {
		t.Fatalf("未登录访问应返回 401: status=%d body=%v", rec.Code, body)
	}
	healthRec := httptest.NewRecorder()
	h.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthRec.Code != http.StatusOK {
		t.Fatalf("/healthz 不应被鉴权拦截: %d", healthRec.Code)
	}

	// 4. 携带 Cookie 访问放行
	rec, body = doJSON(t, h, http.MethodGet, "/api/rooms", nil, cookies1)
	if rec.Code != http.StatusOK || body["code"] != float64(200) {
		t.Fatalf("携带有效 Cookie 应放行: status=%d body=%v", rec.Code, body)
	}

	// 5. 修改密码后，旧 Cookie 立即失效，新 Cookie 有效
	rec, body = doJSON(t, h, http.MethodPut, "/api/auth/password", map[string]string{
		"currentPassword": "secret123",
		"newPassword":     "newpass456",
	}, cookies1)
	if body["code"] != float64(200) {
		t.Fatalf("修改密码失败: %v", body)
	}
	cookies2 := rec.Result().Cookies()

	rec, _ = doJSON(t, h, http.MethodGet, "/api/rooms", nil, cookies1)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("密码修改后旧 Cookie 必须失效: status=%d", rec.Code)
	}
	rec, _ = doJSON(t, h, http.MethodGet, "/api/rooms", nil, cookies2)
	if rec.Code != http.StatusOK {
		t.Fatalf("新 Cookie 应有效: status=%d", rec.Code)
	}

	// 6. 清空密码后恢复免登访问
	_, body = doJSON(t, h, http.MethodPut, "/api/auth/password", map[string]string{
		"currentPassword": "newpass456",
		"newPassword":     "",
	}, cookies2)
	if body["code"] != float64(200) {
		t.Fatalf("清空密码失败: %v", body)
	}
	rec, _ = doJSON(t, h, http.MethodGet, "/api/rooms", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("清空密码后应免登放行: status=%d", rec.Code)
	}
}

func TestAuthLoginLockout(t *testing.T) {
	srv := newAuthTestServer(t, "fixedpass")
	h := srv.Handler()

	for i := 0; i < maxLoginFailures; i++ {
		_, body := doJSON(t, h, http.MethodPost, "/api/auth/login", map[string]string{"password": "wrong"}, nil)
		if body["code"] == float64(200) {
			t.Fatal("错误密码不应登录成功")
		}
	}
	// 达到锁定阈值后，即使输入正确密码也会提示冷却
	_, body := doJSON(t, h, http.MethodPost, "/api/auth/login", map[string]string{"password": "fixedpass"}, nil)
	if body["code"] == float64(200) {
		t.Fatal("锁定期间不应允许登录")
	}
}

func TestAuthRejectsCrossOriginRequests(t *testing.T) {
	for _, password := range []string{"", "fixedpass"} {
		t.Run("password="+password, func(t *testing.T) {
			srv := newAuthTestServer(t, password)
			h := srv.Handler()
			rm, err := srv.rooms.Create(room.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			login, body := doJSON(t, h, http.MethodPost, "/api/auth/login", map[string]string{"password": password}, nil)
			if body["code"] != float64(200) {
				t.Fatalf("登录失败: %v", body)
			}
			for i, tc := range []struct {
				name    string
				headers map[string]string
				allowed bool
			}{
				{"cross-site navigation", map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate"}, false},
				{"same-site sibling", map[string]string{"Sec-Fetch-Site": "same-site"}, false},
				{"foreign origin", map[string]string{"Origin": "https://attacker.invalid"}, false},
				{"foreign referer", map[string]string{"Referer": "https://attacker.invalid/page"}, false},
				{"opaque origin", map[string]string{"Origin": "null"}, false},
				{"same origin", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://example.com"}, true},
				{"reverse proxy", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://panel.example"}, true},
				{"legacy same origin", map[string]string{"Referer": "http://example.com/page"}, true},
				{"direct client", nil, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					id := fmt.Sprintf("review-%d", i)
					req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/mods/toggle?room=%d&id=%s&enabled=true", rm.ID, id), nil)
					for _, c := range login.Result().Cookies() {
						req.AddCookie(c)
					}
					for k, v := range tc.headers {
						req.Header.Set(k, v)
					}
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, req)
					wantStatus := http.StatusForbidden
					if tc.allowed {
						wantStatus = http.StatusOK
					}
					if rec.Code != wantStatus {
						t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
					}
					saved, err := srv.modsFor(rm, rm.MasterShard()).LoadOverrides()
					if err != nil {
						t.Fatal(err)
					}
					if saved.Enabled[id] != tc.allowed {
						t.Fatalf("模组配置被错误修改: %+v", saved.Enabled)
					}
				})
			}
			for _, path := range []string{"/api/auth/login", "/api/auth/logout", "/api/auth/password"} {
				req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"password":"fixedpass"}`))
				req.Header.Set("Sec-Fetch-Site", "cross-site")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
					t.Fatalf("跨站鉴权请求未被拒绝: %s status=%d", path, rec.Code)
				}
			}
		})
	}
}

func TestAuthConfigFailureKeepsAPIsProtected(t *testing.T) {
	for _, failure := range []string{"corrupt", "null", "missing", "unreadable"} {
		t.Run(failure, func(t *testing.T) {
			srv := newAuthTestServer(t, "")
			h := srv.Handler()
			login, body := doJSON(t, h, http.MethodPut, "/api/auth/password", map[string]string{"newPassword": "secret123"}, nil)
			if body["code"] != float64(200) {
				t.Fatalf("设置密码失败: %v", body)
			}
			path := panelcfg.Path(srv.workDir)
			switch failure {
			case "corrupt", "null":
				content := `{"passwordHash":`
				if failure == "null" {
					content = "null"
				}
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing", "unreadable":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if failure == "unreadable" {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, cookies := range [][]*http.Cookie{nil, login.Result().Cookies()} {
				rec, _ := doJSON(t, h, http.MethodGet, "/api/rooms", nil, cookies)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("配置异常时应拒绝业务请求: status=%d body=%s", rec.Code, rec.Body.String())
				}
			}
			if failure != "missing" {
				restarted := NewServer(Options{WorkDir: srv.workDir})
				rec, _ := doJSON(t, restarted.Handler(), http.MethodGet, "/api/rooms", nil, nil)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("启动时配置异常也应拒绝访问: status=%d", rec.Code)
				}
			}
			login, body = doJSON(t, h, http.MethodPost, "/api/auth/login", map[string]string{"password": "secret123"}, nil)
			if body["code"] == float64(200) || len(login.Result().Cookies()) != 0 {
				t.Fatalf("配置异常时不应签发会话: %v", body)
			}
		})
	}
}

func TestAuthRejectsCookieForgedFromStoredHash(t *testing.T) {
	srv := newAuthTestServer(t, "")
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	if err := panelcfg.Save(srv.workDir, panelcfg.Config{PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	payload := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 16)
	key := sha256.Sum256([]byte("dst-panel-session-v1|" + hash))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(payload))
	forged := &http.Cookie{Name: sessionCookieName, Value: payload + "." + hex.EncodeToString(mac.Sum(nil))}
	rec, _ := doJSON(t, srv.Handler(), http.MethodGet, "/api/rooms", nil, []*http.Cookie{forged})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("持有摘要不应能伪造会话: status=%d", rec.Code)
	}
}

func TestAuthConfigWriteDoesNotOverwriteUnreadableConfig(t *testing.T) {
	srv := newAuthTestServer(t, "fixedpass")
	content := []byte(`{"passwordHash":`)
	if err := os.WriteFile(panelcfg.Path(srv.workDir), content, 0600); err != nil {
		t.Fatal(err)
	}
	login, _ := doJSON(t, srv.Handler(), http.MethodPost, "/api/auth/login", map[string]string{"password": "fixedpass"}, nil)
	_, body := doJSON(t, srv.Handler(), http.MethodPut, "/api/workshop/key", map[string]string{"steamApiKey": "fixture"}, login.Result().Cookies())
	if body["code"] == float64(200) {
		t.Fatalf("读取失败时不应保存配置: %v", body)
	}
	got, err := os.ReadFile(panelcfg.Path(srv.workDir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("异常配置被覆盖: %s", got)
	}
}
