package server

import (
	"testing"
)

// BeginShutdown 之后任何启动请求都必须被拒绝，否则 Docker 发 SIGTERM 停服途中，
// 崩溃自动重启或并发点击会把分片再拉起来。
func TestBeginShutdownRejectsNewStarts(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
	cluster := &Cluster{ID: 1, ConfDir: ConfDir, Shards: []string{"Master"}}
	m.Register(cluster)

	m.BeginShutdown()

	if err := m.startShard(cluster, "Master"); err != errShuttingDown {
		t.Errorf("关闭后启动应返回 errShuttingDown，实际 %v", err)
	}
	if err := m.StartShard(cluster.Key(), "Master"); err == nil {
		t.Error("关闭后 StartShard 不应成功")
	}
}
