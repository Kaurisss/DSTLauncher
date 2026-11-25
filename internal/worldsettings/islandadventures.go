// Package worldsettings 的岛屿冒险（Island Adventures）支持。
//
// 模组自带的世界配置项与位置默认值由 tools/extract_ia_worldsettings.py 从模组源码提取，
// 与游戏自身的 options.json 合并成同一份视图：海难、火山各自有独立的选项集合、
// 默认值和预设，不能套用原版 forest/cave 的清单。
//
// 数值一律来自提取结果，这里不做任何猜测——缺数据时宁可少给选项，也不编一个默认值。
package worldsettings

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
)

//go:embed island-adventures.json
var rawIA []byte

// 岛屿冒险的世界位置。与游戏里的 location 字段一致。
const (
	LocationShipwrecked = "shipwrecked"
	LocationVolcano     = "volcanoworld"
)

// iaItem 是模组注册的一个配置项。
type iaItem struct {
	Name     string  `json:"name"`
	Label    string  `json:"label,omitempty"`
	Default  string  `json:"default"`
	Order    int     `json:"order,omitempty"`
	Category string  `json:"category"`
	Group    string  `json:"group"`
	Values   []Value `json:"values,omitempty"`
}

// iaValue 是带注册位置的可选值。运行时取值项靠它区分「这个值属哪个世界」，
// 普通的 Value 只有 data + label，无法表达这一点。
type iaValue struct {
	Data     string `json:"data"`
	Label    string `json:"label,omitempty"`
	Location string `json:"location,omitempty"`
}

// IALocation 是某个世界位置（海难/火山）的完整配置视图来源。
type IALocation struct {
	Label    string              `json:"label"`
	Vanilla  []string            `json:"vanilla"`  // 模组允许在该位置使用的原版选项
	Defaults map[string]string   `json:"defaults"` // 位置级默认值，覆盖选项自身默认值
	Presets  map[string][]Preset `json:"presets"`  // settings / worldgen
	Items    []iaItem            `json:"items"`

	// 世界规则页的显示顺序（按游戏内页面截图）。
	// 原版采纳项与模组新增项合并后按这两张表排，否则两类会各排一段。
	SettingsGroupOrder []string            `json:"settingsGroupOrder"`
	SettingsOrder      map[string][]string `json:"settingsOrder"`

	// 世界生成页的同样两张表。
	WorldgenGroupOrder []string            `json:"worldgenGroupOrder"`
	WorldgenOrder      map[string][]string `json:"worldgenOrder"`

	// 仅当「火山岛」开启时才显示的项。模组用 HasVolcanoIsland() 控制显隐，
	// 当前值 volcanoisland="none" 时整组不列出来。
	VolcanoIslandOnly []string `json:"volcanoIslandOnly"`

	// 运行时取值项（task_set / start_location）的可选值，从注册点推导而来。
	// 它们是原版采纳项，不在 Items 里，需要单独合并。
	DynamicValues map[string][]iaValue `json:"dynamicValues"`
}

type iaDataset struct {
	Version     int    `json:"version"`
	GameVersion string `json:"gameVersion"`
	Source      string `json:"source"`
	// 游戏 + 模组注册的全部运行时取值（含 location）：
	// 原版森林/洞穴也要从这里按位置筛选，不能只看模组位置。
	DynamicValues    map[string][]iaValue  `json:"dynamicValues"`
	MasterControlled []string              `json:"masterControlled"`
	Locations        map[string]IALocation `json:"locations"`
}

var (
	iaOnce sync.Once
	iaErr  error
	iaData iaDataset
)

// LoadIA 解析内嵌的岛屿冒险清单（只做一次）。
func LoadIA() error {
	iaOnce.Do(func() {
		if err := json.Unmarshal(rawIA, &iaData); err != nil {
			iaErr = fmt.Errorf("解析内置岛屿冒险清单失败: %w", err)
		}
	})
	return iaErr
}

// IsIALocation 判断世界位置是否由岛屿冒险提供。
func IsIALocation(location string) bool {
	if err := LoadIA(); err != nil {
		return false
	}
	_, ok := iaData.Locations[location]
	return ok
}

// IAFallbackPresets 返回该位置在没有任何可选预设时的默认预设，供导入与新建使用。
func IAFallbackPresets(location string) (settings, worldgen string) {
	if err := LoadIA(); err != nil {
		return "", ""
	}
	loc, ok := iaData.Locations[location]
	if !ok {
		return "", ""
	}
	pick := func(list []Preset) string {
		if len(list) == 0 {
			return ""
		}
		return list[0].Data
	}
	return pick(loc.Presets[CategorySettings]), pick(loc.Presets[CategoryWorldGen])
}

