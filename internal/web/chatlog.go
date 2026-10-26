package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"dst-windows/internal/chatlog"
)

func (s *Server) StartPlayerLogs() func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			s.collectPlayerLogs()
			select {
			case <-ctx.Done():
				s.collectPlayerLogs()
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}

func (s *Server) collectPlayerLogs() {
	for _, rm := range s.rooms.List() {
		for _, shard := range rm.Shards {
			if err := s.playerLogs.Sync(rm.ClusterDir(s.rootDir, s.confDir), shard.Name); err != nil {
				log.Printf("采集玩家日志 %s/%s: %v", rm.Key(), shard.Name, err)
			}
		}
	}
}

func (s *Server) handleChatLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if query(r, "room") == "" {
		respond(w, 200, nil, fmt.Errorf("请明确指定聊天记录所属房间"))
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	shard := query(r, "shard")
	if shard == "" && r.Method == http.MethodGet {
		shard = rm.MasterShard()
	}
	if _, ok := rm.Shard(shard); !ok {
		respond(w, 200, nil, fmt.Errorf("房间不存在该分片"))
		return
	}
	clusterDir := rm.ClusterDir(s.rootDir, s.confDir)
	if r.Method == http.MethodDelete {
		var body struct {
			IDs []int64 `json:"ids"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			respond(w, 200, nil, err)
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			respond(w, 200, nil, fmt.Errorf("仅接受一份日志删除请求"))
			return
		}
		deleted, err := s.playerLogs.Delete(clusterDir, shard, body.IDs)
		respond(w, 200, map[string]int{"deleted": deleted}, err)
		return
	}
	q := chatlog.Query{Limit: 200, Kind: query(r, "kind"), Search: query(r, "search")}
	if q.Kind == "" {
		q.Kind = "all"
	}
	if raw := query(r, "limit"); raw != "" {
		q.Limit, err = strconv.Atoi(raw)
		if err != nil {
			respond(w, 200, nil, fmt.Errorf("日志条数须为整数"))
			return
		}
	}
	if raw := query(r, "page"); raw != "" {
		q.Page, err = strconv.Atoi(raw)
		if err != nil || q.Page < 1 {
			respond(w, 200, nil, fmt.Errorf("页码须为正整数"))
			return
		}
	}
	result, err := s.playerLogs.Query(clusterDir, shard, q)
	respond(w, 200, result, err)
}
