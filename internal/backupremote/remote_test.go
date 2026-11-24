package backupremote

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func sourceBackup(t *testing.T, index int) (string, string) {
	t.Helper()
	name := fmt.Sprintf("Cluster_1_20261107_120000.%09d.zip", index)
	file := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(file, []byte(fmt.Sprintf("backup-%d", index)), 0600); err != nil {
		t.Fatal(err)
	}
	return name, file
}
func waitResult(t *testing.T, e *Engine, id string) Result {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range e.Snapshot().Results {
			if r.ID == id && (r.State == "synced" || r.State == "failed") {
				return r
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("同步未完成")
	return Result{}
}
func TestDirectorySyncRetentionAndPasswordPersistence(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "backup-remote.json")
	e, err := New(file, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	password := "DAV-SECRET"
	if err = e.Configure(Config{Enabled: true, Kind: "directory", Directory: dest, Keep: 2}, &password); err != nil {
		t.Fatal(err)
	}
	if err = e.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	stop := e.Start()
	defer stop()
	for i := 0; i < 3; i++ {
		name, path := sourceBackup(t, i)
		r, err := e.Enqueue(1, name, path, true)
		if err != nil {
			t.Fatal(err)
		}
		if result := waitResult(t, e, r.ID); result.State != "synced" {
			t.Fatal(result)
		}
	}
	dir := filepath.Join(dest, "dstlauncher-"+e.Snapshot().Namespace, "Cluster_1")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	if !strings.Contains(entries[0].Name(), "000000001") {
		t.Fatal(entries)
	}
	// Explicitly retrying an older local backup must not immediately prune that copy.
	name, path := sourceBackup(t, 0)
	result, err := e.Enqueue(1, name, path, true)
	if err != nil {
		t.Fatal(err)
	}
	if done := waitResult(t, e, result.ID); done.State != "synced" {
		t.Fatal(done)
	}
	if _, err = os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatal("轮转删除了刚同步的副本", err)
	}
	if e.Snapshot().Password != "" || !e.Snapshot().PasswordSet {
		t.Fatal("密码被回传")
	}
	reloaded, err := New(file, root, nil)
	if err != nil || reloaded.saved.Config.Password != password || reloaded.saved.Namespace != e.saved.Namespace {
		t.Fatal(err)
	}
	if err = reloaded.Configure(reloaded.saved.Config, nil); err != nil || reloaded.saved.Config.Password != password {
		t.Fatal("密码未保留", err)
	}
	for _, bad := range []Config{{Enabled: true, Kind: "directory", Directory: filepath.Join(root, "inside"), Keep: 2}, {Enabled: true, Kind: "directory", Directory: filepath.Dir(root), Keep: 2}, {Enabled: true, Kind: "directory", Directory: "relative", Keep: 2}, {Enabled: true, Kind: "webdav", URL: "https://user:secret@example.com", Keep: 2}} {
		if err = e.Configure(bad, nil); err == nil {
			t.Fatal("接受了不合法目标", bad.Kind)
		}
	}
}
func TestWebDAVUploadsThenMovesAndPrunesOnlyOwnNames(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	methods := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "user" || password != "secret" {
			w.WriteHeader(401)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		methods = append(methods, r.Method)
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(201)
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			objects[r.URL.Path] = b
			w.WriteHeader(201)
		case "MOVE":
			dest, _ := url.Parse(r.Header.Get("Destination"))
			objects[dest.Path] = objects[r.URL.Path]
			delete(objects, r.URL.Path)
			w.WriteHeader(201)
		case "DELETE":
			delete(objects, r.URL.Path)
			w.WriteHeader(204)
		case "PROPFIND":
			if r.Header.Get("Depth") != "1" {
				t.Error("缺少 Depth")
			}
			w.WriteHeader(207)
			_, _ = io.WriteString(w, `<d:multistatus xmlns:d="DAV:">`)
			for path := range objects {
				if strings.HasPrefix(path, r.URL.Path) {
					_, _ = fmt.Fprintf(w, `<d:response><d:href>%s</d:href><d:propstat><d:prop><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, strings.TrimPrefix(path, r.URL.Path))
				}
			}
			_, _ = io.WriteString(w, `<d:response><d:href>/outside/Cluster_1_20200101_120000.000000000.zip</d:href><d:propstat><d:prop><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
		default:
			w.WriteHeader(405)
		}
	}))
	defer srv.Close()
	root := t.TempDir()
	e, err := New(filepath.Join(root, "config.json"), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	secret := "secret"
	if err = e.Configure(Config{Enabled: true, Kind: "webdav", URL: srv.URL + "/dav", Username: "user", Keep: 1}, &secret); err != nil {
		t.Fatal(err)
	}
	if err = e.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	stop := e.Start()
	defer stop()
	for i := 0; i < 2; i++ {
		name, path := sourceBackup(t, i)
		r, err := e.Enqueue(1, name, path, true)
		if err != nil {
			t.Fatal(err)
		}
		if result := waitResult(t, e, r.ID); result.State != "synced" {
			t.Fatal(result)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(objects) != 1 {
		t.Fatal(objects)
	}
	for path, b := range objects {
		if !strings.HasSuffix(path, "000000001.zip") || string(b) != "backup-1" {
			t.Fatal(path, string(b))
		}
	}
	if !strings.Contains(strings.Join(methods, ","), "PUT,MOVE,PROPFIND") {
		t.Fatal(methods)
	}
	var payload davMultiStatus
	if err = xml.Unmarshal([]byte(`<multistatus xmlns="DAV:"><response><href>/x</href></response></multistatus>`), &payload); err != nil || len(payload.Responses) != 1 {
		t.Fatal(err)
	}
}
func TestFailedRemoteSyncKeepsLocalBackupAndNotifies(t *testing.T) {
	received := make(chan Result, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer srv.Close()
	root := t.TempDir()
	e, _ := New(filepath.Join(root, "config.json"), root, func(r Result) { received <- r })
	if err := e.Configure(Config{Enabled: true, Kind: "webdav", URL: srv.URL, Keep: 2}, nil); err != nil {
		t.Fatal(err)
	}
	stop := e.Start()
	defer stop()
	name, file := sourceBackup(t, 0)
	r, err := e.Enqueue(1, name, file, true)
	if err != nil {
		t.Fatal(err)
	}
	result := waitResult(t, e, r.ID)
	if result.State != "failed" || !strings.Contains(result.Error, "403") {
		t.Fatal(result)
	}
	select {
	case notice := <-received:
		if notice.Room != 1 {
			t.Fatal(notice)
		}
	case <-time.After(time.Second):
		t.Fatal("未通知失败")
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatal("本地备份被移除")
	}
}
func TestShutdownCancelsWebDAVAndClosesQueuedFiles(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer srv.Close()
	root := t.TempDir()
	e, _ := New(filepath.Join(root, "config.json"), root, nil)
	if err := e.Configure(Config{Enabled: true, Kind: "webdav", URL: srv.URL, Keep: 1}, nil); err != nil {
		t.Fatal(err)
	}
	stop := e.Start()
	name, file := sourceBackup(t, 0)
	if _, err := e.Enqueue(1, name, file, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("未开始请求")
	}
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("关闭未取消网络请求")
	}
	if _, err := e.Enqueue(1, name, file, true); err == nil {
		t.Fatal("关闭后接受任务")
	}
}
