//go:build linux

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerRunsAndStopsLinuxShard(t *testing.T) {
	temp := t.TempDir()
	binDir := filepath.Join(temp, "bin64")
	executable := filepath.Join(binDir, "fake-dst")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	const script = `#!/bin/sh
printf '%s\n' 'Server is now accepting connections'
while IFS= read -r line; do
  case "$line" in
    *c_shutdown*) printf '%s\n' 'fake server stopped'; exit 0 ;;
    *c_save*) printf '%s\n' 'fake server saved' ;;
    *) printf 'CMD_ECHO: %s\n' "$line" ;;
  esac
done
`
	if err := os.WriteFile(executable, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(filepath.Join(temp, "save"), binDir, filepath.Join(temp, "logs"))
	manager.SetExeResolver(func() (string, string) { return executable, binDir })
	cluster := &Cluster{
		ID:      1,
		Name:    "Linux 测试房间",
		ConfDir: ConfDir,
		Shards:  []string{"Master"},
		Ports: map[string]ShardPort{
			"Master": {ServerPort: 11999, MasterServerPort: 21999, AuthenticationPort: 31999},
		},
	}
	manager.Register(cluster)
	defer manager.Stop(cluster.Key(), time.Second) //nolint:errcheck

	if err := manager.StartShard(cluster.Key(), "Master"); err != nil {
		t.Fatalf("启动 Linux 分片失败: %v", err)
	}
	waitForShardLog(t, manager, cluster.Key(), "Master", "Server is now accepting connections")

	if err := manager.SendCmd(cluster.Key(), "Master", "TheWorld:PushEvent(\"test\")"); err != nil {
		t.Fatalf("控制台命令失败: %v", err)
	}
	waitForShardLog(t, manager, cluster.Key(), "Master", "CMD_ECHO: TheWorld:PushEvent")

	if err := manager.StopShard(cluster.Key(), "Master", time.Second); err != nil {
		t.Fatalf("优雅关闭失败: %v", err)
	}
	status := manager.Status(cluster.Key())
	if len(status) != 1 || status[0].State != StateStopped.String() {
		t.Fatalf("关闭后状态 = %+v", status)
	}
	logs, err := manager.ReadLog(cluster.Key(), "Master", 50)
	if err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "fake server saved") ||
		!strings.Contains(strings.Join(logs, "\n"), "fake server stopped") {
		t.Fatalf("优雅关闭日志缺失: %v", logs)
	}
}

func waitForShardLog(t *testing.T, manager *Manager, cluster, shard, fragment string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs, _ := manager.ReadLog(cluster, shard, 50)
		if strings.Contains(strings.Join(logs, "\n"), fragment) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	logs, _ := manager.ReadLog(cluster, shard, 50)
	t.Fatalf("等待日志 %q 超时，当前日志: %v", fragment, logs)
}
