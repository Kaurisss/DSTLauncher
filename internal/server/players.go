package server

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed players.lua
var playersLua string

var playerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Player struct {
	UserID     string `json:"userid"`
	Name       string `json:"name"`
	Prefab     string `json:"prefab"`
	Days       *int   `json:"days"`
	Shard      string `json:"shard"`
	Ghost      bool   `json:"ghost"`
	InWorld    bool   `json:"inWorld"`
	Admin      bool   `json:"admin"`
	AdminSaved bool   `json:"adminSaved"`
}

type PlayerList struct {
	Players []Player          `json:"players"`
	Errors  map[string]string `json:"errors"`
}

type PlayerAction struct {
	Shard  string `json:"shard"`
	UserID string `json:"userid"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
}

type playerReply struct {
	Players []Player `json:"-"`
	Player  *Player  `json:"player"`
	Count   *int     `json:"count"`
	OK      bool     `json:"ok"`
	Error   string   `json:"error"`
}

// DST's playerspawner prints this only when a player actually spawns in this shard.
func (p *process) observePlayerLog(line string) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "[") {
		_, line, _ = strings.Cut(line, "]: ")
	}
	if strings.HasPrefix(line, "Spawning player at: [") {
		p.playersReady.Store(true)
	}
}

// Players queries live shards in parallel; a failed shard must not look empty.
func (m *Manager) Players(ctx context.Context, clusterKey string) (PlayerList, error) {
	result := PlayerList{Players: []Player{}, Errors: map[string]string{}}
	cluster, ok := m.GetCluster(clusterKey)
	if !ok {
		return result, fmt.Errorf("房间不存在: %s", clusterKey)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, shard := range cluster.Shards {
		p, found := m.getProcess(clusterKey, shard)
		if !found || !p.isAlive() {
			continue
		}
		p.mu.RLock()
		running := p.state == StateRunning
		p.mu.RUnlock()
		if !running || !p.playersReady.Load() {
			continue
		}
		wg.Add(1)
		go func(shard string) {
			defer wg.Done()
			reply, err := m.playerRequest(ctx, clusterKey, PlayerAction{Shard: shard, Action: "list"})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors[shard] = err.Error()
				return
			}
			for _, player := range reply.Players {
				player.Shard = shard
				result.Players = append(result.Players, player)
			}
		}(shard)
	}
	wg.Wait()
	sort.Slice(result.Players, func(i, j int) bool {
		a, b := result.Players[i], result.Players[j]
		if a.Shard != b.Shard {
			return a.Shard < b.Shard
		}
		return a.UserID < b.UserID
	})
	// Parallel snapshots may catch one player on both sides of a migration.
	seen := make(map[string]bool, len(result.Players))
	unique := result.Players[:0]
	for _, player := range result.Players {
		if !seen[player.UserID] {
			seen[player.UserID] = true
			unique = append(unique, player)
		}
	}
	result.Players = unique
	return result, nil
}

// ActOnPlayer accepts structured operations, never caller-supplied Lua.
func (m *Manager) ActOnPlayer(ctx context.Context, clusterKey string, action PlayerAction) error {
	if !playerIDPattern.MatchString(action.UserID) {
		return fmt.Errorf("玩家 ID 不合法")
	}
	switch action.Action {
	case "kick", "ban", "revive":
		if action.Target != "" {
			return fmt.Errorf("该操作不接受传送目标")
		}
	case "teleport":
		if !playerIDPattern.MatchString(action.Target) || action.Target == action.UserID {
			return fmt.Errorf("请选择同分片的另一位在线玩家作为传送目标")
		}
	default:
		return fmt.Errorf("不支持的玩家操作: %s", action.Action)
	}
	_, err := m.playerRequest(ctx, clusterKey, action)
	return err
}

func (m *Manager) playerRequest(ctx context.Context, clusterKey string, action PlayerAction) (playerReply, error) {
	var result playerReply
	if m.closing.Load() {
		return result, errShuttingDown
	}
	cluster, ok := m.GetCluster(clusterKey)
	if !ok {
		return result, fmt.Errorf("房间不存在: %s", clusterKey)
	}
	found := false
	for _, shard := range cluster.Shards {
		found = found || shard == action.Shard
	}
	if !found {
		return result, fmt.Errorf("分片不存在或未启用: %s", action.Shard)
	}
	p, ok := m.getProcess(clusterKey, action.Shard)
	if !ok || !p.isAlive() {
		return result, fmt.Errorf("分片未运行: %s", action.Shard)
	}
	p.mu.RLock()
	running := p.state == StateRunning
	p.mu.RUnlock()
	if !running {
		return result, fmt.Errorf("分片正在启停，请稍后再试")
	}
	if !p.playersReady.Load() {
		return result, fmt.Errorf("等待分片日志确认玩家进入世界")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return result, fmt.Errorf("创建玩家请求失败: %w", err)
	}
	marker := "DST_PANEL_PLAYERS_" + hex.EncodeToString(nonce[:])
	id, events := m.Subscribe()
	defer m.Unsubscribe(id)
	if err := p.SendCmd(playerCommand(marker, action)); err != nil {
		return result, err
	}
	// One player per log line avoids the tailer’s 16 KB partial-line limit.
	players := []Player{}
	for {
		select {
		case ev, open := <-events:
			if !open {
				return result, fmt.Errorf("玩家回执连接已关闭")
			}
			if ev.Cluster != clusterKey || ev.Shard != action.Shard || ev.Type != "log" {
				continue
			}
			line := strings.TrimSpace(ev.Message)
			if strings.HasPrefix(line, "[") {
				_, line, _ = strings.Cut(line, "]: ")
			}
			payload, matches := strings.CutPrefix(strings.TrimSpace(line), marker+":")
			if !matches {
				continue
			}
			result = playerReply{}
			if len(payload) > 16*1024 || json.Unmarshal([]byte(payload), &result) != nil {
				return result, fmt.Errorf("游戏返回的玩家数据无效")
			}
			if result.Error != "" {
				messages := map[string]string{
					"not_ready":      "世界尚未加载完成，请稍后刷新",
					"not_found":      "玩家已离线或切换了分片，请刷新列表",
					"not_in_world":   "玩家尚未进入世界或正在迁移",
					"not_ghost":      "该玩家不是幽灵，无需复活",
					"target_missing": "传送目标已离线或不在同一分片",
				}
				if message := messages[result.Error]; message != "" {
					return result, fmt.Errorf("%s", message)
				}
				return result, fmt.Errorf("游戏未能执行玩家请求，请查看分片日志")
			}
			if action.Action == "list" {
				if result.Player != nil && len(players) < 256 {
					players = append(players, *result.Player)
					continue
				}
				if result.Count != nil && *result.Count == len(players) {
					result.Players = players
					return result, nil
				}
				return result, fmt.Errorf("玩家数据不完整，请重新刷新")
			}
			if result.OK {
				return result, nil
			}
			return result, fmt.Errorf("游戏未确认玩家请求")
		case <-ctx.Done():
			if action.Action == "list" {
				return result, fmt.Errorf("未收到玩家列表，请检查分片是否已加载完成")
			}
			return result, fmt.Errorf("未收到操作回执，请查看游戏状态，勿立即重复提交")
		}
	}
}

func playerCommand(marker string, action PlayerAction) string {
	// stdin consumes one line at a time; the embedded Lua intentionally has no line comments.
	body := strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(playersLua))
	return fmt.Sprintf(`local function panelplayers() local request={tag=%q,action=%q,userid=%q,target=%q}; %s end; panelplayers()`,
		marker, action.Action, action.UserID, action.Target, body)
}
