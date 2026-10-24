package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"dst-windows/internal/room"
	"dst-windows/internal/server"
)

func (s *Server) handlePlayers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if query(r, "room") == "" {
		respond(w, 200, nil, fmt.Errorf("请明确指定要查询的房间"))
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	result, err := s.mgr.Players(r.Context(), rm.Key())
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	admins, err := room.ReadAdmins(rm.ClusterDir(s.rootDir, s.confDir))
	if err != nil {
		result.Errors["管理员名单"] = err.Error()
	}
	saved := make(map[string]bool, len(admins))
	for _, id := range admins {
		saved[strings.ToLower(id)] = true
	}
	for i := range result.Players {
		result.Players[i].AdminSaved = saved[strings.ToLower(result.Players[i].UserID)]
	}
	respond(w, 200, result, nil)
}

func (s *Server) handlePlayerAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if query(r, "room") == "" {
		respond(w, 200, nil, fmt.Errorf("请明确指定要操作的房间"))
		return
	}
	var action server.PlayerAction
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&action); err != nil {
		respond(w, 200, nil, fmt.Errorf("玩家操作参数无效: %w", err))
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		respond(w, 200, nil, fmt.Errorf("玩家请求只能包含一份参数"))
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
	if err := s.mgr.ActOnPlayer(r.Context(), rm.Key(), action); err != nil {
		respond(w, 200, nil, err)
		return
	}
	respond(w, 200, map[string]bool{"executed": true}, nil)
}
