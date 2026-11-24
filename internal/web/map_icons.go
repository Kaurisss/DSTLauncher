package web

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/png"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dst-windows/internal/modmgr"
	"dst-windows/internal/worldsettings"
)

// landmarkIconNames 按 DST scripts/prefabs/*.lua 中各预制体实际调用的 MiniMapEntity:SetIcon 映射图标名。
var landmarkIconNames = map[string][]string{
	"multiplayer_portal":                 {"portal_dst.png"},
	"multiplayer_portal_moonrock":        {"portal_dst.png"},
	"multiplayer_portal_moonrock_constr": {"portal_dst.png"},
	"pigking":                            {"pigking.png"},
	"resurrectionstone":                  {"resurrection_stone.png", "resurrect.png"},
	"wormhole":                           {"wormhole.png"},
	"cave_entrance":                      {"cave_closed.png", "cave_open.png"},
	"cave_entrance_open":                 {"cave_open.png"},
	"cave_exit":                          {"cave_open2.png", "cave_open.png"},
	"atrium_gate":                        {"atrium_gate.png"},
	"ancient_altar":                      {"tab_crafting_table.png"},
	"ancient_altar_broken":               {"tab_crafting_table.png"},
	"archive_switch":                     {"archive_power_switch.png"},
	"moonbase":                           {"moonbase.png"},
	"oasislake":                          {"oasis.png"},
	"beequeenhivegrown":                  {"beequeenhivegrown.png", "beequeenhive.png"},
	"hermithouse":                        {"hermitcrab_home2.png", "hermitcrab_home.png"},
	"monkeyisland_portal":                {"monkey_island_portal.png"},
	"terrarium":                          {"terrarium.png"},
	// 海难与火山（Island Adventures，来源于 ia_minimap.xml 与预制体 SetIcon）
	"volcano":                   {"volcano.tex", "volcano_active.tex"},
	"volcano_exit":              {"volcano_entrance.tex"},
	"volcano_altar":             {"volcano_altar.tex"},
	"obsidian_workbench":        {"obisidan_workbench.tex", "obsidian_workbench.tex"},
	"octopusking":               {"octopus.tex"},
	"slotmachine":               {"slot_machine.tex"},
	"sharkittenspawner":         {"sharkden.tex"},
	"bermudatriangle":           {"bermudatriangle.tex"},
	"seaworthy":                 {"shipwrecked_exit.tex", "seaworthy_kit.tex"},
	"skyworthy":                 {"portal_ham.tex", "skyworthy_kit.tex"},
	"packim_fishbone":           {"packim_fishbone.tex"},
	"coral_brain_rock":          {"coral_brain_rock.tex"},
	"tar_pool":                  {"tar.tex"},
	"critterlab_water":          {"critterlab_water.tex"},
	"doydoy":                    {"doydoynest.tex"},
	"doydoynest":                {"doydoynest.tex"},
	"woodlegs_cage":             {"woodlegs_cage.tex"},
	"kraken":                    {"quacken.tex"},
	"buriedtreasure":            {"xspot.tex"},
	"mermhouse_fisher":          {"mermhouse_fisher.tex"},
	"daywalker_pillar_tropical": {"daywalker_pillar_tropical.tex"},
}

type minimapAtlasXML struct {
	Texture struct {
		Filename string `xml:"filename,attr"`
	} `xml:"Texture"`
	Elements struct {
		Items []minimapAtlasElement `xml:"Element"`
	} `xml:"Elements"`
}

type minimapAtlasElement struct {
	Name string  `xml:"name,attr"`
	U1   float64 `xml:"u1,attr"`
	U2   float64 `xml:"u2,attr"`
	V1   float64 `xml:"v1,attr"`
	V2   float64 `xml:"v2,attr"`
}

type resolvedMapIcon struct {
	texPath string
	elem    minimapAtlasElement
	atlas   []minimapAtlasElement
}

func landmarkIconCandidates(kind string) []string {
	if names, ok := landmarkIconNames[kind]; ok {
		return names
	}
	lower := strings.ToLower(kind)
	return []string{lower + ".png", lower + ".tex"}
}

func minimapAtlasXMLPaths(roots []string) []string {
	var xmlPaths []string
	seen := map[string]bool{}
	add := func(p string) {
		clean := filepath.Clean(p)
		key := strings.ToLower(clean)
		if seen[key] {
			return
		}
		if info, err := os.Stat(clean); err == nil && info.Mode().IsRegular() {
			seen[key] = true
			xmlPaths = append(xmlPaths, clean)
		}
	}

	for _, root := range roots {
		// 原版 DST 小地图图集（minimap_data1..3.xml）
		for _, name := range []string{"minimap_data1.xml", "minimap_data2.xml", "minimap_data3.xml", "minimap_data.xml"} {
			add(filepath.Join(root, "minimap", name))
		}
		// 模组小地图图集（如 Island Adventures Core 的 images/minimap/ia_minimap.xml）
		modDir := filepath.Join(root, "images", "minimap")
		if entries, err := os.ReadDir(modDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".xml") {
					add(filepath.Join(modDir, e.Name()))
				}
			}
		}
	}
	return xmlPaths
}

