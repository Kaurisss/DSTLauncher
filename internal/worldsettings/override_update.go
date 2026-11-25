package worldsettings

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"dst-windows/internal/config"
)

// 普通配置与 leveldata 共用静态表扫描，不能按行丢掉未知键或其类型。
func readOverride(content string) (*Override, *levelTable, error) {
	root, err := scanLevelRoot(content)
	if err != nil {
		return nil, nil, err
	}
	o := &Override{Overrides: map[string]string{}, literals: map[string]overrideLiteral{}}
	legacy := ""
	for _, f := range root.fields {
		switch f.name {
		case "override_enabled":
			o.Enabled = f.value.text == "true"
		case "worldgen_preset":
			if f.value.kind == 's' {
				o.WorldGenPreset = f.value.text
			}
		case "settings_preset":
			if f.value.kind == 's' {
				o.SettingsPreset = f.value.text
			}
		case "preset":
			if f.value.kind == 's' {
				legacy = f.value.text
			}
		case "overrides":
			if f.value.table == nil {
				if f.value.text == "nil" {
					continue
				}
				return nil, nil, fmt.Errorf("overrides 必须是静态表")
			}
			for _, item := range f.value.table.fields {
				if item.name != "" {
					o.Overrides[item.name] = item.value.text
					o.literals[item.name] = overrideLiteral{item.value.text, content[item.value.start:item.value.end]}
				}
			}
		}
	}
	if o.WorldGenPreset == "" {
		o.WorldGenPreset = legacy
	}
	if o.SettingsPreset == "" {
		o.SettingsPreset = legacy
	}
	return o, root, nil
}

// UpdateFile 用于面板编辑已有普通配置；导入的整文件替换仍走 WriteFile。
// 未点名项及未变项直接保留源字节，不把 bool/number/table 重新写成字符串。
func UpdateFile(clusterDir, shard string, o *Override) error {
	path := FilePath(clusterDir, shard)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := o.Validate(); err != nil {
			return err
		}
		return WriteFile(clusterDir, shard, o)
	}
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	old, root, err := readOverride(string(b))
	if err != nil {
		return fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if err := o.validateChanges(old.Overrides); err != nil {
		return err
	}
	updates := map[string]string{}
	for name, value := range o.Overrides {
		if previous, ok := old.Overrides[name]; !ok || previous != value {
			updates[name] = value
		}
	}
	raw := string(b)
	changes := map[string]string{}
	if len(updates) > 0 {
		hasTable := false
		for _, f := range root.fields {
			if f.name == "overrides" && f.value.table != nil {
				hasTable = true
			}
		}
		if hasTable {
			raw, err = (&LevelData{Raw: raw}).WithOverrides(updates)
			if err != nil {
				return err
			}
			_, root, err = readOverride(raw)
			if err != nil {
				return err
			}
		} else {
			names := slices.Sorted(maps.Keys(updates))
			var table strings.Builder
			table.WriteByte('{')
			for _, name := range names {
				fmt.Fprintf(&table, "%s=%q,", name, updates[name])
			}
			table.WriteByte('}')
			changes["overrides"] = table.String()
		}
	}
	if o.Enabled != old.Enabled {
		changes["override_enabled"] = luaBool(o.Enabled)
	}
	if o.WorldGenPreset != old.WorldGenPreset || o.SettingsPreset != old.SettingsPreset {
		changes["worldgen_preset"] = presetLiteral(o.WorldGenPreset)
		changes["settings_preset"] = presetLiteral(o.SettingsPreset)
		// 明确更新预设时消除遗留 preset 的回退，其他时候不动它。
		for _, f := range root.fields {
			if f.name == "preset" {
				changes["preset"] = "nil"
			}
		}
	}
	raw = patchLiteralFields(raw, root, changes)
	if _, _, err := readOverride(raw); err != nil {
		return fmt.Errorf("拒绝写入 %s: %w", path, err)
	}
	return config.WriteFileAtomic(path, []byte(raw))
}

func presetLiteral(p string) string {
	if p == "" {
		return "nil"
	}
	return fmt.Sprintf("%q", p)
}

// 仅供已校验的内部字面量使用；外部输入必须先走类型和取值校验。
func patchLiteralFields(src string, table *levelTable, changes map[string]string) string {
	type patch struct {
		start, end int
		text       string
	}
	var patches []patch
	missing := map[string]string{}
	for name, value := range changes {
		missing[name] = value
	}
	for _, f := range table.fields {
		if value, ok := missing[f.name]; ok {
			patches = append(patches, patch{f.value.start, f.value.end, value})
			delete(missing, f.name)
		}
	}
	if len(missing) > 0 {
		names := slices.Sorted(maps.Keys(missing))
		if n := len(table.fields); n > 0 && !table.fields[n-1].separated {
			end := table.fields[n-1].value.end
			patches = append(patches, patch{end, end, ","})
		}
		var added strings.Builder
		for _, name := range names {
			added.WriteString(" " + name + "=" + missing[name] + ",")
		}
		patches = append(patches, patch{table.close, table.close, added.String()})
	}
	slices.SortStableFunc(patches, func(a, b patch) int { return cmp.Compare(a.start, b.start) })
	var out strings.Builder
	at := 0
	for _, p := range patches {
		out.WriteString(src[at:p.start])
		out.WriteString(p.text)
		at = p.end
	}
	out.WriteString(src[at:])
	return out.String()
}
