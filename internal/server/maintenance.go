package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// PauseRecovery waits out an in-flight crash restart before maintenance snapshots processes.
func (m *Manager) PauseRecovery() func() {
	m.recoveryMu.Lock()
	m.recoveryPaused = true
	m.recoveryMu.Unlock()
	return func() { m.recoveryMu.Lock(); m.recoveryPaused = false; m.recoveryMu.Unlock() }
}

// Maintenance checks all clients (including character selection) or waits for a save callback.
func (m *Manager) Maintenance(ctx context.Context, cluster, shard, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if action != "save" && action != "empty" {
		return fmt.Errorf("不支持的维护操作")
	}
	if m.closing.Load() {
		return errShuttingDown
	}
	p, ok := m.getProcess(cluster, shard)
	if !ok || !p.isAlive() {
		return fmt.Errorf("分片未运行: %s/%s", cluster, shard)
	}
	p.mu.RLock()
	running := p.state == StateRunning
	p.mu.RUnlock()
	if !running {
		return fmt.Errorf("分片正在启停: %s/%s", cluster, shard)
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	marker := "DST_PANEL_MAINT_" + hex.EncodeToString(nonce[:])
	id, events := m.Subscribe()
	defer m.Unsubscribe(id)
	body := `for _,v in ipairs(TheNet:GetClientTable() or {}) do if v.performance == nil then reply("occupied"); return end end; reply("ok")`
	if action == "save" {
		body = `if not ShardGameIndex then reply("not_ready"); return end; ShardGameIndex:SaveCurrent(function() reply("ok") end)`
	}
	command := fmt.Sprintf(`local function panelmaintenance() local function reply(v) print(%q..":"..v) end; if not TheWorld or not TheWorld.ismastersim then reply("not_ready"); return end; %s end; panelmaintenance()`, marker, body)
	if err := p.SendCmd(command); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		select {
		case event := <-events:
			if event.Cluster != cluster || event.Shard != shard || event.Type != "log" {
				continue
			}
			line := strings.TrimSpace(event.Message)
			if strings.HasPrefix(line, "[") {
				_, line, _ = strings.Cut(line, "]: ")
			}
			if line == marker+":ok" {
				return nil
			}
			if line == marker+":occupied" {
				return fmt.Errorf("%s/%s 仍有玩家连接，推迟更新", cluster, shard)
			}
			if line == marker+":not_ready" {
				return fmt.Errorf("%s/%s 世界尚未就绪", cluster, shard)
			}
		case <-ctx.Done():
			return fmt.Errorf("%s/%s 未确认%s: %w", cluster, shard, action, ctx.Err())
		}
	}
}
