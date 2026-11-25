package worldsettings

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"dst-windows/internal/config"
)

// LevelDataFileName 是游戏自己读写的完整世界定义文件。
//
// 它和 worldgenoverride.lua 的分工由 Klei 在 saveindex.lua 里写死：
//
// leveldataoverride is for GAME USE ... It completely overrides existing saved world data.
// worldgenoverride is for USER USE ... layered on top of whatever savedata we have.
//
// leveldataoverride 优先级更高，面板只能改 overrides，其余内容原样保留。
const LevelDataFileName = "leveldataoverride.lua"

// LevelData 是 leveldataoverride.lua 里我们关心的部分。
type LevelData struct {
	// Scalar 是顶层标量字段（id / name / location / desc / playstyle ...）。
	Scalar map[string]string
	// Overrides 是 overrides 块里的直接子项，嵌套表值保留原文。
	Overrides map[string]string
	// Raw 是原文件内容，写回时以它为底稿。
	Raw string
}

// Location 返回世界位置（forest / cave / shipwrecked / volcanoworld ...）。
func (l *LevelData) Location() string { return l.Scalar["location"] }

// LevelDataPath 返回某分片的 leveldataoverride.lua 路径。
func LevelDataPath(clusterDir, shard string) string {
	return filepath.Join(clusterDir, shard, LevelDataFileName)
}

// ReadLevelData 读取静态世界定义；不存在时返回 (nil, false, nil)。
func ReadLevelData(clusterDir, shard string) (*LevelData, bool, error) {
	path := LevelDataPath(clusterDir, shard)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	ld, _, err := scanLevelData(string(b))
	if err != nil {
		return nil, false, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if ld.Location() == "" {
		return nil, false, fmt.Errorf("%s 缺少 location 字段，无法判断世界类型", path)
	}
	return ld, true, nil
}

// ParseLevelData 保留旧签名；需要解析错误的调用方应使用 ReadLevelData。
// 无法安全解析时仅保留 Raw，WithOverrides 会重新校验并明确报错。
func ParseLevelData(content string) *LevelData {
	ld, _, err := scanLevelData(content)
	if err != nil {
		return &LevelData{Raw: content, Scalar: map[string]string{}, Overrides: map[string]string{}}
	}
	return ld
}

// WithOverrides 只替换直接子项的值区间，不重排非点名内容。
func (l *LevelData) WithOverrides(updates map[string]string) (string, error) {
	_, table, err := scanLevelData(l.Raw)
	if err != nil {
		return "", fmt.Errorf("无法局部修改 %s: %w", LevelDataFileName, err)
	}
	fields := map[string]levelValue{}
	for _, f := range table.fields {
		if f.name != "" {
			fields[f.name] = f.value
		}
	}
	names := make([]string, 0, len(updates))
	for name := range updates {
		if !isSafeName(name) || levelKeyword(name) {
			return "", fmt.Errorf("设置项名称 %q 非法", name)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	type patch struct {
		start, end int
		text       string
	}
	var patches []patch
	var additions strings.Builder
	for _, name := range names {
		value := updates[name]
		old, exists := fields[name]
		if exists && old.text == value {
			continue
		}
		if !isSafeValue(value) {
			return "", fmt.Errorf("%s 的取值 %q 含非法字符", name, value)
		}
		if !exists {
			additions.WriteString(" " + name + `="` + value + `",`)
			continue
		}
		replacement := value
		switch old.kind {
		case 's':
			quote := l.Raw[old.start : old.start+1]
			if quote == "[" {
				// 新值不含括号，沿用长字符串的定界符即可。
				scan := &levelScanner{src: l.Raw}
				start, end := scan.long(old.start)
				replacement = l.Raw[old.start:start] + value + end
			} else {
				replacement = quote + value + quote
			}
		case 'n':
			if !levelNumber.MatchString(value) {
				return "", fmt.Errorf("%s 必须保持数字类型", name)
			}
		case 'i':
			if (old.text != "true" && old.text != "false") || (value != "true" && value != "false") {
				return "", fmt.Errorf("%s 必须保持布尔类型", name)
			}
		default:
			return "", fmt.Errorf("%s 不是可局部修改的标量", name)
		}
		patches = append(patches, patch{old.start, old.end, replacement})
	}
	if additions.Len() > 0 {
		if n := len(table.fields); n > 0 && !table.fields[n-1].separated {
			end := table.fields[n-1].value.end
			patches = append(patches, patch{end, end, ","})
		}
		patches = append(patches, patch{table.close, table.close, additions.String()})
	}
	slices.SortStableFunc(patches, func(a, b patch) int { return cmp.Compare(a.start, b.start) })
	var out strings.Builder
	at := 0
	for _, p := range patches {
		out.WriteString(l.Raw[at:p.start])
		out.WriteString(p.text)
		at = p.end
	}
	out.WriteString(l.Raw[at:])
	return out.String(), nil
}

// WriteLevelData 校验后原子写回，原文件备份到 .bak。
func WriteLevelData(clusterDir, shard, content string) error {
	path := LevelDataPath(clusterDir, shard)
	ld, _, err := scanLevelData(content)
	if err != nil {
		return fmt.Errorf("拒绝写入 %s: %w", path, err)
	}
	if ld.Location() == "" {
		return fmt.Errorf("拒绝写入 %s: 缺少 location 字段", path)
	}
	old, err := os.ReadFile(path)
	if err == nil {
		if err := os.WriteFile(path+".bak", old, 0o644); err != nil {
			return fmt.Errorf("备份 %s 失败: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取 %s 备份失败: %w", path, err)
	}
	if err := config.WriteFileAtomic(path, []byte(content)); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", path, err)
	}
	return nil
}
