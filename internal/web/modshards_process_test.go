package web

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dst-windows/internal/server"
)

// The child uses this test executable as an isolated game process; no real server is launched.
func init() {
	if os.Getenv("DST_MOD_SHARD_TEST_CHILD") != "1" {
		return
	}
	fmt.Println("Server is now accepting connections on 10999")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "c_shutdown()") {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func TestModWritesRejectedForRunningAndStoppingRoom(t *testing.T) {
	s, rm := modShardFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DST_MOD_SHARD_TEST_CHILD", "1")
	s.mgr.SetExeResolver(func() (string, string) { return executable, filepath.Dir(executable) })
	s.mgr.Register(server.NewCluster(rm))
	if err := s.mgr.StartShard(rm.Key(), "Master"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		s.mgr.BeginShutdown()
		_ = s.mgr.SendCmd(rm.Key(), "Master", "c_shutdown()")
		_ = s.mgr.Stop(rm.Key(), time.Second)
	}()
	check := func() {
		t.Helper()
		for _, path := range []string{"/api/mods/toggle?shard=Caves&id=workshop-100&enabled=true", "/api/mods/config?shard=Caves&id=workshop-100&key=amount&value=3"} {
			_, body := doJSON(t, s.Handler(), http.MethodGet, path, nil, nil)
			if body["code"] == float64(200) || !strings.Contains(body["message"].(string), "请先停止全部分片") {
				t.Fatal(body)
			}
		}
		o, err := s.modsFor(rm, "Caves").LoadOverrides()
		if err != nil || o.Enabled["workshop-100"] || o.Config["workshop-100"]["amount"] != float64(9) {
			t.Fatal("拒绝后仍修改了文件", o, err)
		}
	}
	check()
	stopped := make(chan error, 1)
	go func() { stopped <- s.mgr.StopShard(rm.Key(), "Master", time.Second) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		statuses := s.mgr.Status(rm.Key())
		if len(statuses) > 0 && statuses[0].State == "stopping" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("未进入停止状态")
		}
		time.Sleep(10 * time.Millisecond)
	}
	check()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}
