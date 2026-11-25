package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	wantStream := query(r, "stream") == "1" || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/x-ndjson")
	if !wantStream {
		respond(w, 200, result, nil)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	meta := chatLogStreamEvent{
		Type:      "meta",
		Exists:    result.Exists,
		Truncated: result.Truncated,
		Total:     result.Total,
		Page:      result.Page,
		PageSize:  result.PageSize,
		Count:     len(result.Entries),
	}
	if !result.Modified.IsZero() {
		modified := result.Modified
		meta.Modified = &modified
	}
	if err := enc.Encode(meta); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
	for i := range result.Entries {
		if r.Context().Err() != nil {
			return
		}
		entry := result.Entries[i]
		if err := enc.Encode(chatLogStreamEvent{Type: "entry", Entry: &entry}); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	_ = enc.Encode(chatLogStreamEvent{Type: "done", Total: result.Total})
	if flusher != nil {
		flusher.Flush()
	}
}

type chatLogStreamEvent struct {
	Type      string         `json:"type"`
	Exists    bool           `json:"exists,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
	Modified  *time.Time     `json:"modified,omitempty"`
	Total     int            `json:"total,omitempty"`
	Page      int            `json:"page,omitempty"`
	PageSize  int            `json:"pageSize,omitempty"`
	Count     int            `json:"count,omitempty"`
	Entry     *chatlog.Entry `json:"entry,omitempty"`
	Message   string         `json:"message,omitempty"`
}
