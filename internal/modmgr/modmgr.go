// Package modmgr 实现模组的搜索、下载、启用禁用与配置下发。
//
// 模组在 DST 中有两种来源：
//   - 创意工坊（UGC）：modoverrides.lua 中以 ["workshop-1234567890"] 形式引用
//   - 本地模组：直接从 ugc_mods / mods 目录读取
//
// 配置下发的核心是读写 modoverrides.lua，这是 Lua table 格式。
// modinfo.lua 只使用结构化文本处理和受限只读求值，避免执行外部操作或引入额外依赖。
package modmgr

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"dst-windows/internal/config"
	"dst-windows/internal/textutil"
)

// Mod 表示一个模组的完整信息。
type Mod struct {
	ID           string                 `json:"id"`         // workshop-1234567890
	WorkshopID   string                 `json:"workshopId"` // 纯数字 ID
	Name         string                 `json:"name"`
	Version      string                 `json:"version,omitempty"`
	IconURL      string                 `json:"iconUrl,omitempty"`
	Enabled      bool                   `json:"enabled"`
	Installed    bool                   `json:"installed"` // 本地是否已下载
	Config       map[string]interface{} `json:"config"`    // mod_config 配置项
	Priority     int                    `json:"priority"`
	DownloadedAt string                 `json:"downloadedAt"`
	Size         int64                  `json:"size"`
}

// Manager 管理模组目录与 overrides 文件。
//
// 涉及的路径分两类，务必区分：
//   - 存档侧：modoverrides.lua 位于 <root>/<confDir>/Cluster_N/<shard>/，
//     每个启用中的分片各一份（数量不限），启用/禁用需同步写入全部分片
//   - 游戏侧：模组实体在游戏安装目录，非 UGC 在 <game>/mods，
//     工坊模组（UGC）在 ugcDir —— 这是一个**跨房间共享**的目录，
//     由启动参数 -ugc_directory 指给服务器，所有房间与分片读同一份。
//     默认的按分片隔离布局（ugc_mods/<cluster>/<shard>）会让同一个模组
//     存 N 份、换房间还得搬文件，因此面板统一走共享布局。
type Manager struct {
	clusterDir string   // <root>/<confDir>/Cluster_N
	shards     []string // 启用中的分片名（数量不限）
	gameDir    string   // 游戏安装目录
	modsDir    string   // <game>/mods（非 UGC 模组 + mods_setup.lua）
	ugcDir     string   // 共享工坊模组目录（-ugc_directory 的值）
	setupLua   string   // <game>/mods/dedicated_server_mods_setup.lua
}

// NewManager 创建模组管理器。
// clusterDir 为集群存档目录，gameDir 为游戏安装目录（bin 的上一级），
// ugcDir 为跨房间共享的工坊模组目录（对应服务器的 -ugc_directory）。
func NewManager(clusterDir, gameDir, ugcDir string, shards []string) *Manager {
	return &Manager{
		clusterDir: clusterDir,
		shards:     shards,
		gameDir:    gameDir,
		modsDir:    filepath.Join(gameDir, "mods"),
		ugcDir:     ugcDir,
		setupLua:   filepath.Join(gameDir, "mods", "dedicated_server_mods_setup.lua"),
	}
}

// ModsDir 返回非 UGC 模组目录。
func (m *Manager) ModsDir() string { return m.modsDir }

// overridesPaths 返回所有分片的 modoverrides.lua 路径。
func (m *Manager) overridesPaths() []string {
	out := make([]string, 0, len(m.shards))
	for _, s := range m.shards {
		out = append(out, filepath.Join(m.clusterDir, s, "modoverrides.lua"))
	}
	return out
}

// LoadOverrides 读取单分片配置；多分片调用必须明确选择或使用 EnabledWorkshopIDs 并集。
func (m *Manager) LoadOverrides() (*Overrides, error) {
	paths := m.overridesPaths()
	if len(paths) > 1 {
		return nil, fmt.Errorf("请选择要读取模组配置的分片")
	}
	if len(paths) == 0 {
		return NewOverrides(), nil
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		if os.IsNotExist(err) {
			return NewOverrides(), nil
		}
		return nil, err
	}
	o, err := ParseOverridesChecked(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", paths[0], err)
	}
	return o, nil
}

