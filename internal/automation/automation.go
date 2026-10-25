// Package automation persists opt-in maintenance schedules; one worker executes jobs serially.
package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"dst-windows/internal/config"
)

type Config struct {
	BackupHours   int    `json:"backupHours"`
	Keep          int    `json:"keep"`
	RestartAt     string `json:"restartAt"`
	UpdateMinutes int    `json:"updateMinutes"`
	UpdateMode    string `json:"updateMode"`
}

func (c Config) Validate() error {
	if c.BackupHours < 0 || c.BackupHours > 720 {
		return fmt.Errorf("备份间隔必须为 0–720 小时，0 表示关闭")
	}
	if c.Keep < 1 || c.Keep > 100 {
		return fmt.Errorf("自动备份保留份数必须为 1–100")
	}
	if c.RestartAt != "" {
		if _, err := time.Parse("15:04", c.RestartAt); err != nil {
			return fmt.Errorf("重启时间须为 HH:mm")
		}
	}
	if c.UpdateMinutes != 0 && (c.UpdateMinutes < 15 || c.UpdateMinutes > 1440) {
		return fmt.Errorf("更新检查间隔必须为 15–1440 分钟，0 表示关闭")
	}
	if c.UpdateMode != "notify" && c.UpdateMode != "auto" && c.UpdateMode != "idle" {
		return fmt.Errorf("更新模式须为 notify、auto 或 idle")
	}
	return nil
}

type Job struct {
	Next   time.Time `json:"next"`
	Last   time.Time `json:"last"`
	Result string    `json:"result"`
}
type Snapshot struct {
	Config   Config         `json:"config"`
	Jobs     map[string]Job `json:"jobs"`
	Running  string         `json:"running"`
	Timezone string         `json:"timezone"`
}
type Engine struct {
	mu    sync.Mutex
	path  string
	state Snapshot
}

func New(path string) (*Engine, error) {
	e := &Engine{path: path, state: Snapshot{Config: Config{Keep: 10, UpdateMode: "notify"}, Jobs: map[string]Job{}}}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		var saved *Snapshot
		if err = json.Unmarshal(b, &saved); err != nil {
			return nil, err
		}
		if saved == nil {
			return nil, fmt.Errorf("自动维护配置必须是 JSON 对象")
		}
		e.state = *saved
		if err = e.state.Config.Validate(); err != nil {
			return nil, err
		}
	}
	if e.state.Jobs == nil {
		e.state.Jobs = map[string]Job{}
	}
	if key := e.state.Running; key != "" {
		j := e.state.Jobs[key]
		j.Result = "上次执行被面板退出中断，请检查服务器状态"
		e.state.Jobs[key] = j
		e.state.Running = ""
	}
	return e, nil
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.state
	s.Jobs = make(map[string]Job, len(e.state.Jobs))
	for key, job := range e.state.Jobs {
		s.Jobs[key] = job
	}
	s.Timezone = time.Now().Format("MST (UTC-07:00)")
	return s
}

func next(c Config, kind string, now time.Time) time.Time {
	switch kind {
	case "backup":
		if c.BackupHours > 0 {
			return now.Add(time.Duration(c.BackupHours) * time.Hour)
		}
	case "update":
		if c.UpdateMinutes > 0 {
			return now.Add(time.Duration(c.UpdateMinutes) * time.Minute)
		}
	case "restart":
		if c.RestartAt != "" {
			clock, _ := time.Parse("15:04", c.RestartAt)
			t := time.Date(now.Year(), now.Month(), now.Day(), clock.Hour(), clock.Minute(), 0, 0, now.Location())
			if !t.After(now) {
				t = t.AddDate(0, 0, 1)
			}
			return t
		}
	}
	return time.Time{}
}

func (e *Engine) persist() error {
	b, err := json.MarshalIndent(e.state, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(e.path, b)
}

func (e *Engine) Configure(c Config, now time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Running != "" {
		return fmt.Errorf("维护任务正在执行，请完成后再修改计划")
	}
	old := e.state
	e.state.Config = c
	e.state.Jobs = make(map[string]Job, 3)
	for _, kind := range []string{"backup", "restart", "update"} {
		job := old.Jobs[kind]
		changed := kind == "backup" && c.BackupHours != old.Config.BackupHours || kind == "restart" && c.RestartAt != old.Config.RestartAt || kind == "update" && c.UpdateMinutes != old.Config.UpdateMinutes
		if changed || job.Next.IsZero() {
			job.Next = next(c, kind, now)
		}
		e.state.Jobs[kind] = job
	}
	if err := e.persist(); err != nil {
		e.state = old
		return err
	}
	return nil
}

type Run func(context.Context, string, Config) (string, error)

func (e *Engine) Tick(ctx context.Context, now time.Time, run Run) error {
	for _, kind := range []string{"backup", "restart", "update"} {
		if err := ctx.Err(); err != nil {
			return err
		}
		e.mu.Lock()
		job := e.state.Jobs[kind]
		if e.state.Running != "" || job.Next.IsZero() || now.Before(job.Next) {
			e.mu.Unlock()
			continue
		}
		c := e.state.Config
		// Do not replay a backlog or run twice after a crash/restart.
		previous := job
		job.Last, job.Next, job.Result = now, next(c, kind, now), "执行中"
		e.state.Jobs[kind], e.state.Running = job, kind
		if err := e.persist(); err != nil {
			e.state.Jobs[kind], e.state.Running = previous, ""
			e.mu.Unlock()
			return err
		}
		e.mu.Unlock()
		result, err := run(ctx, kind, c)
		if err != nil {
			result = err.Error()
		}
		e.mu.Lock()
		job.Result = result
		e.state.Jobs[kind], e.state.Running = job, ""
		err = e.persist()
		e.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) Serve(ctx context.Context, run Run, report func(error)) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := e.Tick(ctx, now, run); err != nil && ctx.Err() == nil {
				report(err)
			}
		}
	}
}
