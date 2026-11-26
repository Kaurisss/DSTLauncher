package web

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"dst-windows/internal/modmgr"
	"dst-windows/internal/worldsettings"
)

const (
	mapTexPaperKey      = "__PAPER__"
	mapTexOceanNoiseKey = "__OCEAN_NOISE__"
	mapTexEdgeKey       = "__MAP_EDGE__"
	maxMapTexSize       = 256
	maxMapTexFileBytes  = 16 << 20
	mapRootsCacheTTL    = 10 * time.Second
	maxMapTextureCache  = 256
	maxMapIconCache     = 512
)

var validMapTileKey = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// mapTileTextures 按 DST scripts/tiledefs.lua 与常见模组（如海难 Island Adventures）
// 将地块名映射到游戏内置的小地图 KTEX 材质相对路径。
var mapTileTextures = map[string][]string{
	mapTexPaperKey:      {"images/minimap_paper.tex"},
	mapTexOceanNoiseKey: {"levels/textures/ocean_noise.tex"},
	mapTexEdgeKey:       {"levels/tiles/map_edge.tex"},

	// 原版陆地与洞穴（来源于 scripts/tiledefs.lua）
	"ROAD":                {"levels/textures/mini_cobblestone_noise.tex"},
	"ROCKY":               {"levels/textures/mini_rocky_noise.tex"},
	"DIRT":                {"levels/textures/mini_dirt_noise.tex"},
	"SAVANNA":             {"levels/textures/mini_grass2_noise.tex"},
	"GRASS":               {"levels/textures/mini_grass_noise.tex"},
	"FOREST":              {"levels/textures/mini_forest_noise.tex"},
	"MARSH":               {"levels/textures/mini_marsh_noise.tex"},
	"LUNAR_MARSH":         {"levels/textures/mini_marsh_noise.tex"},
	"SHADOW_MARSH":        {"levels/textures/mini_marsh_noise.tex"},
	"DECIDUOUS":           {"levels/textures/mini_deciduous_noise.tex"},
	"DESERT_DIRT":         {"levels/textures/mini_desert_dirt_noise.tex"},
	"WOODFLOOR":           {"levels/textures/mini_woodfloor_noise.tex"},
	"ROPE_BRIDGE":         {"levels/textures/mini_woodfloor_noise.tex"},
	"WAGSTAFF_FLOOR":      {"levels/textures/mini_woodfloor_noise.tex"},
	"CARPET":              {"levels/textures/mini_carpet_noise.tex"},
	"CARPET2":             {"levels/textures/mini_carpet2_noise.tex"},
	"CHECKER":             {"levels/textures/mini_checker_noise.tex"},
	"SCALE":               {"levels/textures/mini_dragonfly_noise.tex"},
	"BEARD_RUG":           {"levels/textures/mini_beard_hair.tex"},
	"CAVE":                {"levels/textures/mini_cave_noise.tex"},
	"FUNGUS":              {"levels/textures/mini_fungus_noise.tex"},
	"FUNGUSRED":           {"levels/textures/mini_fungus_red_noise.tex"},
	"FUNGUSGREEN":         {"levels/textures/mini_fungus_green_noise.tex"},
	"FUNGUSMOON":          {"levels/textures/Ground_noise_moon_fungus_mini.tex"},
	"SINKHOLE":            {"levels/textures/mini_sinkhole_noise.tex"},
	"UNDERROCK":           {"levels/textures/mini_rock_noise.tex"},
	"MUD":                 {"levels/textures/mini_mud_noise.tex"},
	"VENT":                {"levels/textures/ground_noise_fumarole_mini.tex"},
	"ARCHIVE":             {"levels/textures/Ground_noise_archive_mini.tex"},
	"VAULT":               {"levels/textures/Ground_noise_vault_clean_mini.tex"},
	"VAULT_CLEAN":         {"levels/textures/Ground_noise_vault_clean_mini.tex"},
	"BRICK":               {"levels/textures/mini_ruinsbrick_noise.tex"},
	"BRICK_GLOW":          {"levels/textures/mini_ruinsbrick_noise.tex"},
	"TILES":               {"levels/textures/mini_ruinstile_noise.tex"},
	"TILES_GLOW":          {"levels/textures/mini_ruinstile_noise.tex"},
	"TRIM":                {"levels/textures/mini_ruinstrim_noise.tex"},
	"TRIM_GLOW":           {"levels/textures/mini_ruinstile_noise.tex"},
	"PEBBLEBEACH":         {"levels/textures/mini_pebblebeach.tex"},
	"MONKEY_GROUND":       {"levels/textures/mini_pebblebeach.tex"},
	"METEOR":              {"levels/textures/mini_meteor.tex"},
	"SHELLBEACH":          {"levels/textures/mini_shellbeach_noise.tex"},
	"MONKEY_DOCK":         {"levels/textures/mini_dock_noise.tex"},
	"OCEAN_ICE":           {"levels/textures/mini_oceanice_noise.tex"},
	"CHARLIE_VINE":        {"levels/textures/mini_grass2_noise.tex"},
	"COTL_BRICK":          {"levels/textures/ground_noise_cotl_brick_mini.tex"},
	"COTL_GOLD":           {"levels/textures/ground_noise_cotl_gold_mini.tex"},
	"MOSAIC_GREY":         {"levels/textures/mini_mosaictiles_grey_noise.tex"},
	"MOSAIC_RED":          {"levels/textures/mini_mosaictiles_red_noise.tex"},
	"MOSAIC_BLUE":         {"levels/textures/mini_mosaictiles_blue_noise.tex"},
	"RIFT_MOON":           {"levels/textures/Ground_noise_lunarrift_mini.tex"},
	"FARMING_SOIL":        {"levels/textures/quagmire_soil_mini.tex"},
	"QUAGMIRE_SOIL":       {"levels/textures/quagmire_soil_mini.tex"},
	"QUAGMIRE_GATEWAY":    {"levels/textures/quagmire_gateway_mini.tex"},
	"QUAGMIRE_CITYSTONE":  {"levels/textures/quagmire_citystone_mini.tex"},
	"QUAGMIRE_PARKFIELD":  {"levels/textures/quagmire_parkfield_mini.tex"},
	"QUAGMIRE_PARKSTONE":  {"levels/textures/quagmire_parkstone_mini.tex"},
	"QUAGMIRE_PEATFOREST": {"levels/textures/quagmire_peatforest_mini.tex"},
	"LAVAARENA_FLOOR":     {"levels/textures/lavaarena_floor_mini.tex"},
	"LAVAARENA_TRIM":      {"levels/textures/lavaarena_trim_mini.tex"},
	"OCEAN_COASTAL_SHORE": {"levels/textures/mini_water_shallow.tex"},
	"OCEAN_BRINEPOOL_SHORE": {
		"levels/textures/mini_water_coral.tex",
	},

	// Island Adventures（海难）模组地块
	"BEACH":                     {"levels/textures/mini_beach_noise.tex"},
	"MEADOW":                    {"levels/textures/mini_savannah_noise.tex"},
	"JUNGLE":                    {"levels/textures/mini_jungle_noise.tex"},
	"TIDALMARSH":                {"levels/textures/mini_tidalmarsh_noise.tex"},
	"SHADOW_TIDALMARSH":         {"levels/textures/mini_tidalmarsh_noise.tex"},
	"LUNAR_TIDALMARSH":          {"levels/textures/mini_tidalmarsh_noise.tex"},
	"MAGMAFIELD":                {"levels/textures/mini_magmafield_noise.tex"},
	"VOLCANO_ROCK":              {"levels/textures/mini_ground_volcano_noise.tex"},
	"VOLCANO":                   {"levels/textures/mini_ground_lava_rock.tex"},
	"ASH":                       {"levels/textures/mini_ash.tex"},
	"VOLCANO_LAVA":              {"levels/textures/mini_lava_noise.tex"},
	"MANGROVE":                  {"levels/textures/mini_water_mangrove.tex"},
	"MANGROVE_SHORE":            {"levels/textures/mini_water_mangrove.tex"},
	"OCEAN_SHALLOW":             {"levels/textures/mini_watershallow_noise.tex", "levels/textures/mini_water_shallow.tex"},
	"OCEAN_SHALLOW_SHORE":       {"levels/textures/mini_watershallow_noise.tex", "levels/textures/mini_water_shallow.tex"},
	"OCEAN_CORAL":               {"levels/textures/mini_water_coral.tex"},
	"OCEAN_CORAL_SHORE":         {"levels/textures/mini_water_coral.tex"},
	"OCEAN_MEDIUM":              {"levels/textures/mini_watermedium_noise.tex"},
	"OCEAN_MEDIUM_SHORE":        {"levels/textures/mini_watermedium_noise.tex"},
	"OCEAN_DEEP":                {"levels/textures/mini_waterdeep_noise.tex"},
	"OCEAN_DEEP_SHORE":          {"levels/textures/mini_waterdeep_noise.tex"},
	"OCEAN_SHIPGRAVEYARD":       {"levels/textures/mini_water_graveyard.tex"},
	"OCEAN_SHIPGRAVEYARD_SHORE": {"levels/textures/mini_water_graveyard.tex"},
	"SNAKESKIN":                 {"levels/textures/noise_snakeskinfloor.tex"},
}