// SaveOverrides 写入管理器明确指定的分片。交互编辑必须使用单分片管理器。
//
// 刻意**不**在这里同步 dedicated_server_mods_setup.lua：
// 那是整个服务器共享一份的安装级文件，必须写「所有房间启用模组的并集」。
// 只按当前房间的视角写会覆盖掉别的房间要用的模组（见 WriteSetupLua）。
func (m *Manager) SaveOverrides(o *Overrides) error {
	content := []byte(o.String())
	for _, p := range m.overridesPaths() {
		if err := config.WriteFileAtomic(p, content); err != nil {
			return err
		}
	}
	return nil
}

// EnabledWorkshopIDs 返回启用中的工坊模组 ID（去掉 workshop- 前缀，升序）。
// 下载清单与非工坊模组无关：ServerModSetup 只认创意工坊 ID。
func (o *Overrides) EnabledWorkshopIDs() []string {
	var ids []string
	for id, enabled := range o.Enabled {
		if !enabled || !strings.HasPrefix(id, "workshop-") {
			continue
		}
		ids = append(ids, strings.TrimPrefix(id, "workshop-"))
	}
	slices.Sort(ids)
	return ids
}

func (m *Manager) forShard(shard string) *Manager {
	single := *m
	single.shards = []string{shard}
	return &single
}

// EnabledWorkshopIDs 返回所选全部分片的启用模组并集，不合并各分片的配置参数。
func (m *Manager) EnabledWorkshopIDs() ([]string, error) {
	seen := map[string]bool{}
	for _, shard := range m.shards {
		o, err := m.forShard(shard).LoadOverrides()
		if err != nil {
			return nil, fmt.Errorf("读取 %s 模组配置失败: %w", shard, err)
		}
		for _, id := range o.EnabledWorkshopIDs() {
			seen[id] = true
		}
	}
	ids := slices.Sorted(maps.Keys(seen))
	return ids, nil
}

// WriteSetupLua 重写 dedicated_server_mods_setup.lua。
//
// 该文件是**安装级**的（整个服务器只有一份），服务器开服时会按它下载缺失的模组，
// 因此内容必须是所有房间启用模组的并集。按单个房间写会互相覆盖：
// B 房间保存配置会把 A 房间要用的模组从清单里抹掉。
// 传入空列表等于清空清单——这同样是合法状态（谁都没启用工坊模组）。
func (m *Manager) WriteSetupLua(ids []string) error {
	// 游戏尚未安装时跳过，避免在无意义的路径上创建文件
	if !dirExists(m.gameDir) {
		return nil
	}
	sorted := append([]string(nil), ids...)
	slices.Sort(sorted)

	var sb strings.Builder
	sb.WriteString("-- 由 dst-windows 自动生成，请勿手工编辑\n")
	sb.WriteString("-- 内容是所有房间启用模组的并集；DST 只会下载这里声明过的工坊模组\n")
	for _, id := range sorted {
		sb.WriteString(fmt.Sprintf("ServerModSetup(\"%s\")\n", id))
	}
	return config.WriteFileAtomic(m.setupLua, []byte(sb.String()))
}

// MissingMods 返回已启用但本地尚未下载的工坊模组 ID。
// 服务器以 -skip_update_server_mods 启动时不会自己补模组，
// 所以启动前必须用这份清单确认「要加载的都已经在本地」。
//
// 只统计 workshop- 前缀的条目：本地模组（目录名形式）无法下载，
// 只能由用户手工放进 mods 目录，不能因为它们缺失而拦住启动。
func (m *Manager) MissingMods() ([]string, error) {
	ids, err := m.EnabledWorkshopIDs()
	if err != nil {
		return nil, err
	}
	installed, err := m.InstalledMods()
	if err != nil {
		return nil, err
	}

	var missing []string
	for _, id := range ids {
		if !installed["workshop-"+id] {
			missing = append(missing, "workshop-"+id)
		}
	}
	return missing, nil
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// InstalledMods 扫描本地已下载的模组。
// 返回 map[modID]true，modID 统一为 workshop-<数字> 或本地目录名。
// InstalledMods 返回本地已下载的模组 ID 集合。
func (m *Manager) InstalledMods() (map[string]bool, error) {
	dirs, err := m.InstalledModDirs()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(dirs))
	for id := range dirs {
		out[id] = true
	}
	return out, nil
}

