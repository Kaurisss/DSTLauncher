package worldsettings

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const maxMapBytes = 32 << 20

var worldSnapshotName = regexp.MustCompile(`^[0-9]+$`)

type MapPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type MapRegion struct {
	Name   string     `json:"name"`
	Points []MapPoint `json:"points"`
	Color  string     `json:"color,omitempty"`
}
type MapLandmark struct {
	MapPoint
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Target   string `json:"target,omitempty"`
	recordID int64
	targetID int64
}
type MapTileSpec struct {
	ID   uint16 `json:"id"`
	Name string `json:"name"`
}
type MapTerrain struct {
	Tiles string        `json:"tiles"`
	Specs []MapTileSpec `json:"specs"`
}
type SavedMap struct {
	Width     int           `json:"width"`
	Height    int           `json:"height"`
	SavedAt   time.Time     `json:"savedAt"`
	Snapshot  string        `json:"snapshot"`
	Kind      string        `json:"kind"`
	Regions   []MapRegion   `json:"regions"`
	Landmarks []MapLandmark `json:"landmarks"`
	Terrain   *MapTerrain   `json:"terrain,omitempty"`
	Textures  []string      `json:"textures,omitempty"`
	Icons     []string      `json:"icons,omitempty"`
}

var mapLandmarks = map[string]string{
	// 森林与洞穴（原版）
	"multiplayer_portal": "绚丽之门", "multiplayer_portal_moonrock": "天体传送门", "multiplayer_portal_moonrock_constr": "天体传送门",
	"pigking": "猪王", "resurrectionstone": "试金石", "wormhole": "虫洞",
	"cave_entrance": "洞穴入口", "cave_entrance_open": "洞穴入口", "cave_exit": "洞穴出口",
	"atrium_gate": "远古大门", "ancient_altar": "远古伪科学站", "ancient_altar_broken": "损坏的远古伪科学站", "archive_switch": "远古档案馆",
	"moonbase": "月台", "oasislake": "绿洲", "beequeenhivegrown": "蜂后巢", "hermithouse": "隐士之家",
	"monkeyisland_portal": "月亮码头传送门", "terrarium": "泰拉瑞亚",
	// 海难与火山（Island Adventures）
	"volcano": "火山入口", "volcano_exit": "火山出口", "volcano_altar": "火山祭坛", "obsidian_workbench": "黑曜石工作台",
	"octopusking": "章鱼王", "slotmachine": "老虎机", "sharkittenspawner": "虎鲨巢穴", "bermudatriangle": "百慕大三角",
	"seaworthy": "海之传送门", "skyworthy": "天空传送门", "packim_fishbone": "帕克鱼骨", "coral_brain_rock": "智慧树",
	"tar_pool": "焦油坑", "critterlab_water": "水上岩穴", "doydoy": "渡渡鸟", "doydoynest": "渡渡鸟巢",
	"woodlegs_cage": "木腿笼子", "kraken": "海妖", "buriedtreasure": "藏宝点", "mermhouse_fisher": "渔夫鱼人房",
	"daywalker_pillar_tropical": "噩梦疯猪柱",
}