func (s *Server) mapTextureRoots() []string {
	now := time.Now()
	s.mapRootsMu.Lock()
	if !s.mapRootsAt.IsZero() && now.Sub(s.mapRootsAt) < mapRootsCacheTTL {
		roots := append([]string(nil), s.mapRoots...)
		s.mapRootsMu.Unlock()
		return roots
	}
	roots := s.discoverMapTextureRoots()
	oldKey := strings.Join(s.mapRoots, "|")
	newKey := strings.Join(roots, "|")
	s.mapRoots = append([]string(nil), roots...)
	s.mapRootsAt = now
	s.mapRootsMu.Unlock()
	if oldKey != newKey {
		s.invalidateMapCaches()
	}
	return roots
}

func (s *Server) discoverMapTextureRoots() []string {
	var roots []string
	seen := map[string]bool{}
	add := func(dir string) {
		if dir == "" {
			return
		}
		clean := filepath.Clean(dir)
		key := strings.ToLower(clean)
		if seen[key] {
			return
		}
		if info, err := os.Stat(clean); err == nil && info.IsDir() {
			seen[key] = true
			roots = append(roots, clean)
		}
	}

	if s.steam != nil {
		if gameDir := s.steam.GameDir(); gameDir != "" {
			add(filepath.Join(gameDir, "data"))
			// 专用服务器目录有时裁剪了材质，若同级 Steam 库安装了正式客户端则只读借用其材质。
			add(filepath.Join(filepath.Dir(gameDir), "Don't Starve Together", "data"))
			// 同步扫描同级 Steam 创意工坊目录，兼容使用独立 ugc_mods 的部署环境
			steamWorkshop := filepath.Join(filepath.Dir(filepath.Dir(gameDir)), "workshop", "content", "322330")
			if entries, err := os.ReadDir(steamWorkshop); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						add(filepath.Join(steamWorkshop, e.Name()))
					}
				}
			}
			// 本地 mods 目录
			if entries, err := os.ReadDir(filepath.Join(gameDir, "mods")); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						add(filepath.Join(gameDir, "mods", e.Name()))
					}
				}
			}
		}
	}

	if s.mgr != nil {
		if ugcDir := s.mgr.UgcDir(); ugcDir != "" {
			contentDir := filepath.Join(ugcDir, "content", "322330")
			if entries, err := os.ReadDir(contentDir); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						add(filepath.Join(contentDir, e.Name()))
					}
				}
			}
		}
	}
	return roots
}

