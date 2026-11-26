package web

import (
	"cmp"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/worldsettings"
)

// ---- 世界配置（worldgenoverride.lua）----

// WorldConfigView 是返回给前端的世界配置视图。
type WorldConfigView struct {
	Shard   string `json:"shard"`
	File    string `json:"file"`
	Exists  bool   `json:"exists"` // 磁盘上是否已经有这个文件
	HasSave bool   `json:"hasSave"`

	// Location 是游戏内部的世界位置标识（forest / cave / shipwrecked / volcanoworld）。
	// 取自 leveldataoverride.lua 的 location 字段，是权威来源；房间配置里的
	// WorldType 只是旧数据的兑底。
	Location string `json:"location"`
	// Mod 标明选项清单来自哪个模组适配（当前只有 island-adventures）。
	Mod string `json:"mod,omitempty"`
	// LevelDataFirst 表示这个分片用的是游戏自己维护的 leveldataoverride.lua：
	// 世界生成类选项在存档生成后不可改，只读展示。
	LevelDataSet bool `json:"levelDataInUse"`

	Config  *worldsettings.Override           `json:"config"`
	Presets map[string][]worldsettings.Preset `json:"presets"` // worldgen / settings
	Groups  []worldsettings.Group             `json:"groups"`
	// 完整候选项随 groups 返回，前端按草稿中的火山岛开关即时显隐。
	VolcanoIslandOnly []string `json:"volcanoIslandOnly,omitempty"`

	GameVersion string `json:"gameVersion"`
	GeneratedAt string `json:"generatedAt"`
}

type worldConfigInput struct {
	Room  int    `json:"room"`
	Shard string `json:"shard"`
	worldsettings.Override
}

// handleWorldConfig 读取或保存某个分片的世界配置。
//
// 两种分片走不同文件，判断依据是磁盘上有没有 leveldataoverride.lua：
//   - 有：这是客户端生成过世界的存档，游戏只认这个文件（它优先级更高、且要求完整世界定义），
//     所以在这里读写它，并且只能改 overrides 里的项；世界生成类选项存档后不再生效。
//   - 没有：沿用 worldgenoverride.lua，供新建房间在首次生成前配置世界。
func (s *Server) handleWorldConfig(w http.ResponseWriter, r *http.Request) {
	var in *worldConfigInput
	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		in = &worldConfigInput{}
		if err := decodeJSON(w, r, in, 1<<20); err != nil {
			respond(w, 200, nil, err)
			return
		}
		// 读取文件前统一目标；正文与查询参数冲突时拒绝，不能读一片写另一片。
		params := r.URL.Query()
		if in.Room != 0 {
			if raw := params.Get("room"); raw != "" && raw != strconv.Itoa(in.Room) {
				respond(w, 200, nil, fmt.Errorf("正文与查询参数的房间不一致"))
				return
			}
			params.Set("room", strconv.Itoa(in.Room))
		}
		if in.Shard != "" {
			if raw := params.Get("shard"); raw != "" && raw != in.Shard {
				respond(w, 200, nil, fmt.Errorf("正文与查询参数的分片不一致"))
				return
			}
			params.Set("shard", in.Shard)
		}
		r.URL.RawQuery = params.Encode()
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
	if !s.shardEnabled(rm, shard) {
		respond(w, 200, nil, fmt.Errorf("房间未启用分片 %s", shard))
		return
	}

	clusterDir := rm.ClusterDir(s.rootDir, s.confDir)
	// 世界类型以磁盘上的实际配置为准：旧房间配置可能还写着 forest，
	// 而实际分片是海难——按 rooms.json 过滤会掉走整批模组选项。
	location, _, err := worldsettings.LocationFromFiles(clusterDir, shard, rm.WorldTypeOf(shard))
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	ld, hasLevelData, err := worldsettings.ReadLevelData(clusterDir, shard)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	if hasLevelData {
		location = ld.Location()
	}

	if in != nil {
		s.saveWorldConfig(w, rm, shard, clusterDir, location, ld, hasLevelData, &in.Override)
		return
	}

	view := WorldConfigView{
		Shard:        shard,
		File:         worldsettings.FilePath(clusterDir, shard),
		HasSave:      rm.ShardHasSave(s.rootDir, s.confDir, shard),
		Location:     location,
		LevelDataSet: hasLevelData,
		GameVersion:  worldsettings.Version(),
		GeneratedAt:  worldsettings.GeneratedAt(),
	}

	if hasLevelData {
		view.File = worldsettings.LevelDataPath(clusterDir, shard)
		view.Exists = true
		view.Config = &worldsettings.Override{
			Enabled:   true,
			Overrides: ld.Overrides,
		}
	} else {
		ov, exists, err := worldsettings.ReadFile(clusterDir, shard, location)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		view.Exists, view.Config = exists, ov
	}

	// 预设与可选项按世界类型过滤，不按分片名：
	// 第三个世界叫什么都行，是 forest 还是 cave 只看实际的世界位置。
	if worldsettings.IsIALocation(location) {
		view.Mod = "island-adventures"
		definition, _ := rm.Shard(shard)
		view.Groups = worldsettings.IAGroups(location, definition.IsMaster)
		view.VolcanoIslandOnly = worldsettings.IAVolcanoIslandOnly(location)
		view.Presets = map[string][]worldsettings.Preset{
			worldsettings.CategoryWorldGen: worldsettings.IAPresets(location, worldsettings.CategoryWorldGen),
			worldsettings.CategorySettings: worldsettings.IAPresets(location, worldsettings.CategorySettings),
		}
	} else {
		view.Groups = worldsettings.Groups(location, "")
		// 原版世界的 task_set / start_location 同样是运行时取值，
		// 用注册点推导出的可选值填上，否则只能手填。
		view.Groups = worldsettings.FillDynamicValues(view.Groups, location)
		view.Presets = map[string][]worldsettings.Preset{
			worldsettings.CategoryWorldGen: worldsettings.Presets(location, worldsettings.CategoryWorldGen),
			worldsettings.CategorySettings: worldsettings.Presets(location, worldsettings.CategorySettings),
		}
	}

	respond(w, 200, view, nil)
}