// DynamicValues 返回运行时取值项在该世界类型的可选值。
//
// task_set / start_location 在 customize.lua 里没有 desc 表，取值来自
// AddTaskSet / AddStartLocation 的注册点。筛选依据是注册时声明的 location：
// 模组世界（海难/火山）另收 location 为空的通用值；原版世界只收精确匹配的。
// toValues 把带位置的可选值转成普通 Value。
func toValues(src []iaValue) []Value {
	out := make([]Value, 0, len(src))
	for _, v := range src {
		out = append(out, Value{Data: v.Data, Label: v.Label})
	}
	return out
}

func DynamicValues(location, name string) []Value {
	if err := LoadIA(); err != nil {
		return nil
	}
	out := []Value{}
	seen := map[string]bool{}
	for _, v := range iaData.DynamicValues[name] {
		match := v.Location == location
		if IsIALocation(location) && v.Location == "" {
			match = true // 通用值对任何位置都适用
		}
		if !match || seen[v.Data] {
			continue
		}
		seen[v.Data] = true
		out = append(out, Value{Data: v.Data, Label: v.Label})
	}
	return out
}

// FillDynamicValues 给运行时取值的选项（task_set / start_location）填上可选值。
//
// 这些项在 customize.lua 里没有 desc 表，游戏在界面上是列出一批预设让玩家翻的；
// 不填的话面板只能给个文本框，用户无从知道该写什么。
func FillDynamicValues(groups []Group, location string) []Group {
	if err := LoadIA(); err != nil {
		return groups
	}
	for gi := range groups {
		for oi := range groups[gi].Options {
			o := &groups[gi].Options[oi]
			if !o.Dynamic {
				continue
			}
			if vals := DynamicValues(location, o.Name); len(vals) > 0 {
				o.Values = vals
				o.Dynamic = false
			}
		}
	}
	return groups
}

// IAPresets 返回岛屿冒险位置的预设清单，交给前端渲染下拉框。
func IAPresets(location, category string) []Preset {
	if err := LoadIA(); err != nil {
		return nil
	}
	loc, ok := iaData.Locations[location]
	if !ok {
		return nil
	}
	out := make([]Preset, len(loc.Presets[category]))
	copy(out, loc.Presets[category])
	return out
}

// IADefault 返回某个选项在该位置的默认值；没有位置级默认值时返回空串。
func IADefault(location, name string) (string, bool) {
	if err := LoadIA(); err != nil {
		return "", false
	}
	loc, ok := iaData.Locations[location]
	if !ok {
		return "", false
	}
	v, ok := loc.Defaults[name]
	return v, ok
}