// InstalledModDirs 返回本地已下载模组 ID 到实体目录的映射，
// 目录优先级与 modDirCandidates 保持一致（共享 UGC 优先于旧目录）。
func (m *Manager) InstalledModDirs() (map[string]string, error) {
	out := make(map[string]string)

	// 非 UGC：<game>/mods/<name>
	if entries, err := os.ReadDir(m.modsDir); err == nil {
		workshopDirs := make(map[string][2]string)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			dir := filepath.Join(m.modsDir, name)
			if !fileExists(filepath.Join(dir, "modinfo.lua")) {
				continue
			}
			switch {
			case strings.HasPrefix(name, "workshop-"):
				id := name
				workshopDirs[id] = [2]string{dir, workshopDirs[id][1]}
			case strings.HasPrefix(name, "workshop_"):
				id := "workshop-" + strings.TrimPrefix(name, "workshop_")
				workshopDirs[id] = [2]string{workshopDirs[id][0], dir}
			default:
				// 自制/本地模组直接用目录名
				out[name] = dir
			}
		}
		for id, dirs := range workshopDirs {
			switch {
			case dirs[0] != "":
				out[id] = dirs[0]
			case dirs[1] != "":
				out[id] = dirs[1]
			}
		}
	}

	// UGC（共享布局）：后扫描的高优先级目录覆盖前者
	ugcBases := []string{
		// 兼容 SteamCMD 原始下载布局（steamapps/workshop/...）
		filepath.Join(m.ugcDir, "steamapps", "workshop", "content", "322330"),
		// 服务器写入的是 <ugcDir>/content/322330/<id>
		filepath.Join(m.ugcDir, "content", "322330"),
	}
	if m.ugcDir != "" {
		ugcBases = append([]string{m.ugcDir}, ugcBases...)
	}
	for _, base := range ugcBases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			id := e.Name()
			dir := filepath.Join(base, id)
			if e.IsDir() && id != "" && strings.Trim(id, "0123456789") == "" &&
				fileExists(filepath.Join(dir, "modinfo.lua")) {
				out["workshop-"+id] = dir
			}
		}
	}

	return out, nil
}

func (m *Manager) modDirCandidates(id string) []string {
	num := strings.TrimPrefix(id, "workshop-")
	return []string{
		// 共享 UGC 布局（服务器 -ugc_directory 指向的目录）
		filepath.Join(m.ugcDir, "content", "322330", num),
		filepath.Join(m.ugcDir, "steamapps", "workshop", "content", "322330", num),
		filepath.Join(m.ugcDir, num),
		// 非 UGC：<game>/mods/
		filepath.Join(m.modsDir, "workshop-"+num),
		filepath.Join(m.modsDir, "workshop_"+num),
		filepath.Join(m.modsDir, id),
		filepath.Join(m.modsDir, num),
	}
}

// findModDir 定位某个模组的实体目录，找不到返回空串。
// 依次尝试已知布局，最后做限深搜索兜底（兼容手工安装的目录结构）。
func (m *Manager) findModDir(id string) string {
	num := strings.TrimPrefix(id, "workshop-")
	for _, c := range m.modDirCandidates(id) {
		if fileExists(filepath.Join(c, "modinfo.lua")) {
			return c
		}
	}

	for _, root := range []string{m.modsDir, m.ugcDir} {
		if !dirExists(root) {
			continue
		}
		if p := searchModDir(root, id, num, 6); p != "" {
			return p
		}
	}
	return ""
}