// ReadSavedMap only examines direct numeric world files, never player subdirectories.
func ReadSavedMap(shardDir string) (*SavedMap, error) {
	root, err := os.OpenRoot(shardDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	sessions, err := fs.ReadDir(root.FS(), "save/session")
	if err != nil {
		return nil, fmt.Errorf("尚无世界保存点: %w", err)
	}
	var latest string
	var saved time.Time
	var latestNumber uint64
	for _, session := range sessions {
		if !session.IsDir() || session.Type()&os.ModeSymlink != 0 {
			continue
		}
		entries, err := fs.ReadDir(root.FS(), "save/session/"+session.Name())
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !worldSnapshotName.MatchString(entry.Name()) || !entry.Type().IsRegular() {
				continue
			}
			n, err := strconv.ParseUint(entry.Name(), 10, 64)
			if err != nil {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			name := "save/session/" + session.Name() + "/" + entry.Name()
			if latest == "" || info.ModTime().After(saved) || info.ModTime().Equal(saved) && (filepath.Dir(name) == filepath.Dir(latest) && n > latestNumber || filepath.Dir(name) > filepath.Dir(latest)) {
				latest, saved, latestNumber = name, info.ModTime(), n
			}
		}
	}
	if latest == "" {
		return nil, fmt.Errorf("尚无世界保存点，请先在游戏中保存")
	}
	f, err := root.Open(filepath.FromSlash(latest))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxMapBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxMapBytes {
		return nil, fmt.Errorf("世界快照超过 32 MiB 预览上限")
	}
	text, err := decodeMapSnapshot(b)
	if err != nil {
		return nil, err
	}
	result, err := parseSavedMap(text)
	if err != nil {
		return nil, err
	}
	result.SavedAt, result.Snapshot = saved, filepath.Base(latest)
	return result, nil
}
func decodeMapSnapshot(b []byte) (string, error) {
	const compressed = "KLEI     1D"
	const plain = "KLEI     1 "
	if bytes.HasPrefix(b, []byte(compressed)) {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b[len(compressed):])))
		if err != nil {
			return "", fmt.Errorf("世界快照 Base64 损坏: %w", err)
		}
		if len(raw) < 16 {
			return "", fmt.Errorf("世界快照压缩头不完整")
		}
		version, header, size, packed := binary.LittleEndian.Uint32(raw), binary.LittleEndian.Uint32(raw[4:]), binary.LittleEndian.Uint32(raw[8:]), binary.LittleEndian.Uint32(raw[12:])
		if version != 1 || header != 16 {
			return "", fmt.Errorf("不支持的世界快照压缩版本")
		}
		if size > maxMapBytes || uint64(packed) != uint64(len(raw)-16) {
			return "", fmt.Errorf("世界快照长度不符或超过 32 MiB")
		}
		input := bytes.NewReader(raw[16:])
		zr, err := zlib.NewReader(input)
		if err != nil {
			return "", err
		}
		decoded, err := io.ReadAll(io.LimitReader(zr, int64(size)+1))
		zr.Close()
		if err != nil {
			return "", err
		}
		if len(decoded) != int(size) || input.Len() != 0 {
			return "", fmt.Errorf("世界快照解压长度不符")
		}
		return string(decoded), nil
	}
	if bytes.HasPrefix(b, []byte(plain)) {
		return string(b[len(plain):]), nil
	}
	if bytes.HasPrefix(b, []byte("KLEI")) {
		return "", fmt.Errorf("不支持的 KLEI 世界快照编码")
	}
	return string(b), nil
}
func mapField(table *levelTable, name string) levelValue {
	if table != nil {
		for _, f := range table.fields {
			if f.name == name {
				return f.value
			}
		}
	}
	return levelValue{}
}
func mapNumber(v levelValue) (float64, error) {
	if v.kind != 'n' {
		return 0, fmt.Errorf("地图字段缺少数值")
	}
	n, err := strconv.ParseFloat(v.text, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("地图包含非法数值")
	}
	return n, nil
}
func mapPair(table *levelTable) (float64, float64, error) {
	if table == nil || len(table.fields) != 2 {
		return 0, 0, fmt.Errorf("地图区域坐标格式不受支持")
	}
	x, err := mapNumber(mapIndex(table, 1))
	if err != nil {
		return 0, 0, err
	}
	y, err := mapNumber(mapIndex(table, 2))
	return x, y, err
}