func mapTextureCandidates(tile string) []string {
	if paths, ok := mapTileTextures[tile]; ok {
		return paths
	}
	lower := strings.ToLower(tile)
	return []string{
		"levels/textures/mini_" + lower + "_noise.tex",
		"levels/textures/mini_" + lower + ".tex",
		"levels/textures/ground_noise_" + lower + "_mini.tex",
	}
}

func resolveMapTextureFile(roots []string, tile string) string {
	candidates := mapTextureCandidates(tile)
	for _, rel := range candidates {
		nativeRel := filepath.FromSlash(rel)
		for _, root := range roots {
			full := filepath.Join(root, nativeRel)
			if info, err := os.Stat(full); err == nil && info.Mode().IsRegular() {
				return full
			}
		}
	}
	return ""
}

func (s *Server) availableMapTextures(data *worldsettings.SavedMap) []string {
	if data == nil || data.Terrain == nil {
		return nil
	}
	roots := s.mapTextureRoots()
	if len(roots) == 0 {
		return nil
	}
	wanted := []string{mapTexPaperKey, mapTexOceanNoiseKey, mapTexEdgeKey}
	seen := map[string]bool{
		mapTexPaperKey:      true,
		mapTexOceanNoiseKey: true,
		mapTexEdgeKey:       true,
	}
	for _, spec := range data.Terrain.Specs {
		if validMapTileKey.MatchString(spec.Name) && !seen[spec.Name] {
			seen[spec.Name] = true
			wanted = append(wanted, spec.Name)
		}
	}
	var available []string
	for _, key := range wanted {
		if resolveMapTextureFile(roots, key) != "" {
			available = append(available, key)
		}
	}
	slices.Sort(available)
	return available
}