// searchModDir 在 root 下限深搜索名字匹配且含 modinfo.lua 的目录。
func searchModDir(root, id, num string, maxDepth int) string {
	found := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !d.IsDir() || path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		// 超过深度限制就不再往下钻，避免在巨大的 ugc 目录里遍历过久
		if strings.Count(filepath.ToSlash(rel), "/") > maxDepth {
			return fs.SkipDir
		}
		name := d.Name()
		if name == "workshop-"+num || name == "workshop_"+num || name == num || name == id {
			if fileExists(filepath.Join(path, "modinfo.lua")) {
				found = path
			}
		}
		return nil
	})
	return found
}

// modInfoNameRe 匹配顶层的 name = 赋值行（捕获值部分）。
// OptionChoice 表示下拉配置项的一个候选值及其显示文案。
type OptionChoice struct {
	Data        string `json:"data"`
	Description string `json:"description,omitempty"`
}

// ConfigSchema 解析 modinfo.lua 中的 configuration_options 定义。
// 这是 mod 作者声明的可配置项清单，用于在 Web 面板上自动渲染表单。
type ConfigOption struct {
	Name              string         `json:"name"`
	Label             string         `json:"label"`
	Type              string         `json:"type"` // string / number / boolean / select
	Default           string         `json:"default"`
	DefaultUnresolved bool           `json:"defaultUnresolved,omitempty"`
	Options           []string       `json:"options,omitempty"` // type=select 时的候选值
	Choices           []OptionChoice `json:"choices,omitempty"` // 候选值及其中文描述
	Description       string         `json:"description"`
	Min               string         `json:"min,omitempty"`
	Max               string         `json:"max,omitempty"`
}

type configOptionBlock struct {
	Name        string
	Label       string
	Type        string
	Default     string
	Options     []string
	Choices     []OptionChoice
	Description string
	Min         string
	Max         string
}

var (
	configOptsAssignRe = regexp.MustCompile(`\bconfiguration_options\s*=`)
	localTableRe       = regexp.MustCompile(`(?m)^\s*local\s+(\w+)\s*=\s*\{`)
	fallbackOptsRe     = regexp.MustCompile(`configuration_options\[[^\]]+\]\.options\s*=\s*configuration_options\[[^\]]+\]\.options\s+or\s+(\w+)`)
	fallbackDefaultRe  = regexp.MustCompile(`configuration_options\[[^\]]+\]\.default\s*=\s*configuration_options\[[^\]]+\]\.default\s*==\s*nil\s+and\s+(true|false|-?[0-9.]+)`)
	literalValueRe     = regexp.MustCompile(`^(?:-?[0-9]+(?:\.[0-9]+)?|true|false)$`)
	numberLiteralRe    = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)
	zhEntryRe          = regexp.MustCompile(`(?:\["zh"\]|\['zh'\]|\bzh)\s*=\s*("[^"]*"|'[^']*')`)
)

// ConfigSchema 解析模组声明的配置项。
func (m *Manager) ConfigSchema(id string) ([]ConfigOption, error) {
	dir := m.findModDir(id)
	if dir == "" {
		return nil, fmt.Errorf("未找到模组目录: %s（该模组可能尚未下载）", id)
	}
	data, err := os.ReadFile(filepath.Join(dir, "modinfo.lua"))
	if err != nil {
		return nil, fmt.Errorf("读取 modinfo 失败: %w", err)
	}
	return parseConfigSchemaText(string(data))
}

