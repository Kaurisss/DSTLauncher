package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"dst-windows/internal/panelcfg"
)

const (
	sessionCookieName    = "dst_session"
	sessionMaxAge        = 30 * 24 * 3600 // 30 天
	hashIterations       = 10000
	maxLoginFailures     = 5
	loginLockDuration    = 30 * time.Second
	maxLoginLockDuration = 15 * time.Minute
)

func generateSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("生成会话密钥失败: %w", err))
	}
	return b
}

// HashPassword 生成带随机盐的迭代 SHA-256 密码摘要（格式：saltHex:hashHex）。
func HashPassword(password string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", err
	}
	salt := hex.EncodeToString(saltBytes)
	return salt + ":" + derivePasswordHash(salt, password), nil
}

func derivePasswordHash(salt, password string) string {
	h := sha256.Sum256([]byte(salt + ":" + password))
	for i := 0; i < hashIterations; i++ {
		h = sha256.Sum256(h[:])
	}
	return hex.EncodeToString(h[:])
}

// VerifyPassword 常量时间校验密码与存储的 salt:hash 摘要。
func VerifyPassword(stored, password string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	got := derivePasswordHash(parts[0], password)
	return subtle.ConstantTimeCompare([]byte(got), []byte(parts[1])) == 1
}

func configuredUsername(cfg panelcfg.Config) string {
	if cfg.Username != "" {
		return cfg.Username
	}
	return panelcfg.DefaultUsername
}

// activeAuth 返回当前生效的登录配置和会话签名因子。
// 用户名或密码变化时，旧 Cookie 会自动失效。
func (s *Server) activeAuth() (panelcfg.Config, string, string, bool, error) {
	cfg := panelcfg.Config{}
	username := strings.TrimSpace(s.envUsername)
	envManaged := s.envUsername != "" || s.envPassword != ""
	if username == "" {
		username = panelcfg.DefaultUsername
	}
	if s.envPassword != "" {
		return cfg, username, "env:" + username + "|" + s.envPassword, envManaged, nil
	}
	if s.workDir == "" {
		return cfg, username, "", envManaged, nil
	}
	var err error
	cfg, err = s.loadPanelConfig()
	if err != nil {
		return panelcfg.Config{}, "", "", envManaged, err
	}
	if s.envUsername == "" {
		username = configuredUsername(cfg)
	}
	factor := ""
	if cfg.PasswordHash != "" {
		factor = "cfg:" + username + "|" + cfg.PasswordHash
	}
	return cfg, username, factor, envManaged, nil
}

// activePasswordFactor 返回当前生效的密码凭据指纹，用于绑定会话签名。
func (s *Server) activePasswordFactor() (string, error) {
	_, _, factor, _, err := s.activeAuth()
	return factor, err
}

func (s *Server) loadPanelConfig() (panelcfg.Config, error) {
	cfg, err := panelcfg.Read(s.workDir)
	s.authMu.Lock()
	defer s.authMu.Unlock()
	// 仅首次未配置允许免登；已经读取过的配置消失时也必须拒绝访问。
	if os.IsNotExist(err) && !s.authConfigSeen {
		return panelcfg.Config{}, nil
	}
	if err != nil {
		return panelcfg.Config{}, fmt.Errorf("读取面板配置失败: %w", err)
	}
	s.authConfigSeen = true
	return cfg, nil
}

func (s *Server) authEnabled() bool {
	_, _, factor, _, err := s.activeAuth()
	return err != nil || factor != ""
}

func (s *Server) signSession(exp int64, factor string) string {
	payload := strconv.FormatInt(exp, 16)
	mac := hmac.New(sha256.New, s.authSecret)
	mac.Write([]byte(payload + "|" + factor))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	factor, err := s.activePasswordFactor()
	if err != nil {
		return false
	}
	if factor == "" {
		return true
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 16, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	want := s.signSession(exp, factor)
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(want)) == 1
}

func (s *Server) setSessionCookie(w http.ResponseWriter, factor string) {
	exp := time.Now().Unix() + sessionMaxAge
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.signSession(exp, factor),
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// withAuth 拦截受保护的 /api/* 请求；未开启密码或公开端点直接放行。
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") && isCrossOriginRequest(r) {
			rejectAPI(w, http.StatusForbidden, "不允许跨站访问面板接口")
			return
		}
		if strings.HasPrefix(p, "/api/") &&
			p != "/api/auth/status" &&
			p != "/api/auth/login" &&
			p != "/api/auth/logout" {
			if !s.isAuthenticated(r) {
				rejectAPI(w, http.StatusUnauthorized, "请先登录面板")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isCrossOriginRequest(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return false
	case "":
	default:
		return true
	}
	source := r.Header.Get("Origin")
	if source == "" {
		source = r.Header.Get("Referer")
	}
	if source == "" {
		return false
	}
	u, err := url.Parse(source)
	return err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, r.Host)
}

func rejectAPI(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code": status, "message": message, "data": nil,
	})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	s.loginMu.Lock()
	retryAfter := s.loginRetryAfter()
	s.loginMu.Unlock()
	_, username, _, envManaged, err := s.activeAuth()
	respond(w, 200, map[string]interface{}{
		"enabled":       s.authEnabled(),
		"authenticated": s.isAuthenticated(r),
		"username":      username,
		"envManaged":    envManaged,
		"retryAfter":    retryAfter,
	}, err)
}

