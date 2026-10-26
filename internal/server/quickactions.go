package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type QuickAction struct {
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

type QuickResult struct {
	Action  string            `json:"action"`
	Shards  []string          `json:"shards"`
	Skipped []string          `json:"skipped"`
	Errors  map[string]string `json:"errors"`
}

func (a QuickAction) Validate() error {
	switch a.Action {
	case "save", "skipday":
		if a.Message != "" {
			return fmt.Errorf("此操作不接受公告内容")
		}
	case "announce":
		message := strings.TrimSpace(a.Message)
		if message == "" || !utf8.ValidString(message) || utf8.RuneCountInString(message) > 500 {
			return fmt.Errorf("公告须为 1–500 个字符")
		}
		for _, r := range message {
			if unicode.IsControl(r) {
				return fmt.Errorf("公告不能包含换行或控制字符")
			}
		}
	default:
		return fmt.Errorf("不支持的快捷操作")
	}
	return nil
}

// Quick runs a small structured set of commands. Saves wait for each active
// shard's actual disk callback. Other actions wait for a correlated game reply.
func (m *Manager) Quick(ctx context.Context, clusterKey string, a QuickAction) (QuickResult, error) {
	result := QuickResult{Action: a.Action, Shards: []string{}, Skipped: []string{}, Errors: map[string]string{}}
	if err := a.Validate(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if m.closing.Load() {
		return result, errShuttingDown
	}
	c, ok := m.GetCluster(clusterKey)
	if !ok {
		return result, fmt.Errorf("房间不存在")
	}
	if a.Action == "save" {
		running := []string{}
		for _, shard := range c.Shards {
			p, exists := m.getProcess(clusterKey, shard)
			if !exists || !p.isAlive() {
				result.Skipped = append(result.Skipped, shard)
				continue
			}
			running = append(running, shard)
		}
		if len(running) == 0 {
			return result, fmt.Errorf("没有运行中的分片可以保存")
		}
		var wg sync.WaitGroup
		var resultMu sync.Mutex
		for _, shard := range running {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := m.Maintenance(ctx, clusterKey, shard, "save")
				resultMu.Lock()
				defer resultMu.Unlock()
				if err != nil {
					result.Errors[shard] = err.Error()
				} else {
					result.Shards = append(result.Shards, shard)
				}
			}()
		}
		wg.Wait()
		sort.Strings(result.Shards)
		return result, nil
	}
	master := c.MasterShard()
	p, exists := m.getProcess(clusterKey, master)
	if !exists || !p.isAlive() {
		return result, fmt.Errorf("请先启动主分片，等待世界加载完成")
	}
	p.mu.RLock()
	running := p.state == StateRunning
	p.mu.RUnlock()
	if !running {
		return result, fmt.Errorf("主分片正在启停，请稍后重试")
	}
	if a.Action == "skipday" {
		// Share the existing world-mutation cooldown with rollback. A lost reply
		// must not cause a retry to advance the world twice.
		m.rollbackMu.Lock()
		if time.Now().Before(m.rollbackUntil[clusterKey]) {
			m.rollbackMu.Unlock()
			return result, fmt.Errorf("该房间刚执行过世界时间或回档操作，请等待一分钟后再试")
		}
		if m.rollbackUntil == nil {
			m.rollbackUntil = map[string]time.Time{}
		}
		m.rollbackUntil[clusterKey] = time.Now().Add(RollbackCooldown)
		m.rollbackMu.Unlock()
	}
	accepted, err := m.quickRequest(ctx, clusterKey, master, a)
	if err != nil {
		if a.Action == "skipday" && !accepted {
			m.rollbackMu.Lock()
			delete(m.rollbackUntil, clusterKey)
			m.rollbackMu.Unlock()
		}
		return result, err
	}
	result.Shards = append(result.Shards, master)
	return result, nil
}

func (m *Manager) quickRequest(ctx context.Context, cluster, shard string, a QuickAction) (bool, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return false, err
	}
	marker := "DST_PANEL_QUICK_" + hex.EncodeToString(nonce[:])
	id, events := m.Subscribe()
	defer m.Unsubscribe(id)
	if err := m.SendCmd(cluster, shard, quickCommand(marker, a)); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for {
		select {
		case ev, open := <-events:
			if !open {
				return true, fmt.Errorf("游戏回执连接已关闭，请先检查日志再重试")
			}
			if ev.Cluster != cluster || ev.Shard != shard || ev.Type != "log" {
				continue
			}
			line := strings.TrimSpace(ev.Message)
			if strings.HasPrefix(line, "[") {
				_, line, _ = strings.Cut(line, "]: ")
			}
			switch strings.TrimSpace(line) {
			case marker + ":ok":
				return true, nil
			case marker + ":not_ready":
				return false, fmt.Errorf("主世界尚未加载完成，请稍后重试")
			case marker + ":failed":
				return true, fmt.Errorf("游戏执行失败，请查看主分片日志后再操作")
			}
		case <-ctx.Done():
			return true, fmt.Errorf("未收到游戏回执，请先检查主分片日志，勿立即重复提交: %w", ctx.Err())
		}
	}
}

// Decimal byte escapes work in Lua 5.1, unlike Go's \u and \x escapes. This also
// preserves arbitrary valid UTF-8 without allowing quotes to inject Lua code.
func luaMessage(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, v := range []byte(s) {
		fmt.Fprintf(&b, "\\%03d", v)
	}
	b.WriteByte('"')
	return b.String()
}

func quickCommand(marker string, a QuickAction) string {
	body := "c_skip(1)"
	if a.Action == "announce" {
		body = "c_announce(" + luaMessage(strings.TrimSpace(a.Message)) + ")"
	}
	return fmt.Sprintf(`local function panelquick() local function reply(v) print(%q..":"..v) end; if not TheWorld or not TheWorld.ismastersim or not TheWorld.ismastershard then reply("not_ready"); return end; local ok=pcall(function() %s end); reply(ok and "ok" or "failed") end; panelquick()`, marker, body)
}