func parseConfigSchemaText(raw string) ([]ConfigOption, error) {
	text := stripLuaComments(raw)
	options, dynamicHandled, dynamicErr := parseDynamicConfigSchema(text)
	if dynamicHandled && dynamicErr == nil {
		return options, nil
	}
	loc := findConfigOptionsAssignment(text)
	if loc == nil {
		if dynamicErr != nil {
			return nil, fmt.Errorf("动态解析 modinfo.lua 失败: %w", dynamicErr)
		}
		return nil, nil // 该模组无配置项
	}
	idx := loc[0]
	literalConfig := strings.HasPrefix(strings.TrimSpace(text[loc[1]:]), "{")
	start := strings.Index(text[loc[1]:], "{")
	if start < 0 {
		if dynamicErr != nil {
			return nil, fmt.Errorf("动态解析 modinfo.lua 失败: %w", dynamicErr)
		}
		return nil, nil
	}
	start += loc[1]
	end := matchBrace(text, start)
	if end < 0 {
		if dynamicErr != nil {
			return nil, fmt.Errorf("动态解析 modinfo.lua 失败: %w", dynamicErr)
		}
		return nil, fmt.Errorf("configuration_options 块未闭合，modinfo.lua 可能损坏")
	}

	block := text[start : end+1]
	// 双语模组常写成 `configuration_options = L and { ...英文... } or { ...中文... }`
	// 若紧跟着 or 分支且中文字符更多，取中文分支。
	rest := strings.TrimLeft(text[end+1:], " \t\r\n")
	if strings.HasPrefix(rest, "or") && (len(rest) == 2 || !isIdentChar(rest[2])) {
		if rel := strings.Index(rest, "{"); rel >= 0 {
			open2 := len(text) - len(rest) + rel
			if close2 := matchBrace(text, open2); close2 > open2 {
				alt := text[open2 : close2+1]
				if cjkCount(alt) > cjkCount(block) {
					block = alt
					end = close2
				}
			}
		}
	}

	// 收集顶层 `local xxx = { ... }` 候选表（如岛屿冒险的 options_enable）
	localOpts := map[string][]OptionChoice{}
	for _, m := range localTableRe.FindAllStringSubmatchIndex(text[:idx], -1) {
		open := m[1] - 1
		closeIdx := matchBrace(text, open)
		if closeIdx <= open {
			continue
		}
		if choices := extractChoices(text[open+1 : closeIdx]); len(choices) > 0 {
			localOpts[text[m[2]:m[3]]] = choices
		}
	}

	var fallbackChoices []OptionChoice
	if m := fallbackOptsRe.FindStringSubmatch(text[end+1:]); m != nil {
		fallbackChoices = localOpts[m[1]]
	}
	fallbackDefault := ""
	if m := fallbackDefaultRe.FindStringSubmatch(text[end+1:]); m != nil {
		fallbackDefault = m[1]
	}

	stringsBlock := extractBraceBlock(text[:idx], "STRINGS")

	// 每个配置项是一个独立表，按 depth=1 的边界切分
	var out []ConfigOption
	for _, item := range splitLuaTables(block[1 : len(block)-1]) {
		trimmed := strings.TrimSpace(item)
		if !strings.HasPrefix(trimmed, "{") {
			continue
		}
		opt := parseConfigOptionWithLocals(trimmed, localOpts, fallbackChoices, fallbackDefault)
		if opt.Name == "" {
			continue
		}
		// 兼容通过顶层 STRINGS[name] 注入多语言文案的模组（如 Insight）
		if opt.Label == "" && stringsBlock != "" {
			applyStringsTable(&opt, trimmed, stringsBlock)
		}
		// 跳过伪装成单空选项的分段标题（如 Title / EmptyNull）
		if len(opt.Choices) == 1 && opt.Choices[0].Description == "" {
			continue
		}
		out = append(out, opt)
	}
	if len(out) == 0 && dynamicErr != nil && !literalConfig {
		return nil, fmt.Errorf("动态解析 modinfo.lua 失败: %w", dynamicErr)
	}
	return out, nil
}

