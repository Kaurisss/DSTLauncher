package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func quickFixture(t *testing.T, reply string) (*Manager, *Cluster, *int) {
	t.Helper()
	m := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
	c := &Cluster{ID: 5, Master: "Surface", Shards: []string{"Surface", "Below"}}
	m.Register(c)
	calls := 0
	re := regexp.MustCompile(`DST_PANEL_QUICK_[0-9a-f]{24}`)
	m.procs[procKey(c.Key(), "Surface")] = &process{cluster: c, shard: "Surface", state: StateRunning, cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, stdin: rollbackInput{write: func(command string) {
		calls++
		tag := re.FindString(command)
		m.emit(c.Key(), "Surface", "log", "RemoteCommandInput: "+tag+":ok")
		m.emit("Other", "Surface", "log", tag+":ok")
		m.emit(c.Key(), "Below", "log", tag+":ok")
		if reply != "" {
			m.emit(c.Key(), "Surface", "log", "[00:00:01]: "+tag+":"+reply)
		}
	}}}
	return m, c, &calls
}

func TestQuickActionsValidateAndWaitForGameReply(t *testing.T) {
	for _, a := range []QuickAction{{Action: "os.exit()"}, {Action: "save", Message: "extra"}, {Action: "announce"}, {Action: "announce", Message: "a\nb"}, {Action: "announce", Message: strings.Repeat("中", 501)}} {
		if a.Validate() == nil {
			t.Fatalf("accepted %+v", a)
		}
	}
	m, c, calls := quickFixture(t, "ok")
	r, err := m.Quick(context.Background(), c.Key(), QuickAction{Action: "announce", Message: "欢迎来到服务器"})
	if err != nil || len(r.Shards) != 1 || r.Shards[0] != "Surface" || *calls != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if len(m.subs) != 0 {
		t.Fatal("subscriber leak")
	}
	m.BeginShutdown()
	if _, err := m.Quick(context.Background(), c.Key(), QuickAction{Action: "announce", Message: "hello"}); err == nil {
		t.Fatal("accepted after shutdown")
	}
}

func TestQuickSkipCooldownKeepsAmbiguousRepliesButAllowsNotReadyRetry(t *testing.T) {
	for _, reply := range []string{"ok", "not_ready", ""} {
		t.Run(reply, func(t *testing.T) {
			m, c, calls := quickFixture(t, reply)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, err := m.Quick(ctx, c.Key(), QuickAction{Action: "skipday"})
			if (reply == "ok") != (err == nil) {
				t.Fatalf("unexpected reply: %v", err)
			}
			_, again := m.Quick(context.Background(), c.Key(), QuickAction{Action: "skipday"})
			if reply == "not_ready" {
				if *calls != 2 {
					t.Fatal("explicit rejection should be retryable")
				}
			} else if again == nil || *calls != 1 {
				t.Fatal("duplicate world action passed cooldown")
			}
			if len(m.subs) != 0 {
				t.Fatal("subscriber leak")
			}
		})
	}
}

func TestQuickSaveReportsPartialFailuresAndStoppedShards(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
	c := &Cluster{ID: 7, Master: "Surface", Shards: []string{"Surface", "Below", "Stopped"}}
	m.Register(c)
	re := regexp.MustCompile(`DST_PANEL_MAINT_[0-9a-f]{24}`)
	for _, shard := range []string{"Surface", "Below"} {
		m.procs[procKey(c.Key(), shard)] = &process{cluster: c, shard: shard, state: StateRunning, cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, stdin: rollbackInput{write: func(command string) {
			reply := "ok"
			if shard == "Below" {
				reply = "not_ready"
			}
			m.emit(c.Key(), shard, "log", re.FindString(command)+":"+reply)
		}}}
	}
	r, err := m.Quick(context.Background(), c.Key(), QuickAction{Action: "save"})
	if err != nil || len(r.Shards) != 1 || r.Shards[0] != "Surface" || r.Errors["Below"] == "" || len(r.Skipped) != 1 || r.Skipped[0] != "Stopped" {
		t.Fatalf("partial save: %+v %v", r, err)
	}
}

func TestQuickLuaPreservesUTF8AndTreatsInputAsData(t *testing.T) {
	lua := os.Getenv("DST_LUA_EXECUTABLE")
	if lua == "" {
		t.Skip("Lua 5.1 required")
	}
	message := "中文\" ); error('injected'); -- \\ 😀\u2028末"
	for _, ready := range []bool{true, false} {
		world := "nil"
		if ready {
			world = "{ismastersim=true,ismastershard=true}"
		}
		script := "TheWorld=" + world + "; local got=nil; local output=nil; function c_announce(s) got=s end; function print(s) output=s end; " + quickCommand("test-marker", QuickAction{Action: "announce", Message: message})
		if ready {
			script += "; assert(got==" + luaMessage(message) + "); assert(output=='test-marker:ok')"
		} else {
			script += "; assert(got==nil); assert(output=='test-marker:not_ready')"
		}
		path := filepath.Join(t.TempDir(), "quick.lua")
		if err := os.WriteFile(path, []byte(script), 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(lua, path).CombinedOutput(); err != nil {
			t.Fatalf("Lua behavior: %v %s", err, out)
		}
	}
}
