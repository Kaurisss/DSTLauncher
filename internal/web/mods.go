package web

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"dst-windows/internal/modmgr"
	"dst-windows/internal/panelcfg"
	"dst-windows/internal/room"
	"dst-windows/internal/workshop"
)

// ---- 模组 ----

// modsFor 按房间构造模组管理器。
// overrides 写在存档侧（集群目录），模组实体在游戏侧的共享 ugc 目录。
func (s *Server) modsFor(rm *room.Room, shards ...string) *modmgr.Manager {
	if len(shards) == 0 {
		shards = rm.ShardNames()
	}
	return modmgr.NewManager(
		rm.ClusterDir(s.rootDir, s.confDir),
		s.steam.GameDir(),
		s.mgr.UgcDir(),
		shards,
	)
}

// modShard accepts disabled shards for offline editing, but never arbitrary filesystem paths.
func modShard(r *http.Request, rm *room.Room) (string, error) {
	shard := shardOf(r, rm)
	if _, ok := rm.Shard(shard); !ok || !filepath.IsLocal(shard) || strings.ContainsAny(shard, `/\:`) || shard != strings.TrimRight(shard, ". ") {
		return "", fmt.Errorf("分片不存在: %s", shard)
	}
	return shard, nil
}

// SyncModsSetup 供面板启动时重算安装级的模组下载清单。
// 面板关闭期间房间配置可能被手工改动过，启动时按所有房间重算一次并集最稳妥。
func (s *Server) SyncModsSetup() { s.syncModsSetup() }

// ---- 模组更新检测 ----

// modUpdateView 是一个「本地有旧版」的工坊模组。
type modUpdateView struct {
	ID         string `json:"id"`   // workshop-<数字>
	Name       string `json:"name"` // 取自 modinfo.lua，读不到时为空
	LocalTime  int64  `json:"localTime"`
	RemoteTime int64  `json:"remoteTime"`
}

// handleModUpdates 检查当前房间启用的工坊模组有没有新版。
//
// 客户端提示「服务器有旧版本 mod」的根因就在这里：客户端订阅的是工坊最新版，
// 服务器本地还是旧版。判据用 acf 里记录的 timeupdated 对比远端 time_updated，
// 不依赖 modinfo.lua 的 version 字段——那是作者手填的，很多人根本不改。
func (s *Server) handleModUpdates(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}

	shard, err := modShard(r, rm)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	mods := s.modsFor(rm, shard)
	o, err := mods.LoadOverrides()
	if err != nil {
		respond(w, 200, nil, err)
		return
	}

	// 只查「已启用 + 工坊来源」的模组：本地自制 mod 没有远端可比
	ids := make([]string, 0, len(o.Order))
	seen := make(map[string]bool, len(o.Order))
	addID := func(id string) {
		if seen[id] || !strings.HasPrefix(id, "workshop-") {
			return
		}
		seen[id] = true
		ids = append(ids, strings.TrimPrefix(id, "workshop-"))
	}
	for _, id := range o.Order {
		if o.Enabled[id] {
			addID(id)
		}
	}
	// Order 是写入顺序，可能漏掉人工编辑过的条目，再扫一遍 Enabled 兜底
	for id, on := range o.Enabled {
		if on {
			addID(id)
		}
	}

	if len(ids) == 0 {
		respond(w, 200, map[string]interface{}{
			"checked":      0,
			"updates":      []modUpdateView{},
			"missing":      []string{},
			"notInstalled": []string{},
		}, nil)
		return
	}

	local, err := workshop.LoadInstalled(s.mgr.UgcDir())
	if err != nil {
		respond(w, 200, nil, err)
		return
	}

	items, err := workshop.Details(ids)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	byID := make(map[string]workshop.Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}

	updates := make([]modUpdateView, 0)
	missing := make([]string, 0)
	notInstalled := make([]string, 0)
	installed, _ := mods.InstalledMods()
	for _, id := range ids {
		key := "workshop-" + id
		if !installed[key] {
			// 已启用但本地没有文件：不需要比版本，直接下载
			notInstalled = append(notInstalled, key)
		}

		it, ok := byID[id]
		if !ok {
			// 工坊查不到：下架、ID 写错，或者远端只回了一部分
			missing = append(missing, key)
			continue
		}
		loc := local[id]
		if loc.TimeUpdated <= 0 {
			// acf 里没有记录（模组是手工拷进来的），无从判断新旧
			continue
		}
		if it.TimeUpdated > loc.TimeUpdated {
			name, _, _ := mods.ModInfo(key)
			updates = append(updates, modUpdateView{
				ID:         key,
				Name:       name,
				LocalTime:  loc.TimeUpdated,
				RemoteTime: it.TimeUpdated,
			})
		}
	}

	// 最近更新的排前面：最可能是客户端点名的那个
	slices.SortFunc(updates, func(a, b modUpdateView) int { return cmp.Compare(b.RemoteTime, a.RemoteTime) })

	respond(w, 200, map[string]interface{}{
		"checked":      len(ids),
		"updates":      updates,
		"missing":      missing,
		"notInstalled": notInstalled,
	}, nil)
}