func applyStringsTable(opt *ConfigOption, rawItem, stringsBlock string) {
	entry := extractBraceBlock(stringsBlock, opt.Name)
	if entry == "" {
		return
	}
	if lbl := extractBraceBlock(entry, "label"); lbl != "" {
		opt.Label = textutil.Clean(evalTextExpr(lbl))
	}
	if opt.Description == "" {
		if hvr := extractBraceBlock(entry, "hover"); hvr != "" {
			opt.Description = textutil.Clean(evalTextExpr(hvr))
		}
	}
	if optsBlock := extractBraceBlock(entry, "options"); optsBlock != "" {
		for i := range opt.Choices {
			if opt.Choices[i].Description != "" {
				continue
			}
			key := regexp.QuoteMeta(opt.Choices[i].Data)
			re := regexp.MustCompile(`(?:\["` + key + `"\]|\['` + key + `'\]|\[` + key + `\]|\b` + key + `)\s*=\s*\{`)
			if loc := re.FindStringIndex(optsBlock); loc != nil {
				open := loc[1] - 1
				if closeIdx := matchBrace(optsBlock, open); closeIdx > open {
					if desc := extractBraceBlock(optsBlock[open+1:closeIdx], "description"); desc != "" {
						opt.Choices[i].Description = textutil.Clean(evalTextExpr(desc))
					}
				}
			}
		}
	}
	if tags := extractBraceBlock(rawItem, "tags"); strings.Contains(tags, `"undefined"`) {
		opt.Choices = append(opt.Choices, OptionChoice{Data: "undefined", Description: "默认"})
		opt.Options = append(opt.Options, "undefined")
		opt.Default = "undefined"
		opt.DefaultUnresolved = false
	}
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// splitLuaTables 按顶层逗号切分多个 Lua 表。
func splitLuaTables(s string) []string {
	var out []string
	depth := 0
	start := 0
	inStr := false
	var quote byte

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++ // 跳过转义字符
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = true
			quote = c
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// parseConfigOption 解析单个配置项定义的键值。
//
// 注意 DST 的 modinfo 里 default/min/max 既可能是字符串也可能是裸字面量
// （default = 3 / default = true），options 既可能是字符串数组也可能是
// {description=..., data=...} 的表数组，label/hover/description 也常写双语表达式。
func parseConfigOption(s string) ConfigOption {
	return parseConfigOptionWithLocals(s, nil, nil, "")
}

func parseConfigOptionWithLocals(
	s string,
	localOpts map[string][]OptionChoice,
	fallbackChoices []OptionChoice,
	fallbackDefault string,
) ConfigOption {
	s = stripLuaComments(s)
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		s = trimmed[1 : len(trimmed)-1]
	}
	block := configOptionBlock{}

	// 先把 options 块整体摘出去。否则 options 里每个候选项的
	// description = "简单" 会被当成配置项自身的 description。
	scalar := s
	hasExplicitOptions := false
	if a, b := braceBlockRange(s, "options"); a >= 0 && b > a {
		hasExplicitOptions = true
		optBody := s[a+1 : b]
		scalar = s[:a] + s[b+1:]
		block.Choices = extractChoices(optBody)
	} else if localOpts != nil {
		if ident := strings.TrimSpace(extractFieldExpr(scalar, "options")); ident != "" {
			hasExplicitOptions = true
			block.Choices = localOpts[ident]
		}
	}
	if !hasExplicitOptions && len(fallbackChoices) > 0 {
		block.Choices = fallbackChoices
		for _, c := range block.Choices {
			block.Options = append(block.Options, c.Data)
		}
	}
	if len(block.Options) == 0 {
		for _, c := range block.Choices {
			block.Options = append(block.Options, c.Data)
		}
	}

	str := func(field string) string {
		return textutil.Clean(evalTextExpr(extractFieldExpr(scalar, field)))
	}
	lit := func(field string) string {
		value, _ := evalLiteralExpr(extractFieldExpr(scalar, field))
		return value
	}

	block.Name = evalTextExpr(extractFieldExpr(scalar, "name"))
	block.Label = str("label")
	block.Type = str("type")
	defaultExpr := extractFieldExpr(scalar, "default")
	defaultValue, defaultKnown := evalLiteralExpr(defaultExpr)
	block.Default = defaultValue
	if (defaultExpr == "" || defaultExpr == "nil") && fallbackDefault != "" {
		block.Default = fallbackDefault
		defaultKnown = true
	}
	block.Description = str("description")
	if block.Description == "" {
		block.Description = str("hover")
	}
	block.Min = lit("min")
	block.Max = lit("max")

	if block.Type == "" && len(block.Options) == 0 {
		if block.Default == "true" || block.Default == "false" {
			block.Type = "boolean"
		} else if numberLiteralRe.MatchString(block.Default) {
			block.Type = "number"
		}
	}

	return ConfigOption{
		Name:              block.Name,
		Label:             block.Label,
		Type:              block.Type,
		Default:           block.Default,
		DefaultUnresolved: !defaultKnown,
		Options:           block.Options,
		Choices:           block.Choices,
		Description:       block.Description,
		Min:               block.Min,
		Max:               block.Max,
	}
}

// extractFieldExpr 提取表内 `field = <expr>` 的表达式部分（在顶层逗号/分号处截断）。
func extractFieldExpr(s, field string) string {
	re := regexp.MustCompile(`\b` + field + `\s*=\s*`)
	loc := re.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	rest := s[loc[1]:]
	parenDepth, braceDepth := 0, 0
	inStr := false
	var quote byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = true
			quote = c
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth == 0 {
				return strings.TrimSpace(rest[:i])
			}
			braceDepth--
		case ',', ';':
			if parenDepth == 0 && braceDepth == 0 {
				return strings.TrimSpace(rest[:i])
			}
		}
	}
	return strings.TrimSpace(rest)
}

