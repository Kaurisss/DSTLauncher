package web

import (
	"fmt"
	"net/http"
	"strconv"

	"dst-windows/internal/chatlog"
)

func (s *Server) handleChatLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
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
	if shard == "" {
		shard = rm.MasterShard()
	}
	if _, ok := rm.Shard(shard); !ok {
		respond(w, 200, nil, fmt.Errorf("房间不存在该分片"))
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
	result, err := chatlog.Read(rm.ClusterDir(s.rootDir, s.confDir), shard, q)
	respond(w, 200, result, err)
}
