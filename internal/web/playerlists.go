package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"dst-windows/internal/chatlog"
	"dst-windows/internal/room"
)

func (s *Server) handlePlayerLists(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, POST, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if query(r, "room") == "" {
		respond(w, 200, nil, fmt.Errorf("请明确指定名单所属房间"))
		return
	}
	kind := query(r, "kind")
	if _, err := room.PlayerListPath("", kind); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	clusterDir := rm.ClusterDir(s.rootDir, s.confDir)
	var ids []string
	switch r.Method {
	case http.MethodGet:
		ids, err = room.ReadPlayerList(clusterDir, kind)
	case http.MethodPost, http.MethodDelete:
		var input struct {
			KleiID string `json:"kleiId"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&input); err != nil {
			err = fmt.Errorf("名单参数无效: %w", err)
			break
		}
		if decoder.Decode(new(any)) != io.EOF {
			err = fmt.Errorf("名单请求只能包含一份参数")
			break
		}
		ids, err = room.ChangePlayerList(clusterDir, kind, input.KleiID, r.Method == http.MethodDelete)
		if err == nil && kind == room.WhiteList {
			// Refresh only cluster.ini. Retrying after a write failure is safe.
			err = room.RefreshClusterConfig(s.rootDir, s.confDir, rm)
			if err != nil {
				err = fmt.Errorf("名单已保存，预留位配置更新失败；请重试: %w", err)
			}
		}
	}
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	path, _ := room.PlayerListPath(clusterDir, kind)
	respond(w, 200, map[string]any{
		"ids": ids, "file": path, "needRestart": s.mgr.AnyRunning(rm.Key()),
		"reservedSlots": rm.ReservedSlots(clusterDir),
		"seen":          s.seenPlayers(clusterDir, rm.Shards),
	}, nil)
}

// seenPlayers matches Klei IDs to names and characters already stored in the room logs.
// shortcut: a bad history file hides names; the ID list still loads.
func (s *Server) seenPlayers(clusterDir string, shards []room.Shard) map[string]chatlog.Face {
	names := make([]string, 0, len(shards))
	for _, shard := range shards {
		if shard.Name != "" {
			names = append(names, shard.Name)
		}
	}
	faces, err := s.playerLogs.Faces(clusterDir, names)
	if err != nil || faces == nil {
		if err != nil {
			log.Printf("读取玩家身份: %v", err)
		}
		return map[string]chatlog.Face{}
	}
	return faces
}