// evalTextExpr 从 Lua 文本表达式（字面量、`..` 拼接、`L and a or b`、`en_zh(a, b)`）中提取字符串。
// 双语表达式优先取含中文的字符串。
func evalTextExpr(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	strs := luaStringsIn(expr)
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	// 判断是否为纯字符串拼接（"a" .. "b"）
	if isStringConcatExpr(expr) {
		return strings.Join(strs, "")
	}
	if m := zhEntryRe.FindStringSubmatch(expr); m != nil {
		if zh := luaStringsIn(m[1]); len(zh) > 0 && zh[0] != "" {
			return zh[0]
		}
	}
	for _, s := range strs {
		if containsCJK(s) {
			return s
		}
	}
	return strs[0]
}

func isStringConcatExpr(expr string) bool {
	inStr := false
	var quote byte
	hasConcat := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = true
			quote = c
			continue
		}
		if c == '.' && i+1 < len(expr) && expr[i+1] == '.' {
			hasConcat = true
			i++
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			continue
		}
		return false
	}
	return hasConcat
}

func evalLiteralExpr(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if len(expr) >= 2 && (expr[0] == '"' || expr[0] == '\'') {
		for i := 1; i < len(expr); i++ {
			if expr[i] == '\\' {
				i++
				continue
			}
			if expr[i] == expr[0] {
				if i == len(expr)-1 {
					return unescapeLuaString(expr[1:i]), true
				}
				break
			}
		}
	}
	if literalValueRe.MatchString(expr) {
		return expr, true
	}
	return "", false
}

// braceBlockRange 返回 field 之后第一个 {...} 的起止下标（含大括号）。
// 找不到返回 -1, -1。
func braceBlockRange(s, field string) (int, int) {
	open := findKeyBlock(s, field)
	if open < 0 {
		// 退一步：field 后可能不是 "= {" 形式，直接找首个左括号
		idx := strings.Index(s, field)
		if idx < 0 {
			return -1, -1
		}
		rel := strings.Index(s[idx:], "{")
		if rel < 0 {
			return -1, -1
		}
		open = idx + rel
	}
	closeIdx := matchBrace(s, open)
	if closeIdx < 0 {
		return -1, -1
	}
	return open, closeIdx
}

// extractBraceBlock 定位 field 之后第一个 {...}，返回其配对内容（不含最外层大括号）。
func extractBraceBlock(s, field string) string {
	a, b := braceBlockRange(s, field)
	if a < 0 || b <= a {
		return ""
	}
	return s[a+1 : b]
}