func (s *Server) handleMapTexture(w http.ResponseWriter, r *http.Request) {
	tile := query(r, "tile")
	if !validMapTileKey.MatchString(tile) {
		http.NotFound(w, r)
		return
	}
	roots := s.mapTextureRoots()
	for _, candidate := range mapTextureCandidates(tile) {
		for _, root := range roots {
			cacheKey := filepath.Join(root, filepath.FromSlash(candidate))
			s.mapTexMu.RLock()
			cached := s.mapTexCache[cacheKey]
			s.mapTexMu.RUnlock()
			if len(cached) > 0 {
				writeMapTexturePNG(w, cached)
				return
			}
		}
	}
	filePath := resolveMapTextureFile(roots, tile)
	if filePath == "" {
		http.NotFound(w, r)
		return
	}

	s.mapTexMu.RLock()
	cached := s.mapTexCache[filePath]
	s.mapTexMu.RUnlock()
	if len(cached) > 0 {
		writeMapTexturePNG(w, cached)
		return
	}

	info, err := os.Stat(filePath)
	if err != nil || info.Size() <= 0 || info.Size() > maxMapTexFileBytes {
		http.NotFound(w, r)
		return
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	img, err := modmgr.DecodeTEX(raw)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	img = downsampleMapTexture(img, maxMapTexSize)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "encode failed", http.StatusInternalServerError)
		return
	}
	encoded := buf.Bytes()

	s.mapTexMu.Lock()
	if s.mapTexCache == nil {
		s.mapTexCache = map[string][]byte{}
	}
	if len(s.mapTexCache) >= maxMapTextureCache {
		s.mapTexCache = map[string][]byte{}
	}
	s.mapTexCache[filePath] = encoded
	s.mapTexMu.Unlock()

	writeMapTexturePNG(w, encoded)
}

func (s *Server) invalidateMapCaches() {
	s.mapTexMu.Lock()
	s.mapTexCache = nil
	s.mapTexMu.Unlock()
	s.mapIconMu.Lock()
	s.mapIconCache = nil
	s.mapIconIdxKey = ""
	s.mapIconIdx = nil
	s.mapIconMu.Unlock()
}

func writeMapTexturePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// downsampleMapTexture 将超大材质（如 2048x2048 ocean_noise.tex）按盒式平均缩放到上限尺寸，
// 保持无缝平铺与色彩准确，同时将传输体积压到几十 KB。
func downsampleMapTexture(src *image.NRGBA, maxDim int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return src
	}
	scale := (max(w, h) + maxDim - 1) / maxDim
	if scale < 2 {
		scale = 2
	}
	nw, nh := max(1, w/scale), max(1, h/scale)
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy0 := y * scale
		sy1 := min(h, sy0+scale)
		for x := 0; x < nw; x++ {
			sx0 := x * scale
			sx1 := min(w, sx0+scale)
			var sr, sg, sb, sa, n uint64
			for sy := sy0; sy < sy1; sy++ {
				rowOff := sy * src.Stride
				for sx := sx0; sx < sx1; sx++ {
					off := rowOff + sx*4
					sr += uint64(src.Pix[off])
					sg += uint64(src.Pix[off+1])
					sb += uint64(src.Pix[off+2])
					sa += uint64(src.Pix[off+3])
					n++
				}
			}
			if n > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{
					R: uint8(sr / n),
					G: uint8(sg / n),
					B: uint8(sb / n),
					A: uint8(sa / n),
				})
			}
		}
	}
	return dst
}
