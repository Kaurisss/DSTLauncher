package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dst-windows/internal/server"
)

func fakeSteamInstall(t *testing.T, srv *Server) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Steam 库", "steamapps", "common", "DST")
	executable := filepath.Join(root, "bin64", filepath.Base(srv.steam.Executable()))
	if err := os.MkdirAll(filepath.Dir(executable), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestServerSwitchAlsoSwitchesWorkshopDirectory(t *testing.T) {
	srv := newAuthTestServer(t, "")
	root := fakeSteamInstall(t, srv)
	_, body := doJSON(t, srv.Handler(), http.MethodPut, "/api/server", map[string]string{
		"serverDir": root, "arch": "64",
	}, nil)
	if body["code"] != float64(200) {
		t.Fatalf("切换安装失败: %v", body)
	}
	want := filepath.Join(filepath.Dir(filepath.Dir(root)), "workshop")
	data := body["data"].(map[string]interface{})
	if srv.mgr.UgcDir() != want || data["ugcDir"] != want {
		t.Fatalf("读取/下载/设置展示应指向同一工坊: %s, %v", srv.mgr.UgcDir(), data)
	}
}

func TestServerSwitchRejectedWhileModsUpdating(t *testing.T) {
	srv := newAuthTestServer(t, "")
	root := fakeSteamInstall(t, srv)
	if _, err := srv.steam.Configure(root, "64", "test"); err != nil {
		t.Fatal(err)
	}
	srv.mgr.SetExeResolver(func() (string, string) { return srv.steam.Executable(), srv.steam.ServerDir() })
	srv.mgr.Register(&server.Cluster{ID: 1, ConfDir: "DST", Shards: []string{"Master"}})
	started, release := make(chan struct{}), make(chan struct{})
	srv.mgr.SetModsSetupSwap(func(string) (func(), error) {
		close(started)
		<-release
		return nil, fmt.Errorf("模拟更新中止")
	})
	if err := srv.mgr.UpdateMods("Cluster_1", "Master", "workshop-123"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(release)
		deadline := time.Now().Add(time.Second)
		for srv.mgr.ModsUpdating() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("更新会话未启动")
	}
	h := srv.Handler()
	_, body := doJSON(t, h, http.MethodGet, "/api/server", nil, nil)
	if body["data"].(map[string]interface{})["locked"] != true {
		t.Fatalf("下载期间设置应锁定: %v", body)
	}
	before := srv.mgr.UgcDir()
	_, body = doJSON(t, h, http.MethodPut, "/api/server", map[string]string{"serverDir": root, "arch": "64"}, nil)
	if body["code"] == float64(200) || srv.mgr.UgcDir() != before {
		t.Fatalf("下载期间不应切换路径: %v", body)
	}
}