func (s *Server) loginRetryAfter() int {
	return max(0, int((time.Until(s.lockUntil)+time.Second-1)/time.Second))
}

func rejectLoginCooldown(w http.ResponseWriter, wait int) {
	w.Header().Set("Retry-After", strconv.Itoa(wait))
	rejectAPI(w, http.StatusTooManyRequests, fmt.Sprintf("登录失败次数过多，请 %d 秒后再试", wait))
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, 200, nil, fmt.Errorf("仅支持 POST"))
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body, 4096); err != nil {
		respond(w, 200, nil, fmt.Errorf("请求格式错误"))
		return
	}

	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if wait := s.loginRetryAfter(); wait > 0 {
		rejectLoginCooldown(w, wait)
		return
	}

	cfg, username, factor, envManaged, err := s.activeAuth()
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	valid := true
	if factor != "" {
		valid = subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Username)), []byte(username)) == 1
		if envManaged && s.envPassword != "" {
			valid = valid && subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.envPassword)) == 1
		} else {
			valid = valid && VerifyPassword(cfg.PasswordHash, body.Password)
		}
	}
	if !valid {
		if s.failCount < maxLoginFailures {
			s.failCount++
		}
		if s.failCount >= maxLoginFailures {
			s.loginDelay = min(max(s.loginDelay*2, loginLockDuration), maxLoginLockDuration)
			s.lockUntil = time.Now().Add(s.loginDelay)
			rejectLoginCooldown(w, s.loginRetryAfter())
			return
		}
		respond(w, 200, nil, fmt.Errorf("用户名或密码错误"))
		return
	}

	s.failCount = 0
	s.lockUntil = time.Time{}
	s.loginDelay = 0

	if factor != "" {
		s.setSessionCookie(w, factor)
	}
	respond(w, 200, map[string]interface{}{
		"enabled":       s.authEnabled(),
		"authenticated": true,
		"username":      username,
		"envManaged":    envManaged,
	}, nil)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, 200, nil, fmt.Errorf("仅支持 POST"))
		return
	}
	clearSessionCookie(w)
	_, username, _, envManaged, err := s.activeAuth()
	respond(w, 200, map[string]interface{}{
		"enabled":       s.authEnabled(),
		"authenticated": !s.authEnabled(),
		"username":      username,
		"envManaged":    envManaged,
	}, err)
}

func (s *Server) handleAuthPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respond(w, 200, nil, fmt.Errorf("仅支持 PUT"))
		return
	}
	if s.envUsername != "" || s.envPassword != "" {
		respond(w, 200, nil, fmt.Errorf("用户名或密码由启动参数或环境变量指定，无法在面板内修改"))
		return
	}
	if s.workDir == "" {
		respond(w, 200, nil, fmt.Errorf("未配置工作目录"))
		return
	}

	var body struct {
		Username        *string `json:"username"`
		CurrentPassword string  `json:"currentPassword"`
		NewPassword     string  `json:"newPassword"`
	}
	if err := decodeJSON(w, r, &body, 4096); err != nil {
		respond(w, 200, nil, fmt.Errorf("请求格式错误"))
		return
	}

	cfg, err := s.loadPanelConfig()
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	if cfg.PasswordHash != "" && !VerifyPassword(cfg.PasswordHash, body.CurrentPassword) {
		respond(w, 200, nil, fmt.Errorf("当前密码不正确"))
		return
	}
	username := configuredUsername(cfg)
	if body.Username != nil {
		username = strings.TrimSpace(*body.Username)
		if username == "" {
			respond(w, 200, nil, fmt.Errorf("用户名不能为空"))
			return
		}
		if len([]rune(username)) > 64 {
			respond(w, 200, nil, fmt.Errorf("用户名不能超过 64 个字符"))
			return
		}
	}
	cfg.Username = username

	newPwd := strings.TrimSpace(body.NewPassword)
	if newPwd == "" {
		cfg.PasswordHash = ""
		if err := panelcfg.Save(s.workDir, cfg); err != nil {
			respond(w, 200, nil, fmt.Errorf("保存密码配置失败: %w", err))
			return
		}
		clearSessionCookie(w)
		respond(w, 200, map[string]interface{}{
			"enabled":       false,
			"authenticated": true,
			"username":      username,
			"envManaged":    false,
		}, nil)
		return
	}

	if len([]rune(newPwd)) < 4 {
		respond(w, 200, nil, fmt.Errorf("新密码至少需要 4 个字符"))
		return
	}
	hash, err := HashPassword(newPwd)
	if err != nil {
		respond(w, 200, nil, fmt.Errorf("生成密码摘要失败: %w", err))
		return
	}
	cfg.PasswordHash = hash
	if err := panelcfg.Save(s.workDir, cfg); err != nil {
		respond(w, 200, nil, fmt.Errorf("保存密码配置失败: %w", err))
		return
	}
	s.authMu.Lock()
	s.authConfigSeen = true
	s.authMu.Unlock()
	s.setSessionCookie(w, "cfg:"+username+"|"+hash)
	respond(w, 200, map[string]interface{}{
		"enabled":       true,
		"authenticated": true,
		"username":      username,
		"envManaged":    false,
	}, nil)
}
