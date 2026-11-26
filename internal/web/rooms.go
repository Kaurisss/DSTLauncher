package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"dst-windows/internal/modmgr"
	"dst-windows/internal/room"
	"dst-windows/internal/server"
	"dst-windows/internal/worldsettings"
)

// ---- 房间 ----

// roomByID 解析 ?room= 参数。缺省时回退到第一个房间。
func (s *Server) roomByID(r *http.Request) (*room.Room, error) {
	raw := query(r, "room")
	list := s.rooms.List()
	if raw == "" {
		if len(list) == 0 {
			return nil, fmt.Errorf("还没有任何房间，请先在「房间」页新建")
		}
		return list[0], nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("房间参数不合法: %s", raw)
	}
	rm, ok := s.rooms.Get(id)
	if !ok {
		return nil, fmt.Errorf("房间不存在: %d", id)
	}
	return rm, nil
}

// roomView 是返回给前端的房间信息：房间配置 + 进程状态 + 存档情况。
type roomView struct {
	*room.Room
	Key       string          `json:"key"`
	Running   bool            `json:"running"`
	Statuses  []server.Status `json:"statuses"`
	HasSave   bool            `json:"hasSave"`
	NeedToken bool            `json:"needToken"`
	ModCount  int             `json:"modCount"`
	// Locations 是各分片存档里实际写着的世界位置（forest / cave /
	// shipwrecked / volcanoworld），键是分片名。
	//
	// 分片目录名叫 Cave 但里面可能是火山，所以界面上的分片名不能靠名字猜；
	// 也不写回 rooms.json：世界类型是存档的事实，不是面板的配置。
	Locations map[string]string `json:"locations,omitempty"`
}

func (s *Server) enabledModCount(rm *room.Room) int {
	clusterDir := rm.ClusterDir(s.rootDir, s.confDir)
	seen := make(map[string]struct{})
	for _, sh := range rm.Shards {
		if sh.Disabled {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(clusterDir, sh.Name, "modoverrides.lua"))
		if err != nil {
			continue
		}
		ov := modmgr.ParseOverrides(string(raw))
		for id, enabled := range ov.Enabled {
			if enabled {
				seen[id] = struct{}{}
			}
		}
	}
	return len(seen)
}

// shardLocations 读出每个分片的实际世界位置。
//
// 没生成过世界（没有 leveldataoverride.lua）分片不出现在结果里，
// 前端回落到房间配置的 worldType。
func (s *Server) shardLocations(rm *room.Room) map[string]string {
	out := make(map[string]string)
	clusterDir := rm.ClusterDir(s.rootDir, s.confDir)
	for _, sh := range rm.Shards {
		ld, ok, err := worldsettings.ReadLevelData(clusterDir, sh.Name)
		if err != nil || !ok {
			continue
		}
		if loc := ld.Location(); loc != "" {
			out[sh.Name] = loc
		}
	}
	return out
}

func (s *Server) roomViews() []roomView {
	list := s.rooms.List()
	out := make([]roomView, 0, len(list))
	for _, rm := range list {
		sts := s.mgr.Status(rm.Key())
		running := false
		for _, st := range sts {
			if st.State == "running" || st.State == "starting" || st.State == "stopping" {
				running = true
				break
			}
		}
		out = append(out, roomView{
			Room:      rm,
			Key:       rm.Key(),
			Running:   running,
			Statuses:  sts,
			HasSave:   rm.ExistsSave(s.rootDir, s.confDir),
			NeedToken: rm.NeedsToken() && !rm.HasToken(),
			ModCount:  s.enabledModCount(rm),
			Locations: s.shardLocations(rm),
		})
	}
	return out
}

// handleRooms 房间的查询 / 新建 / 修改。
func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, s.roomViews(), nil)

	case http.MethodPost:
		var rm room.Room
		if err := decodeJSON(w, r, &rm, 1<<20); err != nil {
			respond(w, 200, nil, err)
			return
		}
		created, err := s.rooms.Create(&rm)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		// 配置落盘失败不回收房间：用户可在面板上改好端口后重新保存
		if err := room.Generate(s.rootDir, s.confDir, created); err != nil {
			respond(w, 200, created, fmt.Errorf("房间已创建，但配置写入失败: %w", err))
			return
		}
		s.mgr.Register(server.NewCluster(created))
		// 新房间可能自带 modoverrides（存档上传场景），清单要重算并集
		s.syncModsSetup()
		respond(w, 200, created, nil)

	case http.MethodPut:
		var rm room.Room
		if err := decodeJSON(w, r, &rm, 1<<20); err != nil {
			respond(w, 200, nil, err)
			return
		}
		if rm.ID == 0 {
			respond(w, 200, nil, fmt.Errorf("缺少房间 ID"))
			return
		}
		updated, err := s.rooms.Update(&rm)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		if err := room.Generate(s.rootDir, s.confDir, updated); err != nil {
			respond(w, 200, updated, fmt.Errorf("房间已保存，但配置写入失败: %w", err))
			return
		}
		// 覆盖注册，让新的端口与分片列表立即生效
		s.mgr.Register(server.NewCluster(updated))
		s.syncModsSetup()
		// 运行中的进程不会重新读取 ini，必须重启才生效，这里如实告知
		if s.mgr.AnyRunning(updated.Key()) {
			respond(w, 200, map[string]interface{}{
				"room": updated,
				"hint": "配置已写入，但房间正在运行，需重启房间后生效",
			}, nil)
			return
		}
		respond(w, 200, updated, nil)

	case http.MethodDelete:
		rm, err := s.roomByID(r)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		if s.mgr.AnyRunning(rm.Key()) {
			respond(w, 200, nil, fmt.Errorf("房间正在运行，请先停止再删除"))
			return
		}
		hadSave := rm.ExistsSave(s.rootDir, s.confDir)
		if err := s.rooms.Delete(rm.ID); err != nil {
			respond(w, 200, nil, err)
			return
		}
		// 只注销进程层记录，保留磁盘上的存档目录——
		// 误删存档无法挽回，清理交给用户手工完成
		_ = s.mgr.Unregister(rm.Key(), 30*time.Second)
		// 被删房间启用的模组不再参与并集，清单要跟着收敛
		s.syncModsSetup()
		msg := "房间已删除"
		if hadSave {
			msg = fmt.Sprintf("房间已删除，世界存档仍保留在 %s", rm.ClusterDir(s.rootDir, s.confDir))
		}
		respond(w, 200, map[string]interface{}{"message": msg, "keptDir": hadSave}, nil)

	default:
		respond(w, 200, nil, fmt.Errorf("不支持的方法: %s", r.Method))
	}
}

// handleRoomMeta 返回表单需要的枚举值与默认值，避免前端硬编码。
func (s *Server) handleRoomMeta(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]interface{}{
		"gameModes":    room.ValidGameModes(),
		"languages":    room.ValidLanguages(),
		"tickRates":    []int{15, 30, 60},
		"defaults":     room.Defaults(),
		"roomsFile":    s.rooms.Path(),
		"panelVersion": s.version,
	}, nil)
}
