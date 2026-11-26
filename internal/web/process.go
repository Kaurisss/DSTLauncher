package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dst-windows/internal/room"
	"dst-windows/internal/server"
)

// ---- 房间管理员（adminlist.txt）----

// handleAdmins 管理员名单的查询 / 追加 / 移除。
// 名单在服务器启动时读取：运行中修改不报错，但提示需重启生效。
func (s *Server) handleAdmins(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	clusterDir := rm.ClusterDir(s.rootDir, s.confDir)
	path := room.AdminsPath(clusterDir)

	switch r.Method {
	case http.MethodGet:
		list, err := room.ReadAdmins(clusterDir)
		respond(w, 200, map[string]interface{}{
			"file":        path,
			"admins":      list,
			"needRestart": s.mgr.AnyRunning(rm.Key()),
			"seen":        s.seenPlayers(clusterDir, rm.Shards),
		}, err)

	case http.MethodPost:
		var in struct {
			KleiID string `json:"kleiId"`
		}
		if err := decodeJSON(w, r, &in, 4096); err != nil {
			respond(w, 200, nil, err)
			return
		}
		list, err := room.AddAdmin(clusterDir, in.KleiID)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, map[string]interface{}{
			"file":        path,
			"admins":      list,
			"needRestart": s.mgr.AnyRunning(rm.Key()),
			"seen":        s.seenPlayers(clusterDir, rm.Shards),
		}, nil)

	case http.MethodDelete:
		list, err := room.RemoveAdmin(clusterDir, query(r, "id"))
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, map[string]interface{}{
			"file":        path,
			"admins":      list,
			"needRestart": s.mgr.AnyRunning(rm.Key()),
			"seen":        s.seenPlayers(clusterDir, rm.Shards),
		}, nil)

	default:
		respond(w, 200, nil, fmt.Errorf("不支持的方法: %s", r.Method))
	}
}

// ---- 进程控制 ----

// handleHealth 返回固定的健康状态，供容器编排探活。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]interface{}{
		"status":  "ok",
		"version": s.version,
	}, nil)
}

// handleStatus 返回分片状态。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	respond(w, 200, s.mgr.Status(rm.Key()), nil)
}

// withRoom 把「取房间 -> 执行 -> 回状态」三步收敛成一个调用。
func (s *Server) withRoom(w http.ResponseWriter, r *http.Request, fn func(rm *room.Room) error) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	err = fn(rm)
	respond(w, 200, s.mgr.Status(rm.Key()), err)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	// 安装/更新服务端或下载模组时不能开服：三者都会改动游戏安装目录与共享 ugc 目录
	if err := s.busyTaskError(""); err != nil {
		respond(w, 200, nil, err)
		return
	}
	// 在线开服没有令牌会被 Steam 拒绝注册，提前拦下比等服务器报错更省事
	if rm.NeedsToken() && !rm.HasToken() {
		respond(w, 200, nil, fmt.Errorf("当前为在线模式，请先填写 Klei 服务器令牌（cluster_token.txt）"))
		return
	}

	shard := query(r, "shard")
	if err := s.checkShardMods(rm, shard); err != nil {
		respond(w, 200, nil, err)
		return
	}
	if shard == "" {
		err = s.mgr.Start(rm.Key())
	} else {
		err = s.mgr.StartShard(rm.Key(), shard)
	}
	respond(w, 200, s.mgr.Status(rm.Key()), err)
}

