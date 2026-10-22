package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const RollbackCooldown = time.Minute

// Rollback uses the master's own snapshot list and native truncation/reset APIs.
// Secondary shards synchronize to the master's snapshot when the world reloads.
// It never saves first: that would change the snapshot the user asked to restore.
func (m *Manager) Rollback(clusterKey string, count int) error {
	if count < 1 || count > 99 {
		return fmt.Errorf("回退存档次数必须是 1-99 的整数")
	}
	if m.closing.Load() {
		return errShuttingDown
	}
	c, ok := m.GetCluster(clusterKey)
	if !ok {
		return fmt.Errorf("房间不存在: %s", clusterKey)
	}
	for _, shard := range c.Shards {
		p, found := m.getProcess(clusterKey, shard)
		if !found || !p.isAlive() {
			return fmt.Errorf("请先启动房间所有启用的分片，等待世界加载完成（%s 未运行）", shard)
		}
		p.mu.RLock()
		running := p.state == StateRunning
		p.mu.RUnlock()
		if !running {
			return fmt.Errorf("分片 %s 正在启停，请等待世界加载完成后回档", shard)
		}
	}
	if len(c.Shards) == 0 {
		return fmt.Errorf("房间没有启用的分片")
	}
	m.rollbackMu.Lock()
	if time.Now().Before(m.rollbackUntil[clusterKey]) {
		m.rollbackMu.Unlock()
		return fmt.Errorf("该房间正在回档，请等待一分钟后再试")
	}
	if m.rollbackUntil == nil {
		m.rollbackUntil = make(map[string]time.Time)
	}
	m.rollbackUntil[clusterKey] = time.Now().Add(RollbackCooldown)
	m.rollbackMu.Unlock()
	// An explicit game rejection is safe to retry. A missing acknowledgement is
	// ambiguous, so keep the cooldown to prevent repeating a possibly accepted reset.
	clearPending := func() {
		m.rollbackMu.Lock()
		delete(m.rollbackUntil, clusterKey)
		m.rollbackMu.Unlock()
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		clearPending()
		return fmt.Errorf("创建回档请求失败: %w", err)
	}
	marker := "DST_PANEL_ROLLBACK_" + hex.EncodeToString(nonce[:])
	id, events := m.Subscribe()
	defer m.Unsubscribe(id)
	master := c.MasterShard()
	if err := m.SendCmd(clusterKey, master, rollbackCommand(marker, count, c.Shards, master)); err != nil {
		clearPending()
		return err
	}
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	for {
		select {
		case ev := <-events:
			if ev.Cluster != clusterKey || ev.Shard != master || ev.Type != "log" {
				continue
			}
			// Match only an actual printed reply, not RemoteCommandInput's echo.
			_, reply, found := strings.Cut(ev.Message, "]: ")
			if !found {
				reply = ev.Message
			}
			reply = strings.TrimSpace(reply)
			if !strings.HasPrefix(reply, marker+":") {
				continue
			}
			switch strings.TrimPrefix(reply, marker+":") {
			case "accepted":
				m.emit(clusterKey, master, "rollback", fmt.Sprintf("已请求回退 %d 个存档点，等待所有世界重新加载", count))
				return nil
			case "not_ready":
				clearPending()
				return fmt.Errorf("世界尚未加载完成或分片尚未连接，请稍后再试")
			case "insufficient":
				clearPending()
				return fmt.Errorf("可用历史存档不足，无法回退 %d 个存档点", count)
			default:
				clearPending()
				return fmt.Errorf("游戏拒绝回档，请查看主分片日志")
			}
		case <-timer.C:
			return fmt.Errorf("未收到游戏回档回执，请查看主分片日志确认结果，勿立即重复提交")
		}
	}
}

func rollbackCommand(marker string, count int, shards []string, master string) string {
	var required []string
	for _, shard := range shards {
		if shard != master {
			required = append(required, fmt.Sprintf("%q", shard))
		}
	}
	// Select a concrete snapshot so count=1 always restores the snapshot before
	// the most recent saved snapshot, independent of autosave timing. The native
	// negative argument selects a rollback; a positive ID only prunes old files.
	return fmt.Sprintf(`local function panelrollback() local tag=%q; local function reply(s) print(tag..":"..s) end; if not TheWorld or not TheWorld.ismastershard or not TheWorld.net or not TheWorld.net.components.autosaver or not ShardGameIndex then reply("not_ready"); return end; local connected=Shard_GetConnectedShards(); for _,name in ipairs({%s}) do local found=false; for _,v in pairs(connected or {}) do if v.ready and v.shard_name==name then found=true end end; if not found then reply("not_ready"); return end end; local data=ShardGameIndex:GetServerData(); local snapshots=TheNet:ListSnapshots(TheWorld.meta.session_identifier,data.online_mode~=false,100); local target=snapshots and snapshots[%d]; if not target or not target.snapshot_id or target.snapshot_id<=0 then reply("insufficient"); return end; TheNet:TruncateSnapshots(TheWorld.meta.session_identifier,-%d); reply("accepted"); c_reset() end; panelrollback()`, marker, strings.Join(required, ","), count+1, count+1)
}
