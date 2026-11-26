package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"dst-windows/internal/panelcfg"
)

const (
	sessionCookieName      = "dst_session"
	sessionMaxAge          = 30 * 24 * 3600 // 30 天
	legacyHashIterations   = 10000
	pbkdf2Iterations       = 600000
	pbkdf2KeySize          = 32
	maxLoginFailures       = 5
	maxGlobalLoginFailures = 50
	loginLockDuration      = 30 * time.Second
	maxLoginLockDuration   = 15 * time.Minute
	maxLoginStates         = 1024
)

type loginState struct {
	failCount int
	lockUntil time.Time
	delay     time.Duration
	lastSeen  time.Time
}

func generateSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("生成会话密钥失败: %w", err))
	}
	return b
}

// HashPassword 生成带随机盐的 PBKDF2-SHA256 密码摘要。
func HashPassword(password string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", err
	}
	hash := derivePBKDF2(saltBytes, password, pbkdf2Iterations)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
		pbkdf2Iterations,
		hex.EncodeToString(saltBytes),
		hex.EncodeToString(hash),
	), nil
}

func derivePasswordHash(salt, password string) string {
	h := sha256.Sum256([]byte(salt + ":" + password))
	for i := 0; i < legacyHashIterations; i++ {
		h = sha256.Sum256(h[:])
	}
	return hex.EncodeToString(h[:])
}

func derivePBKDF2(salt []byte, password string, iterations int) []byte {
	mac := hmac.New(sha256.New, []byte(password))
	var block [4]byte
	binary.BigEndian.PutUint32(block[:], 1)
	mac.Write(salt)
	mac.Write(block[:])
	previous := mac.Sum(nil)
	result := append([]byte(nil), previous...)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(previous)
		previous = mac.Sum(nil)
		for j := range result {
			result[j] ^= previous[j]
		}
	}
	return result
}

// VerifyPassword 常量时间校验密码与存储的摘要，兼容旧版 salt:hash 格式。
func VerifyPassword(stored, password string) bool {
	if strings.HasPrefix(stored, "pbkdf2-sha256$") {
		parts := strings.Split(stored, "$")
		if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
			return false
		}
		iterations, err := strconv.Atoi(parts[1])
		if err != nil || iterations < 100000 || iterations > 10000000 {
			return false
		}
		salt, err := hex.DecodeString(parts[2])
		if err != nil || len(salt) < 16 {
			return false
		}
		want, err := hex.DecodeString(parts[3])
		if err != nil || len(want) != pbkdf2KeySize {
			return false
		}
		return subtle.ConstantTimeCompare(derivePBKDF2(salt, password, iterations), want) == 1
	}
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	got := derivePasswordHash(parts[0], password)
	return subtle.ConstantTimeCompare([]byte(got), []byte(parts[1])) == 1
}

func needsPasswordUpgrade(stored string) bool {
	return stored != "" && !strings.HasPrefix(stored, "pbkdf2-sha256$")
}

func loginClientKey(r *http.Request) string {
	remote := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remote = host
	}
	if remoteIP := net.ParseIP(remote); remoteIP != nil && remoteIP.IsLoopback() {
		for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
			for _, value := range strings.Split(r.Header.Get(header), ",") {
				if ip := net.ParseIP(strings.TrimSpace(value)); ip != nil {
					return ip.String()
				}
			}
		}
	}
	return remote
}

func (s *Server) pruneLoginStates(now time.Time) {
	if len(s.loginStates) < maxLoginStates {
		return
	}
	for key, state := range s.loginStates {
		if state.lockUntil.Before(now) && now.Sub(state.lastSeen) > maxLoginLockDuration {
			delete(s.loginStates, key)
		}
	}
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
	retryAfter := s.loginRetryAfter(loginClientKey(r))
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

func retryAfter(state loginState, now time.Time) int {
	return max(0, int((time.Until(state.lockUntil)+time.Second-1)/time.Second))
}

func (s *Server) loginRetryAfter(clientKey string) int {
	now := time.Now()
	wait := retryAfter(s.globalLogin, now)
	if client := s.loginStates[clientKey]; retryAfter(client, now) > wait {
		wait = retryAfter(client, now)
	}
	return wait
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
	clientKey := loginClientKey(r)
	now := time.Now()
	if wait := s.loginRetryAfter(clientKey); wait > 0 {
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
		client := s.loginStates[clientKey]
		client.lastSeen = now
		if client.failCount < maxLoginFailures {
			client.failCount++
		}
		if client.failCount >= maxLoginFailures {
			client.delay = min(max(client.delay*2, loginLockDuration), maxLoginLockDuration)
			client.lockUntil = now.Add(client.delay)
		}
		s.loginStates[clientKey] = client
		s.globalLogin.lastSeen = now
		if s.globalLogin.failCount < maxGlobalLoginFailures {
			s.globalLogin.failCount++
		}
		if s.globalLogin.failCount >= maxGlobalLoginFailures {
			s.globalLogin.delay = min(max(s.globalLogin.delay*2, loginLockDuration), maxLoginLockDuration)
			s.globalLogin.lockUntil = now.Add(s.globalLogin.delay)
		}
		if wait := s.loginRetryAfter(clientKey); wait > 0 {
			s.pruneLoginStates(now)
			rejectLoginCooldown(w, wait)
			return
		}
		s.pruneLoginStates(now)
		respond(w, 200, nil, fmt.Errorf("用户名或密码错误"))
		return
	}

	if factor != "" && s.envPassword == "" && s.envUsername == "" && needsPasswordUpgrade(cfg.PasswordHash) {
		if upgraded, hashErr := HashPassword(body.Password); hashErr == nil {
			cfg.PasswordHash = upgraded
			if saveErr := panelcfg.Save(s.workDir, cfg); saveErr == nil {
				factor = "cfg:" + username + "|" + upgraded
				s.authMu.Lock()
				s.authConfigSeen = true
				s.authMu.Unlock()
			}
		}
	}
	delete(s.loginStates, clientKey)
	s.globalLogin = loginState{}

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
