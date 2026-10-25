package server

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestMaintenanceWaitsForCorrelatedReplyAndRejectsOccupied(t *testing.T) {
	for _, action := range []string{"save", "empty"} {
		m := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
		c := &Cluster{ID: 1, Master: "Master", Shards: []string{"Master"}}
		m.Register(c)
		markerRE := regexp.MustCompile(`DST_PANEL_MAINT_[0-9a-f]{24}`)
		occupied := false
		m.procs[procKey(c.Key(), "Master")] = &process{
			cluster: c, shard: "Master", state: StateRunning, cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}},
			stdin: rollbackInput{write: func(command string) {
				marker := markerRE.FindString(command)
				if marker == "" {
					t.Error("缺少请求标识")
				}
				if action == "save" && !strings.Contains(command, "SaveCurrent(function()") {
					t.Error("必须等待落盘回调")
				}
				m.emit(c.Key(), "Master", "log", "RemoteCommandInput: "+marker+":ok")
				m.emit("Cluster_2", "Master", "log", marker+":ok")
				reply := "ok"
				if occupied {
					reply = "occupied"
				}
				m.emit(c.Key(), "Master", "log", "[00:00:01]: "+marker+":"+reply)
			}},
		}
		if err := m.Maintenance(context.Background(), c.Key(), "Master", action); err != nil {
			t.Fatal(err)
		}
		occupied = true
		if err := m.Maintenance(context.Background(), c.Key(), "Master", action); err == nil {
			t.Fatal("玩家连接应阻止无人更新")
		}
		if len(m.subs) != 0 {
			t.Fatal("订阅泄漏")
		}
		m.BeginShutdown()
		if err := m.Maintenance(context.Background(), c.Key(), "Master", action); err == nil {
			t.Fatal("关闭后仍在发送维护命令")
		}
	}
}
