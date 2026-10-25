package automation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchedulesPersistAndDoNotReplayBacklog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "automation.json")
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 3, 0, 0, 0, time.FixedZone("CST", 8*3600))
	c := Config{BackupHours: 2, Keep: 3, RestartAt: "04:00", UpdateMinutes: 30, UpdateMode: "idle"}
	if err := e.Configure(c, now); err != nil {
		t.Fatal(err)
	}
	state := e.Snapshot()
	if !state.Jobs["backup"].Next.Equal(now.Add(2*time.Hour)) || !state.Jobs["restart"].Next.Equal(now.Add(time.Hour)) {
		t.Fatal(state)
	}
	e, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]int{}
	run := func(_ context.Context, kind string, _ Config) (string, error) {
		calls[kind]++
		if err := e.Configure(c, now); err == nil {
			t.Error("执行中不得修改计划")
		}
		if kind == "update" {
			return "", errors.New("Steam 暂时不可用")
		}
		return "完成", nil
	}
	later := now.Add(25 * time.Hour)
	if err := e.Tick(context.Background(), later, run); err != nil {
		t.Fatal(err)
	}
	if err := e.Tick(context.Background(), later, run); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"backup", "restart", "update"} {
		if calls[kind] != 1 {
			t.Fatalf("%s 重复补跑: %v", kind, calls)
		}
	}
	e, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Tick(context.Background(), later, run); err != nil {
		t.Fatal(err)
	}
	if calls["update"] != 1 || e.Snapshot().Jobs["update"].Result != "Steam 暂时不可用" {
		t.Fatal(e.Snapshot())
	}
	c.BackupHours, c.RestartAt, c.UpdateMinutes = 0, "", 0
	if err := e.Configure(c, later); err != nil {
		t.Fatal(err)
	}
	for _, j := range e.Snapshot().Jobs {
		if !j.Next.IsZero() {
			t.Fatal("关闭后仍安排执行")
		}
	}
}

func TestValidationAndPersistenceFailures(t *testing.T) {
	valid := Config{Keep: 10, UpdateMode: "notify"}
	for _, change := range []func(*Config){
		func(c *Config) { c.Keep = 0 }, func(c *Config) { c.BackupHours = -1 },
		func(c *Config) { c.UpdateMinutes = 1 }, func(c *Config) { c.RestartAt = "24:00" }, func(c *Config) { c.UpdateMode = "unknown" },
	} {
		c := valid
		change(&c)
		if c.Validate() == nil {
			t.Errorf("非法配置被接受: %+v", c)
		}
	}
	path := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(path, []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("损坏文件不能静默重置")
	}
	e, err := New(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	e.path = filepath.Join(path, "cannot-write")
	c := valid
	c.BackupHours = 1
	if e.Configure(c, time.Now()) == nil {
		t.Fatal("写入失败应报告错误")
	}
	if e.Snapshot().Config.BackupHours != 0 {
		t.Fatal("持久化失败仍修改了运行配置")
	}
}

func TestDisabledAndCancelledNeverRun(t *testing.T) {
	e, _ := New(filepath.Join(t.TempDir(), "state"))
	run := func(context.Context, string, Config) (string, error) { t.Fatal("不应执行"); return "", nil }
	if err := e.Tick(context.Background(), time.Now(), run); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(e.Tick(ctx, time.Now(), run), context.Canceled) {
		t.Fatal("未尊重取消")
	}
}