func mapIndex(table *levelTable, index int) levelValue {
	if table == nil {
		return levelValue{}
	}
	implicit := 0
	for _, f := range table.fields {
		if f.index != "" {
			n, err := strconv.ParseFloat(f.index, 64)
			if err == nil && n == float64(index) {
				return f.value
			}
		} else if f.name == "" {
			implicit++
			if implicit == index {
				return f.value
			}
		}
	}
	return levelValue{}
}
func parseSavedMap(text string) (*SavedMap, error) {
	root, err := scanSavedMapRoot(text)
	if err != nil {
		return nil, fmt.Errorf("世界快照不是静态数据: %w", err)
	}
	world := mapField(root, "map").table
	w, err := mapNumber(mapField(world, "width"))
	if err != nil {
		return nil, err
	}
	h, err := mapNumber(mapField(world, "height"))
	if err != nil {
		return nil, err
	}
	if w < 1 || h < 1 || w > 4096 || h > 4096 || math.Trunc(w) != w || math.Trunc(h) != h {
		return nil, fmt.Errorf("地图尺寸不合法")
	}
	result := &SavedMap{Width: int(w), Height: int(h), Kind: "regions", Regions: []MapRegion{}, Landmarks: []MapLandmark{}}
	if encoded := mapField(world, "tiles"); encoded.kind == 's' {
		tiles, names, err := decodeMapTiles(encoded.text, int(w), int(h), mapField(world, "world_tile_map").table)
		if err != nil {
			return nil, err
		}
		bytes := make([]byte, len(tiles)*2)
		for i, id := range tiles {
			binary.LittleEndian.PutUint16(bytes[2*i:], id)
		}
		specs := make([]MapTileSpec, 0, len(names))
		for id, name := range names {
			specs = append(specs, MapTileSpec{id, name})
		}
		slices.SortFunc(specs, func(a, b MapTileSpec) int { return cmp.Compare(a.ID, b.ID) })
		result.Terrain = &MapTerrain{Tiles: base64.StdEncoding.EncodeToString(bytes), Specs: specs}
		result.Kind = "terrain"
	}
	project := func(x, z float64) (MapPoint, error) {
		p := MapPoint{w/2 + x/4, h/2 + z/4}
		if math.Abs(p.X) > 8192 || math.Abs(p.Y) > 8192 {
			return p, fmt.Errorf("地图坐标超出预览范围")
		}
		return p, nil
	}
	topology := mapField(world, "topology").table
	nodes, ids, colors := mapField(topology, "nodes").table, mapField(topology, "ids").table, mapField(topology, "colours").table
	if nodes != nil && result.Terrain == nil {
		if len(nodes.fields) > 10000 {
			return nil, fmt.Errorf("地图区域数量超过上限")
		}
		for i, node := range nodes.fields {
			nodeIndex := i + 1
			if node.index != "" {
				n, err := strconv.Atoi(node.index)
				if err != nil || n < 1 || n > 100000 {
					return nil, fmt.Errorf("地图区域索引不合法")
				}
				nodeIndex = n
			}
			poly := mapField(node.value.table, "poly").table
			if poly == nil || len(poly.fields) < 3 {
				continue
			}
			if len(poly.fields) > 4096 {
				return nil, fmt.Errorf("地图区域顶点过多")
			}
			region := MapRegion{Name: fmt.Sprintf("区域 %d", i+1), Points: []MapPoint{}}
			if id := mapIndex(ids, nodeIndex); id.kind == 's' {
				region.Name = id.text
			}
			for _, point := range poly.fields {
				x, z, err := mapPair(point.value.table)
				if err != nil {
					return nil, err
				}
				p, err := project(x, z)
				if err != nil {
					return nil, err
				}
				region.Points = append(region.Points, p)
			}
			index := mapField(node.value.table, "c")
			if index.kind != 0 && colors != nil {
				c, err := mapNumber(index)
				if err != nil {
					return nil, err
				}
				if c < 1 || math.Trunc(c) != c || c > 100000 {
					return nil, fmt.Errorf("地图区域颜色索引不合法")
				}
				color := mapIndex(colors, int(c)).table
				channels := []float64{}
				byteColor := false
				if alpha := mapField(color, "a"); alpha.kind != 0 {
					a, err := mapNumber(alpha)
					if err != nil {
						return nil, err
					}
					if a < 0 || a > 255 {
						return nil, fmt.Errorf("地图区域透明度不合法")
					}
					byteColor = a > 1
				}
				for _, key := range []string{"r", "g", "b"} {
					v, err := mapNumber(mapField(color, key))
					if err != nil {
						return nil, err
					}
					if v < 0 || v > 255 {
						return nil, fmt.Errorf("地图区域颜色值不合法")
					}
					byteColor = byteColor || v > 1
					channels = append(channels, v)
				}
				factor := 255.0
				if byteColor {
					factor = 1
				}
				region.Color = fmt.Sprintf("#%02x%02x%02x", int(math.Round(channels[0]*factor)), int(math.Round(channels[1]*factor)), int(math.Round(channels[2]*factor)))
			}
			result.Regions = append(result.Regions, region)
		}
	}
	ents := mapField(root, "ents").table
	if ents != nil {
		for _, group := range ents.fields {
			name, ok := mapLandmarks[group.name]
			if !ok || group.value.table == nil {
				continue
			}
			for index, entity := range group.value.table.fields {
				x, err := mapNumber(mapField(entity.value.table, "x"))
				if err != nil {
					return nil, err
				}
				z, err := mapNumber(mapField(entity.value.table, "z"))
				if err != nil {
					return nil, err
				}
				p, err := project(x, z)
				if err != nil {
					return nil, err
				}
				landmark := MapLandmark{MapPoint: p, ID: fmt.Sprintf("%s:%d", group.name, index), Kind: group.name, Name: name}
				if group.name == "wormhole" || group.name == "bermudatriangle" {
					if id := mapField(entity.value.table, "id"); id.kind == 'n' {
						n, err := mapNumber(id)
						if err != nil {
							return nil, err
						}
						landmark.recordID = int64(n)
					}
					teleporter := mapField(mapField(entity.value.table, "data").table, "teleporter").table
					if target := mapField(teleporter, "target"); target.kind == 'n' {
						n, err := mapNumber(target)
						if err != nil {
							return nil, err
						}
						landmark.targetID = int64(n)
					}
				}
				result.Landmarks = append(result.Landmarks, landmark)
				if len(result.Landmarks) > 2000 {
					return nil, fmt.Errorf("地图地标过多")
				}
			}
		}
	}
	pairMapWormholes(result.Landmarks)
	if len(result.Regions) == 0 && result.Terrain == nil {
		return nil, fmt.Errorf("此保存点没有可预览的区域轮廓，暂不支持其地形流编码")
	}
	return result, nil
}

func pairMapWormholes(landmarks []MapLandmark) {
	pairLinkedLandmarks(landmarks, "wormhole", "虫洞")
	pairLinkedLandmarks(landmarks, "bermudatriangle", "百慕大三角")
}

func pairLinkedLandmarks(landmarks []MapLandmark, kind, label string) {
	indices := map[int64]int{}
	for i, point := range landmarks {
		if point.Kind == kind && point.recordID != 0 {
			indices[point.recordID] = i
		}
	}
	ids := slices.Sorted(maps.Keys(indices))
	pair := 0
	for _, id := range ids {
		i := indices[id]
		other, ok := indices[landmarks[i].targetID]
		if !ok || other == i || landmarks[other].targetID != id || landmarks[i].Target != "" {
			continue
		}
		pair++
		name := fmt.Sprintf("%s %d", label, pair)
		landmarks[i].Target, landmarks[other].Target = landmarks[other].ID, landmarks[i].ID
		landmarks[i].Name, landmarks[other].Name = name, name
	}
}