// ---- 创意工坊（搜索 / 详情） ----

// installedSet 返回共享目录里已下载的模组集合。
func (s *Server) installedSet() map[string]bool {
	installed, err := modmgr.NewManager("", s.steam.GameDir(), s.mgr.UgcDir(), nil).InstalledMods()
	if err != nil {
		return map[string]bool{}
	}
	return installed
}

// handleWorkshopDetails 按 ID / 链接查询创意工坊模组详情（免 key）。
// input 可以是纯数字、workshop-数字、创意工坊链接，或它们的逗号分隔组合。
func (s *Server) handleWorkshopDetails(w http.ResponseWriter, r *http.Request) {
	input := query(r, "input")
	ids := workshop.NormalizeIDs(strings.Split(input, ","))
	if len(ids) == 0 {
		respond(w, 200, nil, fmt.Errorf("请输入创意工坊模组 ID 或链接"))
		return
	}
	items, err := workshop.Details(ids)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	if len(items) == 0 {
		respond(w, 200, nil, fmt.Errorf("没有查到对应的模组（ID 不存在或已从创意工坊下架）"))
		return
	}
	installed := s.installedSet()
	for i := range items {
		items[i].Installed = installed["workshop-"+items[i].ID]
	}
	respond(w, 200, items, nil)
}

// handleWorkshopSearch 按关键词搜索创意工坊。
// 需要 Steam Web API Key（panel.json 的 steamApiKey），没有时给出明确指引。
func (s *Server) handleWorkshopSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(query(r, "q"))
	if q == "" {
		respond(w, 200, nil, fmt.Errorf("请输入搜索关键词"))
		return
	}
	page, _ := strconv.Atoi(query(r, "page"))
	apiKey := panelcfg.Load(s.workDir).SteamApiKey
	if apiKey == "" {
		respond(w, 200, nil, fmt.Errorf(
			"关键词搜索需要 Steam Web API Key：点击下方「设置 API Key」填入后即可使用；不填也可以直接粘贴模组 ID 或链接查询"))
		return
	}
	items, err := workshop.Search(apiKey, q, page)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	installed := s.installedSet()
	for i := range items {
		items[i].Installed = installed["workshop-"+items[i].ID]
	}
	respond(w, 200, items, nil)
}

// handleWorkshopKey 查询 / 保存 Steam Web API Key（创意工坊搜索用）。
// 单独一个端点而不是并进「服务器位置」的设置接口，避免两处保存互相覆盖。
func (s *Server) handleWorkshopKey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, map[string]string{"steamApiKey": panelcfg.Load(s.workDir).SteamApiKey}, nil)

	case http.MethodPut:
		var req struct {
			SteamApiKey string `json:"steamApiKey"`
		}
		if err := decodeJSON(w, r, &req, 4096); err != nil {
			respond(w, 200, nil, err)
			return
		}
		cfg, err := s.loadPanelConfig()
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		cfg.SteamApiKey = strings.TrimSpace(req.SteamApiKey)
		if err := panelcfg.Save(s.workDir, cfg); err != nil {
			respond(w, 200, nil, fmt.Errorf("写入 %s 失败: %w", panelcfg.Path(s.workDir), err))
			return
		}
		respond(w, 200, map[string]string{"steamApiKey": cfg.SteamApiKey}, nil)

	default:
		respond(w, 200, nil, fmt.Errorf("不支持的方法: %s", r.Method))
	}
}

// SwapModsSetup 供单模组更新临时改写安装级下载清单：
// 只写目标模组一条，让引擎这一轮只下载它。
// 返回的 restore 在会话结束后恢复所有房间的并集。
// 必须成对使用——更新会话以 defer 保证 restore 一定执行。
func (s *Server) SwapModsSetup(target string) (func(), error) {
	num := strings.TrimPrefix(target, "workshop-")
	mods := modmgr.NewManager("", s.steam.GameDir(), s.mgr.UgcDir(), nil)
	if err := mods.WriteSetupLua([]string{num}); err != nil {
		return nil, err
	}
	return s.syncModsSetup, nil
}