// extractChoices 抽取配置项的可选值及其显示文案。
// DST 有两种写法：
//
//	options = {"Easy", "Hard"}
//	options = {{description = "Easy", data = 1}, {description = "Hard", data = 2}}
func extractChoices(body string) []OptionChoice {
	var out []OptionChoice
	for _, item := range splitLuaTables(body) {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			inner := trimmed[1 : len(trimmed)-1]
			dataExpr := extractFieldExpr(inner, "data")
			descExpr := extractFieldExpr(inner, "description")
			if data, known := evalLiteralExpr(dataExpr); known {
				out = append(out, OptionChoice{
					Data:        data,
					Description: textutil.Clean(evalTextExpr(descExpr)),
				})
			}
			continue
		}
		if value, known := evalLiteralExpr(trimmed); known {
			out = append(out, OptionChoice{Data: value, Description: value})
		}
	}
	return out
}

func extractOptions(body string) []string {
	choices := extractChoices(body)
	out := make([]string, 0, len(choices))
	for _, c := range choices {
		out = append(out, c.Data)
	}
	return out
}

// SetEnabled 启用或禁用一个模组。
// 注意：改完必须重启服务器才生效，DST 不支持热加载模组。
func (m *Manager) SetEnabled(id string, enabled bool) error {
	o, err := m.LoadOverrides()
	if err != nil {
		return err
	}
	if _, ok := o.Enabled[id]; !ok {
		o.Order = append(o.Order, id)
	}
	o.Enabled[id] = enabled
	return m.SaveOverrides(o)
}

// SetConfig 设置模组某个配置项的值。
func (m *Manager) SetConfig(id, key string, value interface{}) error {
	o, err := m.LoadOverrides()
	if err != nil {
		return err
	}
	if _, ok := o.Enabled[id]; !ok {
		// 未在 overrides 中出现过的模组，启用时才写入
		o.Order = append(o.Order, id)
		o.Enabled[id] = true
	}
	if o.Config[id] == nil {
		o.Config[id] = make(map[string]interface{})
	}
	o.Config[id][key] = value
	return m.SaveOverrides(o)
}

// DeleteMod 删除本地模组目录，并从 overrides 中移除。
//
// 注意：模组实体是跨房间共享的，删除会同时影响其它房间——
// 调用方（Web 层 / 前端确认框）必须把这一点明确告知用户。
func (m *Manager) DeleteMod(id string) error {
	if !filepath.IsLocal(id) || id != strings.TrimRight(id, ". ") || strings.ContainsAny(id, `/\:`) {
		return fmt.Errorf("无效的模组 ID")
	}
	dirs := []string{filepath.Join(m.modsDir, id)}
	if num, workshop := strings.CutPrefix(id, "workshop-"); workshop {
		if num == "" || strings.Trim(num, "0123456789") != "" {
			return fmt.Errorf("无效的创意工坊模组 ID")
		}
		dirs = m.modDirCandidates(id)
	}
	if dir := m.findModDir(id); dir != "" {
		dirs = append(dirs, dir)
	}
	// 先读完配置，读取失败时保留实体文件，避免留下半次删除。
	overrides := make(map[string]*Overrides, len(m.shards))
	for _, shard := range m.shards {
		o, err := m.forShard(shard).LoadOverrides()
		if err != nil {
			return err
		}
		overrides[shard] = o
	}
	// 清理全部已知副本，包括没有 modinfo.lua 的残缺下载目录。
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("读取模组目录失败: %w", err)
		}
		if !info.IsDir() {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("删除模组目录失败: %w", err)
		}
	}

	for _, shard := range m.shards {
		o := overrides[shard]
		if _, exists := o.Enabled[id]; !exists {
			continue
		}
		for i, existing := range o.Order {
			if existing == id {
				o.Order = append(o.Order[:i], o.Order[i+1:]...)
				break
			}
		}
		delete(o.Enabled, id)
		delete(o.Config, id)
		delete(o.Priority, id)
		if err := m.forShard(shard).SaveOverrides(o); err != nil {
			return err
		}
	}
	return nil
}
