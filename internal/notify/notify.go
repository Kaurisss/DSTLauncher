package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var Events = []string{"crash_limit", "maintenance_failed", "backup_failed", "update_failed", "start_failed", "started", "stopped"}

type Config struct {
	Enabled bool     `json:"enabled"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret,omitempty"`
	Events  []string `json:"events"`
	Rooms   []int    `json:"rooms"`
}

func (c Config) Validate() error {
	if c.Enabled || c.URL != "" {
		u, err := url.Parse(c.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("通知地址须为不含账号或片段的 HTTP/HTTPS URL")
		}
	}
	if c.Enabled && len(c.Events) == 0 {
		return fmt.Errorf("请至少选择一种通知事件")
	}
	for _, event := range c.Events {
		if !slices.Contains(Events, event) {
			return fmt.Errorf("未知通知事件: %s", event)
		}
	}
	for _, id := range c.Rooms {
		if id < 1 {
			return fmt.Errorf("通知房间 ID 不合法")
		}
	}
	return nil
}

type Event struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Event    string    `json:"event"`
	RoomID   int       `json:"roomId,omitempty"`
	RoomName string    `json:"roomName,omitempty"`
	Shard    string    `json:"shard,omitempty"`
	Message  string    `json:"message"`
	Version  string    `json:"version"`
}

type Result struct {
	Time   time.Time `json:"time"`
	Event  string    `json:"event"`
	RoomID int       `json:"roomId,omitempty"`
	Status int       `json:"status"`
	Error  string    `json:"error,omitempty"`
}

type Snapshot struct {
	Config
	SecretSet bool     `json:"secretSet"`
	Results   []Result `json:"results"`
}

type delivery struct {
	config Config
	event  Event
}

type Engine struct {
	mu      sync.Mutex
	path    string
	config  Config
	results []Result
	queue   chan delivery
	client  *http.Client
}

func New(path string) (*Engine, error) {
	e := &Engine{path: path, config: Config{Events: []string{"crash_limit", "maintenance_failed", "backup_failed", "update_failed"}, Rooms: []int{}}, queue: make(chan delivery, 64)}
	e.client = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	b, err := os.ReadFile(path)
	if err == nil {
		var saved *Config
		if err = json.Unmarshal(b, &saved); err != nil {
			return nil, fmt.Errorf("读取通知配置失败: %w", err)
		}
		if saved == nil {
			return nil, fmt.Errorf("通知配置必须是 JSON 对象")
		}
		e.config = *saved
		if err := e.config.Validate(); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return e, nil
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.config
	c.Events = append([]string{}, c.Events...)
	c.Rooms = append([]int{}, c.Rooms...)
	c.Secret = ""
	return Snapshot{Config: c, SecretSet: e.config.Secret != "", Results: append([]Result{}, e.results...)}
}

// Configure preserves the secret when it is omitted; an explicit empty string clears it.
func (e *Engine) Configure(c Config, secret *string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	c.URL = strings.TrimSpace(c.URL)
	c.Secret = e.config.Secret
	if secret != nil {
		c.Secret = *secret
	}
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(e.path), ".notifications-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), e.path); err != nil {
		return err
	}
	e.config = c
	return nil
}

func (e *Engine) record(result Result) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.results = append([]Result{result}, e.results...)
	if len(e.results) > 20 {
		e.results = e.results[:20]
	}
}

func (e *Engine) Publish(event Event) {
	e.mu.Lock()
	c := e.config
	e.mu.Unlock()
	if !c.Enabled || !slices.Contains(c.Events, event.Event) || (len(c.Rooms) > 0 && event.RoomID > 0 && !slices.Contains(c.Rooms, event.RoomID)) {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	select {
	case e.queue <- delivery{c, event}:
	default:
		e.record(Result{Time: time.Now(), Event: event.Event, RoomID: event.RoomID, Error: "通知队列已满，本次未发送"})
	}
}

func (e *Engine) Serve(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-e.queue:
			e.send(ctx, item.config, item.event)
		}
	}
}

func (e *Engine) Test(ctx context.Context, version string) Result {
	e.mu.Lock()
	c := e.config
	e.mu.Unlock()
	if c.URL == "" {
		return Result{Time: time.Now(), Event: "test", Error: "请先保存通知地址"}
	}
	return e.send(ctx, c, Event{Event: "test", Message: "DST 面板测试通知", Version: version})
}

func (e *Engine) send(ctx context.Context, c Config, event Event) (result Result) {
	result = Result{Time: time.Now(), Event: event.Event, RoomID: event.RoomID}
	defer func() { e.record(result) }()
	event.ID = rand.Text()
	if event.Time.IsZero() {
		event.Time = result.Time
	}
	body, err := json.Marshal(event)
	if err != nil {
		result.Error = err.Error()
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		result.Error = "无法创建通知请求"
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DSTLauncher/"+event.Version)
	timestamp := strconv.FormatInt(event.Time.Unix(), 10)
	req.Header.Set("X-DST-Timestamp", timestamp)
	if c.Secret != "" {
		mac := hmac.New(sha256.New, []byte(c.Secret))
		mac.Write([]byte(timestamp + "."))
		mac.Write(body)
		req.Header.Set("X-DST-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	res, err := e.client.Do(req)
	if err != nil {
		// net/http errors can contain the webhook URL and its token-bearing query.
		result.Error = "通知连接失败或超时"
		return
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	result.Status = res.StatusCode
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		result.Error = fmt.Sprintf("接收端返回 HTTP %d", res.StatusCode)
	}
	return
}
