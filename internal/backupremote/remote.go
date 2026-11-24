package backupremote

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Enabled   bool   `json:"enabled"`
	Kind      string `json:"kind"`
	Directory string `json:"directory"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	Keep      int    `json:"keep"`
}
type Result struct {
	ID    string    `json:"id"`
	Room  int       `json:"room"`
	Name  string    `json:"name"`
	State string    `json:"state"`
	Time  time.Time `json:"time"`
	Error string    `json:"error,omitempty"`
}
type Snapshot struct {
	Config
	PasswordSet bool     `json:"passwordSet"`
	Results     []Result `json:"results"`
	Namespace   string   `json:"namespace"`
}
type diskState struct {
	Config    Config `json:"config"`
	Namespace string `json:"namespace"`
}
type delivery struct {
	config Config
	result Result
	file   *os.File
}
type Engine struct {
	mu        sync.Mutex
	path      string
	root      string
	namespace string
	saved     diskState
	results   []Result
	queue     chan delivery
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closed    bool
	client    *http.Client
	onFailure func(Result)
}

var ownBackupName = regexp.MustCompile(`^Cluster_[1-9][0-9]*(?:_auto)?_[0-9]{8}_[0-9]{6}\.[0-9]{9}\.zip$`)
var namespaceName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func New(path, root string, onFailure func(Result)) (*Engine, error) {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{path: path, root: root, queue: make(chan delivery, 8), ctx: ctx, cancel: cancel, onFailure: onFailure, results: []Result{}, saved: diskState{Config: Config{Kind: "directory", Keep: 10}, Namespace: rand.Text()}}
	e.client = &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	b, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(b, &e.saved); err != nil {
			return nil, err
		}
		if !namespaceName.MatchString(e.saved.Namespace) {
			return nil, fmt.Errorf("异地备份目录标识损坏")
		}
		if err = e.validate(e.saved.Config); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	e.namespace = e.saved.Namespace
	return e, nil
}
func (e *Engine) validate(c Config) error {
	if c.Kind != "directory" && c.Kind != "webdav" {
		return fmt.Errorf("请选择目录或 WebDAV")
	}
	if c.Keep < 1 || c.Keep > 100 {
		return fmt.Errorf("保留份数必须为 1–100")
	}
	if c.Kind == "directory" && c.Directory != "" {
		if !filepath.IsAbs(c.Directory) {
			return fmt.Errorf("备份目录必须是绝对路径")
		}
		root, err := filepath.Abs(e.root)
		if err != nil {
			return err
		}
		dest, err := filepath.Abs(c.Directory)
		if err != nil {
			return err
		}
		pairs := [][2]string{{root, dest}, {dest, root}}
		actualRoot, rootErr := filepath.EvalSymlinks(root)
		actualDest, destErr := filepath.EvalSymlinks(dest)
		if rootErr == nil && destErr == nil {
			pairs = append(pairs, [2]string{actualRoot, actualDest}, [2]string{actualDest, actualRoot})
		}
		for _, pair := range pairs {
			rel, err := filepath.Rel(pair[0], pair[1])
			if err != nil {
				continue
			}
			if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("异地目录须与面板存档目录分开，不能互相包含")
			}
		}
	}
	if c.Kind == "webdav" && c.URL != "" {
		u, err := url.Parse(c.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("WebDAV 地址须为不含账号、查询参数或片段的 HTTP/HTTPS URL")
		}
	}
	if c.Enabled && (c.Kind == "directory" && c.Directory == "" || c.Kind == "webdav" && c.URL == "") {
		return fmt.Errorf("请填写备份目标")
	}
	return nil
}
func (e *Engine) Configure(c Config, password *string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	c.Directory, c.URL, c.Username = strings.TrimSpace(c.Directory), strings.TrimRight(strings.TrimSpace(c.URL), "/"), strings.TrimSpace(c.Username)
	c.Password = e.saved.Config.Password
	if password != nil {
		c.Password = *password
	}
	if err := e.validate(c); err != nil {
		return err
	}
	state := diskState{Config: c, Namespace: e.saved.Namespace}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(e.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(e.path), ".remote-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), e.path); err != nil {
		return err
	}
	e.saved = state
	return nil
}
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.saved.Config
	set := c.Password != ""
	c.Password = ""
	return Snapshot{Config: c, PasswordSet: set, Results: append([]Result{}, e.results...), Namespace: e.saved.Namespace}
}
func (e *Engine) Start() func() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-e.ctx.Done():
				for {
					select {
					case item := <-e.queue:
						_ = item.file.Close()
						e.finish(item.result, fmt.Errorf("面板关闭，同步已取消"))
					default:
						return
					}
				}
			case item := <-e.queue:
				e.setState(item.result.ID, "syncing", "")
				ctx, cancel := context.WithTimeout(e.ctx, 30*time.Minute)
				err := e.sync(ctx, item.config, item.result.Room, item.result.Name, item.file)
				cancel()
				_ = item.file.Close()
				e.finish(item.result, err)
			}
		}
	}()
	return func() { e.mu.Lock(); e.closed = true; e.cancel(); e.mu.Unlock(); e.wg.Wait() }
}
func (e *Engine) setState(id, state, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.results {
		if e.results[i].ID == id {
			e.results[i].State, e.results[i].Error = state, message
			return
		}
	}
}
func (e *Engine) finish(result Result, err error) {
	state, message := "synced", ""
	if err != nil {
		state = "failed"
		message = err.Error()
	}
	e.setState(result.ID, state, message)
	if err != nil && e.ctx.Err() == nil && e.onFailure != nil {
		result.State, result.Error = state, message
		e.onFailure(result)
	}
}
func (e *Engine) Enqueue(room int, name, path string, explicit bool) (*Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, fmt.Errorf("面板正在关闭")
	}
	c := e.saved.Config
	if !c.Enabled {
		if explicit {
			return nil, fmt.Errorf("请先启用异地备份")
		}
		return nil, nil
	}
	if room < 1 || !ownBackupName.MatchString(name) || !strings.HasPrefix(name, fmt.Sprintf("Cluster_%d_", room)) {
		return nil, fmt.Errorf("不是本面板生成的备份文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("本地备份不可读取")
	}
	result := Result{ID: rand.Text(), Room: room, Name: name, State: "queued", Time: time.Now()}
	for _, previous := range e.results {
		if previous.Room == room && previous.Name == name && (previous.State == "queued" || previous.State == "syncing") {
			f.Close()
			return nil, fmt.Errorf("此备份已在同步队列中")
		}
	}
	select {
	case e.queue <- delivery{c, result, f}:
		e.results = append([]Result{result}, e.results...)
		if len(e.results) > 40 {
			e.results = e.results[:40]
		}
		return &result, nil
	default:
		f.Close()
		return nil, fmt.Errorf("同步队列已满，请稍后重试")
	}
}
func (e *Engine) Test(ctx context.Context) error {
	e.mu.Lock()
	c, namespace := e.saved.Config, e.saved.Namespace
	e.mu.Unlock()
	if c.Kind == "directory" && c.Directory == "" || c.Kind == "webdav" && c.URL == "" {
		return fmt.Errorf("请先保存备份目标")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	name := ".connection-test-" + rand.Text()
	if c.Kind == "directory" {
		if err := os.MkdirAll(c.Directory, 0700); err != nil {
			return err
		}
		if err := e.validate(c); err != nil {
			return err
		}
		dir := filepath.Join(c.Directory, "dstlauncher-"+namespace)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("创建目标目录失败: %w", err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("DST backup write test"), 0600); err != nil {
			return err
		}
		return os.Remove(path)
	}
	dir, err := e.davDirectory(ctx, c, 0)
	if err != nil {
		return err
	}
	if err = e.davWrite(ctx, c, dir+name, strings.NewReader("DST backup write test"), 21); err != nil {
		return err
	}
	return e.davDelete(ctx, c, dir+name)
}
func (e *Engine) sync(ctx context.Context, c Config, room int, name string, file *os.File) error {
	if c.Kind == "directory" {
		return e.syncDirectory(ctx, c, room, name, file)
	}
	return e.syncDAV(ctx, c, room, name, file)
}