func loadMinimapIconIndex(roots []string) map[string]resolvedMapIcon {
	index := map[string]resolvedMapIcon{}
	for _, xmlPath := range minimapAtlasXMLPaths(roots) {
		raw, err := os.ReadFile(xmlPath)
		if err != nil {
			continue
		}
		var doc minimapAtlasXML
		if err := xml.Unmarshal(raw, &doc); err != nil || doc.Texture.Filename == "" {
			continue
		}
		texPath := filepath.Join(filepath.Dir(xmlPath), filepath.Base(doc.Texture.Filename))
		info, err := os.Stat(texPath)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxMapTexFileBytes {
			continue
		}
		for _, elem := range doc.Elements.Items {
			key := strings.ToLower(elem.Name)
			if _, exists := index[key]; !exists {
				index[key] = resolvedMapIcon{
					texPath: texPath,
					elem:    elem,
					atlas:   doc.Elements.Items,
				}
			}
		}
	}
	return index
}

func lookupLandmarkIcon(index map[string]resolvedMapIcon, kind string) (resolvedMapIcon, bool) {
	for _, name := range landmarkIconCandidates(kind) {
		if hit, ok := index[strings.ToLower(name)]; ok {
			return hit, true
		}
	}
	return resolvedMapIcon{}, false
}

func (s *Server) getMinimapIconIndex() map[string]resolvedMapIcon {
	roots := s.mapTextureRoots()
	if len(roots) == 0 {
		return nil
	}
	key := strings.Join(roots, "|")
	s.mapIconMu.Lock()
	if s.mapIconIdxKey == key && s.mapIconIdx != nil {
		idx := s.mapIconIdx
		s.mapIconMu.Unlock()
		return idx
	}
	s.mapIconMu.Unlock()

	idx := loadMinimapIconIndex(roots)
	s.mapIconMu.Lock()
	s.mapIconIdxKey = key
	s.mapIconIdx = idx
	s.mapIconMu.Unlock()
	return idx
}

func (s *Server) availableMapIcons(data *worldsettings.SavedMap) []string {
	if data == nil || len(data.Landmarks) == 0 {
		return nil
	}
	index := s.getMinimapIconIndex()
	if len(index) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var available []string
	for _, lm := range data.Landmarks {
		if !validMapTileKey.MatchString(lm.Kind) || seen[lm.Kind] {
			continue
		}
		seen[lm.Kind] = true
		if hit, ok := lookupLandmarkIcon(index, lm.Kind); ok {
			if _, ok := s.loadCachedMapIcon(hit); !ok {
				continue
			}
			available = append(available, lm.Kind)
		}
	}
	sort.Strings(available)
	return available
}

func cropAtlasIconPNG(atlas *image.NRGBA, elem minimapAtlasElement) ([]byte, error) {
	b := atlas.Bounds()
	w, h := b.Dx(), b.Dy()
	x0 := max(0, min(w-1, int(math.Round(elem.U1*float64(w)))))
	x1 := max(x0+1, min(w, int(math.Round(elem.U2*float64(w)))))
	// DecodeTEX 已将 Klei 底部原点翻转成标准顶部原点，因此 v2 对应上边界、v1 对应下边界。
	y0 := max(0, min(h-1, int(math.Round((1.0-elem.V2)*float64(h)))))
	y1 := max(y0+1, min(h, int(math.Round((1.0-elem.V1)*float64(h)))))

	cw, ch := x1-x0, y1-y0
	crop := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	for y := 0; y < ch; y++ {
		srcOff := (y0+y)*atlas.Stride + x0*4
		dstOff := y * crop.Stride
		copy(crop.Pix[dstOff:dstOff+cw*4], atlas.Pix[srcOff:srcOff+cw*4])
	}
	crop = downsampleMapTexture(crop, 128)
	var buf bytes.Buffer
	if err := png.Encode(&buf, crop); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Server) handleMapIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	kind := query(r, "kind")
	if !validMapTileKey.MatchString(kind) {
		http.NotFound(w, r)
		return
	}
	index := s.getMinimapIconIndex()
	hit, ok := lookupLandmarkIcon(index, kind)
	if !ok {
		http.NotFound(w, r)
		return
	}

	encoded, ok := s.loadCachedMapIcon(hit)
	if !ok || len(encoded) == 0 {
		http.NotFound(w, r)
		return
	}
	writeMapTexturePNG(w, encoded)
}

func (s *Server) loadCachedMapIcon(hit resolvedMapIcon) ([]byte, bool) {
	cacheKey := hit.texPath + "|" + strings.ToLower(hit.elem.Name)
	s.mapIconMu.Lock()
	defer s.mapIconMu.Unlock()

	if cached := s.mapIconCache[cacheKey]; len(cached) > 0 {
		return cached, true
	}

	info, err := os.Stat(hit.texPath)
	if err != nil || info.Size() <= 0 || info.Size() > maxMapTexFileBytes {
		return nil, false
	}
	raw, err := os.ReadFile(hit.texPath)
	if err != nil {
		return nil, false
	}
	atlasImg, err := modmgr.DecodeTEX(raw)
	if err != nil {
		return nil, false
	}
	if s.mapIconCache == nil {
		s.mapIconCache = map[string][]byte{}
	}

	// 同图集上的已知地标图标顺便一次切完缓存，避免同一张 2048x2048 图集被重复解码。
	wantedOnAtlas := map[string]bool{strings.ToLower(hit.elem.Name): true}
	for _, names := range landmarkIconNames {
		for _, n := range names {
			wantedOnAtlas[strings.ToLower(n)] = true
		}
	}
	for _, elem := range hit.atlas {
		lower := strings.ToLower(elem.Name)
		if !wantedOnAtlas[lower] {
			continue
		}
		k := hit.texPath + "|" + lower
		if len(s.mapIconCache[k]) > 0 {
			continue
		}
		if encoded, err := cropAtlasIconPNG(atlasImg, elem); err == nil {
			s.mapIconCache[k] = encoded
		}
	}
	encoded := s.mapIconCache[cacheKey]
	return encoded, len(encoded) > 0
}