// IAGroups 返回该位置可用的分组（原版采纳项 + 模组新增项）。
//
// 原版选项直接复用 options.json 的定义（标签、可选值、分组），只把默认值换成
// 位置默认值——模组自己的位置定义会覆盖原版默认，例如火山默认只有小地图、
// 关船、关陷阱。分组名与顺序也沿用原版，保证两个世界页看着一致。
//
// isMaster 来自分片定义，不能由世界位置猜测。候选项完整返回，
// 火山岛开关的即时显隐由前端按 IAVolcanoIslandOnly 名单处理。
func IAGroups(location string, isMaster bool) []Group {
	if err := LoadIA(); err != nil {
		return nil
	}
	if err := Load(); err != nil {
		return nil
	}
	loc, ok := iaData.Locations[location]
	if !ok {
		return nil
	}

	hidden := map[string]bool{}
	if !isMaster {
		for _, name := range iaData.MasterControlled {
			hidden[name] = true
		}
	}

	// 按分组收集：原版项的 group 来自 options.json，模组项自带 group 字段。
	collected := map[string]*Group{}
	order := []string{}
	ensure := func(id, category string) *Group {
		key := category + "/" + id
		if g, ok := collected[key]; ok {
			return g
		}
		g := &Group{ID: id, Category: category}
		// 同一个分组 id 在原版里已有中文名与排序，直接沿用
		for _, vg := range dataset.Groups {
			if vg.ID == id && vg.Category == category {
				g.Label, g.Order = vg.Label, vg.Order
				break
			}
		}
		collected[key] = g
		order = append(order, key)
		return g
	}

	addVanilla := func(name string) {
		if hidden[name] {
			return
		}
		opt, ok := Lookup(name)
		if !ok {
			return // 游戏版本变动导致清单缺失：跳过而不是编造
		}
		gid, gcat := GroupOf(name)
		if gid == "" {
			return
		}
		if v, ok := loc.Defaults[name]; ok {
			opt.Default = v
		}
		// 运行时取值项：用注册点推导出的可选值，就不再是自由输入了。
		if vals, ok := loc.DynamicValues[name]; ok && len(vals) > 0 {
			opt.Values = toValues(vals)
			opt.Dynamic = false
		}
		g := ensure(gid, gcat)
		g.Options = append(g.Options, opt)
	}
	for _, name := range loc.Vanilla {
		addVanilla(name)
	}

	for _, item := range loc.Items {
		if hidden[item.Name] {
			continue
		}
		g := ensure(item.Group, item.Category)
		opt := Option{
			Name: item.Name, Label: item.Label, Default: item.Default,
			Order: item.Order, Values: item.Values, Category: item.Category,
		}
		if v, ok := loc.Defaults[item.Name]; ok {
			opt.Default = v
		}
		if item.Name == "task_set" || item.Name == "start_location" {
			opt.Dynamic = true
		}
		if vals, ok := loc.DynamicValues[item.Name]; ok && len(vals) > 0 {
			opt.Values = toValues(vals)
			opt.Dynamic = false
		}
		// 模组项的适用位置由提取结果决定；这里不再二次过滤
		g.Options = append(g.Options, opt)
	}

	out := make([]Group, 0, len(order))
	for _, key := range order {
		g := collected[key]
		sortOptions(g.Options)
		// 组内顺序：按类别只取自己那张表。
		// 两张表的组 ID 会重名（misc / resources / monsters 都各有一份），
		// 不分类别取的话，世界规则组会被世界生成的顺序覆盖。
		seq := loc.SettingsOrder[g.ID]
		if g.Category == CategoryWorldGen {
			seq = loc.WorldgenOrder[g.ID]
		}
		if len(seq) > 0 {
			rank := make(map[string]int, len(seq))
			for i, name := range seq {
				rank[name] = i
			}
			slices.SortStableFunc(g.Options, func(a, b Option) int {
				ri, okI := rank[a.Name]
				rj, okJ := rank[b.Name]
				if okI != okJ {
					if okI {
						return -1 // 截图里有的排前面
					}
					return 1
				}
				if okI {
					if c := cmp.Compare(ri, rj); c != 0 {
						return c
					}
				}
				return cmp.Compare(a.Name, b.Name)
			})
		}
		if len(g.Options) > 0 {
			out = append(out, *g)
		}
	}

	// 分组顺序：世界规则用 settingsGroupOrder，世界生成用 worldgenGroupOrder，
	// 没有指定的（如火山）保持原版的 order。
	groupRank := make(map[string]int, len(loc.SettingsGroupOrder))
	for i, id := range loc.SettingsGroupOrder {
		groupRank[id] = i
	}
	worldgenRank := make(map[string]int, len(loc.WorldgenGroupOrder))
	for i, id := range loc.WorldgenGroupOrder {
		worldgenRank[id] = i
	}
	slices.SortStableFunc(out, func(a, b Group) int {
		if c := cmp.Compare(a.Category, b.Category); c != 0 {
			return c
		}
		// 世界规则用 settingsGroupOrder，世界生成用 worldgenGroupOrder
		rank := groupRank
		if a.Category == CategoryWorldGen && len(loc.WorldgenGroupOrder) > 0 {
			rank = worldgenRank
		}
		ri, okI := rank[a.ID]
		rj, okJ := rank[b.ID]
		if okI != okJ {
			if okI {
				return -1
			}
			return 1
		}
		if okI {
			if c := cmp.Compare(ri, rj); c != 0 {
				return c
			}
		}
		return cmp.Or(
			cmp.Compare(a.Order, b.Order),
			cmp.Compare(a.ID, b.ID),
		)
	})
	return out
}

// IAVolcanoIslandOnly 返回由火山岛开关控制的候选项，供界面即时显隐。
func IAVolcanoIslandOnly(location string) []string {
	if err := LoadIA(); err != nil {
		return nil
	}
	return append([]string(nil), iaData.Locations[location].VolcanoIslandOnly...)
}

// IACategoryOf 返回该位置某个选项的类别；不存在时返回空串。
//
// 存档保存用它判断某个键能不能改：世界生成类只在建图时生效，改了对已有存档无意义。
func IACategoryOf(location, name string) string {
	if err := LoadIA(); err != nil {
		return ""
	}
	loc, ok := iaData.Locations[location]
	if !ok {
		return CategoryOf(name)
	}
	for _, item := range loc.Items {
		if item.Name == name {
			return item.Category
		}
	}
	for _, adopted := range loc.Vanilla {
		if adopted == name {
			return CategoryOf(name)
		}
	}
	return CategoryOf(name)
}

// sortOptions 按游戏内顺序排选项：先看显式 order，再看名字。
func sortOptions(opts []Option) {
	slices.SortStableFunc(opts, func(a, b Option) int {
		return cmp.Or(
			cmp.Compare(a.Order, b.Order),
			cmp.Compare(a.Name, b.Name),
		)
	})
}
