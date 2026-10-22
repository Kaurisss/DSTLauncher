package server

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

type rollbackInput struct{ write func(string) }

func (w rollbackInput) Write(b []byte) (int, error) { w.write(string(b)); return len(b), nil }
func (w rollbackInput) Close() error                { return nil }

func rollbackFixture(t *testing.T, reply func(*Manager, string, string, string)) (*Manager, *Cluster, map[string][]string) {
	t.Helper()
	m := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
	c := &Cluster{ID: 2, Master: "Surface", Shards: []string{"Surface", "Below"}}
	m.Register(c)
	commands := map[string][]string{}
	markerRE := regexp.MustCompile(`DST_PANEL_ROLLBACK_[0-9a-f]{24}`)
	for _, name := range c.Shards {
		shard := name
		m.procs[procKey(c.Key(), shard)] = &process{
			cluster: c, shard: shard, state: StateRunning,
			cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}},
			stdin: rollbackInput{write: func(command string) {
				commands[shard] = append(commands[shard], command)
				reply(m, c.Key(), shard, markerRE.FindString(command))
			}},
		}
	}
	return m, c, commands
}

func TestRollbackUsesOnlyMasterAndRejectsRepeatedRequest(t *testing.T) {
	m, c, commands := rollbackFixture(t, func(m *Manager, cluster, shard, marker string) {
		m.emit(cluster, shard, "log", "[00:00:30]: "+marker+":accepted\t")
	})
	if err := m.Rollback(c.Key(), 2); err != nil {
		t.Fatal(err)
	}
	if len(commands["Surface"]) != 1 || len(commands["Below"]) != 0 {
		t.Fatalf("应只向主分片发送一条集群回档指令: %v", commands)
	}
	command := commands["Surface"][0]
	if !strings.Contains(command, "snapshots[3]") || !strings.Contains(command, `ipairs({"Below"})`) {
		t.Fatalf("没有选择目标历史存档或校验次分片: %s", command)
	}
	if !strings.Contains(command, "TheNet:TruncateSnapshots(TheWorld.meta.session_identifier,-3)") {
		t.Fatal("必须使用原生接口的负数回退参数，正数仅清理旧存档")
	}
	if strings.Contains(command, "c_save(") {
		t.Fatal("回档前不能额外保存并改变目标存档")
	}
	if err := m.Rollback(c.Key(), 2); err == nil || !strings.Contains(err.Error(), "正在回档") {
		t.Fatalf("重复请求未被拦截: %v", err)
	}
	if len(commands["Surface"]) != 1 {
		t.Fatal("重复请求发送了第二条命令")
	}
}

func TestRollbackRequiresAllEnabledShardsAndValidCount(t *testing.T) {
	m, c, commands := rollbackFixture(t, func(*Manager, string, string, string) { t.Fatal("不应发送命令") })
	for _, count := range []int{-1, 0, 100} {
		if err := m.Rollback(c.Key(), count); err == nil {
			t.Errorf("接受了非法回退次数 %d", count)
		}
	}
	m.procs[procKey(c.Key(), "Below")].exited.Store(true)
	if err := m.Rollback(c.Key(), 1); err == nil || !strings.Contains(err.Error(), "Below") {
		t.Fatalf("未拦截未运行的分片: %v", err)
	}
	if len(commands) != 0 {
		t.Fatal("校验失败仍发送了命令")
	}
	m.BeginShutdown()
	if err := m.Rollback(c.Key(), 1); err != errShuttingDown {
		t.Fatalf("关闭过程中仍接受回档: %v", err)
	}
}

func TestRollbackRejectsEchoAndUnrelatedRepliesAndAllowsRetry(t *testing.T) {
	for _, reason := range []string{"not_ready", "insufficient"} {
		t.Run(reason, func(t *testing.T) {
			response := reason
			m, c, _ := rollbackFixture(t, func(m *Manager, cluster, shard, marker string) {
				m.emit(cluster, shard, "log", fmt.Sprintf(`[00:00:30]: RemoteCommandInput: "print(%q)"`, marker+":accepted"))
				m.emit("Cluster_999", shard, "log", marker+":accepted")
				m.emit(cluster, "OtherShard", "log", marker+":accepted")
				m.emit(cluster, shard, "log", "[00:00:30]: "+marker+":"+response)
			})
			if err := m.Rollback(c.Key(), 1); err == nil {
				t.Fatal("错误地把命令回显或其它分片的回执当成成功")
			}
			response = "accepted"
			if err := m.Rollback(c.Key(), 1); err != nil {
				t.Fatalf("游戏明确拒绝后不允许重试: %v", err)
			}
		})
	}
}
