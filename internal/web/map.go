package web

import (
	"fmt"
	"net/http"
	"path/filepath"

	"dst-windows/internal/worldsettings"
)

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	shard := query(r, "shard")
	found := false
	for _, item := range rm.Shards {
		if item.Name == shard {
			found = true
			break
		}
	}
	if !found {
		respond(w, 200, nil, fmt.Errorf("请选择有效的分片"))
		return
	}
	if !s.mapMu.TryLock() {
		respond(w, 200, nil, fmt.Errorf("地图正在读取，请稍后重试"))
		return
	}
	defer s.mapMu.Unlock()
	data, err := worldsettings.ReadSavedMap(filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), shard))
	if err == nil && data.Terrain == nil {
		err = fmt.Errorf("此保存点缺少可读取的地块数据")
	}
	if err == nil {
		data.Textures = s.availableMapTextures(data)
		data.Icons = s.availableMapIcons(data)
	}
	respond(w, 200, data, err)
}