func (s *Server) saveWorldConfig(
	w http.ResponseWriter, rm *room.Room, shard, clusterDir, loc string,
	ld *worldsettings.LevelData,
	ldOK bool,
	in *worldsettings.Override,
) {
	if ldOK {
		if err := in.Validate(); err != nil {
			respond(w, 200, nil, err)
			return
		}
		// 存档已经在用 leveldataoverride.lua：只允许改世界规则。
		// 世界生成类选项只在建图时读取，存档生成后改了也不会重建地图，
		// 写进去只会让人以为生效了。
		for name := range in.Overrides {
			if worldsettings.IACategoryOf(loc, name) == worldsettings.CategoryWorldGen {
				respond(w, 200, nil, fmt.Errorf(
					"%s 属于世界生成设置：存档已生成，这类选项只在建图时生效，面板不写入", name))
				return
			}
		}
		content, err := ld.WithOverrides(in.Overrides)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		if err := worldsettings.WriteLevelData(clusterDir, shard, content); err != nil {
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, map[string]interface{}{
			"file":        worldsettings.LevelDataPath(clusterDir, shard),
			"needRestart": s.mgr.AnyRunning(rm.Key()),
		}, nil)
		return
	}

	if rm.ShardHasSave(s.rootDir, s.confDir, shard) {
		current, _, err := worldsettings.ReadFile(clusterDir, shard, loc)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		if in.WorldGenPreset != current.WorldGenPreset {
			respond(w, 200, nil, fmt.Errorf("存档已生成，不能修改世界生成预设"))
			return
		}
		for name, value := range in.Overrides {
			if worldsettings.IACategoryOf(loc, name) != worldsettings.CategoryWorldGen {
				continue
			}
			previous, exists := current.Overrides[name]
			if !exists {
				if def, ok := worldsettings.IADefault(loc, name); ok {
					previous = def
				} else if option, ok := worldsettings.Lookup(name); ok {
					previous = option.Default
				}
			}
			if value != previous {
				respond(w, 200, nil, fmt.Errorf("%s 属于世界生成设置：存档已生成，不能修改", name))
				return
			}
		}
	}
	if err := worldsettings.UpdateFile(clusterDir, shard, in); err != nil {
		respond(w, 200, nil, err)
		return
	}

	respond(w, 200, map[string]interface{}{
		"file":   worldsettings.FilePath(clusterDir, shard),
		"config": in,
		// 游戏只在启动时读这个文件，运行中的分片不会感知到改动
		"needRestart": s.mgr.AnyRunning(rm.Key()),
	}, nil)
}

// shardEnabled 判断分片是否在该房间的启用范围内。
func (s *Server) shardEnabled(rm *room.Room, shard string) bool {
	for _, n := range rm.ShardNames() {
		if n == shard {
			return true
		}
	}
	return false
}

// ---- 配置预览 ----

// handleConfig 返回房间将生成的配置文件内容。
//
// 这里刻意只读：cluster.ini / server.ini 由房间表单统一生成，
// 如果额外允许直接改 ini，两边会互相覆盖，用户很难判断哪份才是最新的。
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respond(w, 200, nil, fmt.Errorf("配置由「房间」表单统一管理，请到房间页修改后保存"))
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}

	// 预览用未保存的最新生成结果；顺带带上文件是否已落盘
	files := rm.Files(s.rootDir, s.confDir)
	type fileView struct {
		Path     string `json:"path"`
		Content  string `json:"content"`
		OnDisk   bool   `json:"onDisk"`
		Redacted bool   `json:"redacted"`
	}
	out := make([]fileView, 0, len(files))
	for path, content := range files {
		// 令牌文件不回传明文：预览场景没有必要，降低误泄露风险
		redact := filepath.Base(path) == "cluster_token.txt"
		if redact {
			content = maskToken(content)
		}
		out = append(out, fileView{
			Path:     path,
			Content:  content,
			OnDisk:   config.FileExists(path),
			Redacted: redact,
		})
	}
	// 按路径排序，保证前端每次展示顺序一致
	slices.SortFunc(out, func(a, b fileView) int { return cmp.Compare(a.Path, b.Path) })

	respond(w, 200, map[string]interface{}{
		"clusterDir": rm.ClusterDir(s.rootDir, s.confDir),
		"files":      out,
	}, nil)
}

// maskToken 只保留令牌的首尾各 4 位，中间用星号代替。
func maskToken(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return "（未配置，在线模式下服务器无法注册到 Steam）"
	}
	if len(t) <= 12 {
		return strings.Repeat("*", len(t))
	}
	return t[:4] + strings.Repeat("*", 8) + t[len(t)-4:]
}
