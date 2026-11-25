package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"dst-windows/internal/server"
)

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	// Unlike read-only endpoints, never default a destructive operation to the first room.
	if query(r, "room") == "" {
		respond(w, 200, nil, fmt.Errorf("请明确指定要回档的房间"))
		return
	}
	var body struct {
		Count int `json:"count"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		respond(w, 200, nil, fmt.Errorf("回档参数无效: %w", err))
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		respond(w, 200, nil, fmt.Errorf("回档请求只能包含一份参数"))
		return
	}
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	if err := s.busyTaskError(""); err != nil {
		respond(w, 200, nil, err)
		return
	}
	if err := s.mgr.Rollback(rm.Key(), body.Count); err != nil {
		respond(w, 200, nil, err)
		return
	}
	respond(w, 200, map[string]any{
		"requested": true, "count": body.Count,
		"retryAfterSeconds": int(server.RollbackCooldown.Seconds()),
	}, nil)
}