// checkShardMods validates the actual start/restart scope before stopping or launching anything.
func (s *Server) checkShardMods(rm *room.Room, shard string) error {
	if shard != "" {
		sh, ok := rm.Shard(shard)
		if !ok || !sh.Enabled() {
			return fmt.Errorf("分片不存在或未启用: %s", shard)
		}
	}
	if s.mgr.UgcDir() != "" {
		mods := s.modsFor(rm)
		if shard != "" {
			mods = s.modsFor(rm, shard)
		}
		missing, err := mods.MissingMods()
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return fmt.Errorf("以下模组尚未下载：%s。请先到「模组」页下载缺失模组", strings.Join(missing, "、"))
		}
	}

	return nil
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	timeout := 30 * time.Second
	if v := query(r, "timeout"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}
	s.withRoom(w, r, func(rm *room.Room) error {
		if shard := query(r, "shard"); shard != "" {
			return s.mgr.StopShard(rm.Key(), shard, timeout)
		}
		return s.mgr.Stop(rm.Key(), timeout)
	})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	if err := s.busyTaskError(""); err != nil {
		respond(w, 200, s.mgr.Status(rm.Key()), err)
		return
	}
	// 重启前必须优雅关闭，否则会丢存档
	if err := s.checkShardMods(rm, query(r, "shard")); err != nil {
		respond(w, 200, nil, err)
		return
	}
	if shard := query(r, "shard"); shard != "" {
		if err := s.mgr.StopShard(rm.Key(), shard, 30*time.Second); err != nil {
			respond(w, 200, s.mgr.Status(rm.Key()), err)
			return
		}
		time.Sleep(2 * time.Second)
		err = s.mgr.StartShard(rm.Key(), shard)
	} else {
		if err := s.mgr.Stop(rm.Key(), 30*time.Second); err != nil {
			respond(w, 200, s.mgr.Status(rm.Key()), err)
			return
		}
		time.Sleep(2 * time.Second)
		err = s.mgr.Start(rm.Key())
	}
	respond(w, 200, s.mgr.Status(rm.Key()), err)
}

func (s *Server) handleCmd(w http.ResponseWriter, r *http.Request) {
	shard, cmd := query(r, "shard"), query(r, "cmd")
	if strings.TrimSpace(cmd) == "" {
		respond(w, 200, nil, fmt.Errorf("命令不能为空"))
		return
	}
	s.withRoom(w, r, func(rm *room.Room) error {
		// 缺省发往主分片：分片名不再固定是 Master
		if shard == "" {
			shard = rm.MasterShard()
		}
		return s.mgr.SendCmd(rm.Key(), shard, cmd)
	})
}

// ---- 日志 ----

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	n := 200
	if v := query(r, "lines"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			n = i
		}
	}
	lines, err := s.mgr.ReadLog(rm.Key(), shardOf(r, rm), n)
	respond(w, 200, lines, err)
}

// shardOf 读取分片参数，缺省为主分片。
func shardOf(r *http.Request, rm *room.Room) string {
	if s := query(r, "shard"); s != "" {
		return s
	}
	return rm.MasterShard()
}

// matchEvent 判断事件是否命中订阅者的过滤条件。
// 空值表示不过滤（订阅全部）。
func matchEvent(ev server.Event, cluster, shard string) bool {
	if cluster != "" && ev.Cluster != cluster {
		return false
	}
	if shard != "" && ev.Shard != shard {
		return false
	}
	return true
}

// handleLogStream 以 SSE 推送实时日志与进程事件。
//
// 相比 WebSocket，SSE 在本机场景下更简单且自带断线重连。
// 每个连接独立订阅（broker 扇出），多开标签页不会互相抢事件。
// 用 ?room=<id>&shard=<name> 只订阅指定房间的分片，缺省则推送全部。
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// 房间参数不合法时直接拒绝，避免前端静默收到空流
	wantCluster := ""
	if query(r, "room") != "" {
		rm, err := s.roomByID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		wantCluster = rm.Key()
	}
	wantShard := query(r, "shard")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	lines := 200
	if v := query(r, "lines"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			lines = i
		}
	}

	send := func(ev server.Event) {
		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
	}

	// 先补发历史日志，避免前端刚打开时一片空白
	if wantCluster != "" && wantShard != "" {
		if hist, err := s.mgr.ReadLog(wantCluster, wantShard, lines); err == nil {
			for _, l := range hist {
				send(server.Event{
					Time: time.Now(), Cluster: wantCluster, Shard: wantShard,
					Type: "log", Message: l,
				})
			}
		}
	}
	flusher.Flush()

	id, ch := s.mgr.Subscribe()
	defer s.mgr.Unsubscribe(id)

	// 每 15 秒发一个注释心跳，防止中间层把空闲连接掐掉
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			// 把队列里已积压的事件一次性带出去，合并成一次 flush。
			// 启动阶段每秒几百行日志，逐行 flush 会把写系统调用与前端的
			// SSE 事件解析都打满；批量发送对浏览器没有任何语义差别。
			count := 0
		drain:
			for {
				if matchEvent(ev, wantCluster, wantShard) {
					send(ev)
					count++
				}
				select {
				case next, ok2 := <-ch:
					if !ok2 {
						break drain
					}
					ev = next
				default:
					break drain
				}
			}
			if count > 0 {
				flusher.Flush()
			}
		}
	}
}