// MissingModIDs 汇总所有房间「已启用但尚未下载」的工坊模组（去重）。
// 供模组更新循环判断进度：它只关心这个列表是否清零。
func (s *Server) MissingModIDs() ([]string, error) {
	if s.mgr.UgcDir() == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	for _, rm := range s.rooms.List() {
		missing, err := s.modsFor(rm).MissingMods()
		if err != nil {
			return nil, err
		}
		for _, id := range missing {
			seen[id] = true
		}
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

// syncModsSetup 重写安装级的 dedicated_server_mods_setup.lua。
//
// 服务器开服时按这个文件下载缺失的模组，而它是整个服务器只有一份的安装级文件，
// 内容必须是「所有房间启用模组的并集」。按单个房间写会互相覆盖：
// B 房间保存配置会把 A 房间要用的模组从清单里抹掉。
// 因此每次模组或房间发生增删改之后都要重算一次。
func (s *Server) syncModsSetup() {
	if s.mgr.UgcDir() == "" {
		return
	}
	seen := map[string]bool{}
	for _, rm := range s.rooms.List() {
		enabled, err := s.modsFor(rm).EnabledWorkshopIDs()
		if err != nil {
			fmt.Printf("[!] 读取房间「%s」的模组配置失败: %v\n", rm.Name, err)
			return
		}
		for _, id := range enabled {
			seen[id] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	// 不依赖任何具体房间：没有房间时也要能清空清单
	mods := modmgr.NewManager("", s.steam.GameDir(), s.mgr.UgcDir(), nil)
	if err := mods.WriteSetupLua(ids); err != nil {
		fmt.Printf("[!] 写入 dedicated_server_mods_setup.lua 失败: %v\n", err)
	}
}

type modStreamEvent struct {
	Type  string      `json:"type"`
	Total int         `json:"total,omitempty"`
	Mod   *modmgr.Mod `json:"mod,omitempty"`
}

func (s *Server) handleMods(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	shard, err := modShard(r, rm)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	mods := s.modsFor(rm, shard)

	installedDirs, err := mods.InstalledModDirs()
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	o, err := mods.LoadOverrides()
	if err != nil {
		respond(w, 200, nil, err)
		return
	}

	candidates := make([]string, 0, len(o.Order)+len(installedDirs))
	seen := make(map[string]bool, len(o.Order)+len(installedDirs))
	// overrides 中的顺序即用户期望的加载顺序
	for _, id := range o.Order {
		if seen[id] {
			continue
		}
		seen[id] = true
		candidates = append(candidates, id)
	}
	// 已下载但未在 overrides 中出现的模组按 ID 排序追加，保持列表稳定
	extra := make([]string, 0, len(installedDirs))
	for id := range installedDirs {
		if !seen[id] {
			seen[id] = true
			extra = append(extra, id)
		}
	}
	slices.Sort(extra)
	candidates = append(candidates, extra...)

	inspect := func(id string) *modmgr.Mod {
		dir, isInstalled := installedDirs[id]
		summary := mods.InspectMod(id, dir)
		if summary.ClientOnly {
			return nil
		}
		iconURL := ""
		if isInstalled && summary.IconVersion != "" {
			iconURL = "/api/mods/icon?" + url.Values{"room": {strconv.Itoa(rm.ID)}, "id": {id}, "v": {summary.IconVersion}}.Encode()
		}
		return &modmgr.Mod{
			ID:        id,
			Name:      summary.Name,
			Version:   summary.Version,
			IconURL:   iconURL,
			Enabled:   o.Enabled[id],
			Installed: isInstalled,
			Config:    o.Config[id],
			Priority:  o.Priority[id],
		}
	}

	streaming := r.URL.Query().Get("stream") == "1" || strings.Contains(r.Header.Get("Accept"), "application/x-ndjson")
	ctx := r.Context()

	type inspectedItem struct {
		index int
		mod   *modmgr.Mod
	}

	runOrdered := func(onMod func(*modmgr.Mod) bool) {
		n := len(candidates)
		if n == 0 {
			return
		}
		wctx, cancel := context.WithCancel(ctx)
		defer cancel()
		workers := min(8, n)
		jobs := make(chan int)
		results := make(chan inspectedItem, workers*2)

		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				for idx := range jobs {
					if wctx.Err() != nil {
						return
					}
					item := inspectedItem{index: idx, mod: inspect(candidates[idx])}
					select {
					case results <- item:
					case <-wctx.Done():
						return
					}
				}
			})
		}

		go func() {
			defer close(jobs)
			for i := range candidates {
				select {
				case jobs <- i:
				case <-wctx.Done():
					return
				}
			}
		}()

		go func() {
			wg.Wait()
			close(results)
		}()

		ready := make(map[int]*modmgr.Mod)
		next := 0
		for res := range results {
			ready[res.index] = res.mod
			for {
				mod, ok := ready[next]
				if !ok {
					break
				}
				delete(ready, next)
				next++
				if mod != nil && !onMod(mod) {
					return
				}
			}
		}
	}

	if !streaming {
		list := make([]*modmgr.Mod, 0, len(candidates))
		runOrdered(func(mod *modmgr.Mod) bool {
			list = append(list, mod)
			return true
		})
		respond(w, 200, list, nil)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	writeEvent := func(ev modStreamEvent, flush bool) bool {
		if ctx.Err() != nil {
			return false
		}
		if err := enc.Encode(ev); err != nil {
			return false
		}
		if flush && flusher != nil {
			flusher.Flush()
		}
		return true
	}

	if !writeEvent(modStreamEvent{Type: "meta", Total: len(candidates)}, true) {
		return
	}
	emitted := 0
	runOrdered(func(mod *modmgr.Mod) bool {
		emitted++
		return writeEvent(modStreamEvent{Type: "mod", Mod: mod}, true)
	})
	writeEvent(modStreamEvent{Type: "done", Total: emitted}, true)
}

func (s *Server) handleModToggle(w http.ResponseWriter, r *http.Request) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if err := s.busyTaskError(""); err != nil {
		respond(w, 200, nil, err)
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	shard, err := modShard(r, rm)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	if s.mgr.AnyRunning(rm.Key()) {
		respond(w, 200, nil, fmt.Errorf("房间正在运行或启停，请先停止全部分片再修改模组"))
		return
	}
	if err := s.modsFor(rm, shard).SetEnabled(query(r, "id"), query(r, "enabled") == "true"); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.syncModsSetup()
	respond(w, 200, nil, nil)
}

func (s *Server) handleModConfig(w http.ResponseWriter, r *http.Request) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if err := s.busyTaskError(""); err != nil {
		respond(w, 200, nil, err)
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	id, key := query(r, "id"), query(r, "key")
	shard, err := modShard(r, rm)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	if s.mgr.AnyRunning(rm.Key()) {
		respond(w, 200, nil, fmt.Errorf("房间正在运行或启停，请先停止全部分片再修改模组"))
		return
	}
	if id == "" || key == "" {
		respond(w, 200, nil, fmt.Errorf("缺少 id 或 key 参数"))
		return
	}
	// 前端以 JSON 字符串传递值，兼容数字与布尔
	var value interface{}
	raw := query(r, "value")
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		value = raw // 解析失败则按字符串处理
	}
	if err := s.modsFor(rm, shard).SetConfig(id, key, value); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.syncModsSetup()
	respond(w, 200, nil, nil)
}

func (s *Server) handleModSchema(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	schema, err := s.modsFor(rm).ConfigSchema(query(r, "id"))
	respond(w, 200, schema, err)
}

func (s *Server) handleModDelete(w http.ResponseWriter, r *http.Request) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if err := s.busyTaskError(""); err != nil {
		respond(w, 200, nil, err)
		return
	}
	if s.anyRunning() {
		respond(w, 200, nil, fmt.Errorf("还有分片在运行，请先全部停止再删除共享模组"))
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	shards := make([]string, 0, len(rm.Shards))
	for _, sh := range rm.Shards {
		shards = append(shards, sh.Name)
	}
	if err := s.modsFor(rm, shards...).DeleteMod(query(r, "id")); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.syncModsSetup()
	respond(w, 200, nil, nil)
}

// handleModsUpdate 触发或查询模组预更新。
//
// 更新是一个可能持续几十分钟的异步过程：POST 立即返回，前端轮询 GET 拿进度。
// 进度来自服务器输出里的百分比，完整输出一并返回供界面展示。
func (s *Server) handleModsUpdate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, s.mgr.ModsUpdateStatus(), nil)

	case http.MethodPost:
		s.taskMu.Lock()
		defer s.taskMu.Unlock()
		rm, err := s.roomByID(r)
		if err != nil {
			respond(w, 200, nil, err)
			return
		}
		// 服务端安装/更新途中不能下载模组：两者都写游戏安装目录
		if err := s.busyTaskError("mods"); err != nil {
			respond(w, 200, nil, err)
			return
		}
		if err := s.mgr.UpdateMods(rm.Key(), query(r, "shard"), query(r, "id")); err != nil {
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, s.mgr.ModsUpdateStatus(), nil)

	case http.MethodDelete:
		respond(w, 200, nil, s.mgr.StopModsUpdate())

	default:
		respond(w, 200, nil, fmt.Errorf("不支持的方法: %s", r.Method))
	}
}
