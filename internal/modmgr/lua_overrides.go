package modmgr

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Overrides 解析后的 modoverrides.lua 结构。
type Overrides struct {
	// 顺序保持与原文件一致
	Order    []string
	Enabled  map[string]bool
	Config   map[string]map[string]interface{}
	Priority map[string]int
}

// NewOverrides 创建一个空的 overrides 结构。
func NewOverrides() *Overrides {
	return &Overrides{
		Enabled:  make(map[string]bool),
		Config:   make(map[string]map[string]interface{}),
		Priority: make(map[string]int),
	}
}

// 解析 modoverrides.lua 用到的正则。
//
// 注意：块结构（模组块、configuration_options 块）绝不能用非贪婪正则提取。
// 曾经用 `\["..."\]\s*=\s*\{(.*?)\n\s*\}` 匹配，遇到嵌套大括号时会被
// configuration_options 的右括号提前截断，导致参数丢失、并解析出一个
// 名为 "configuration_options" 的伪配置项。块结构一律用括号配对处理。
var (
	modKeyRe   = regexp.MustCompile(`\[(?:"([^"]+)"|'([^']+)')\]\s*=\s*\{`)
	enabledRe  = regexp.MustCompile(`\benabled\s*=\s*(true|false)`)
	priorityRe = regexp.MustCompile(`\bpriority\s*=\s*(\d+)`)
)

// ParseOverrides 解析 modoverrides.lua 内容。
func ParseOverrides(content string) *Overrides {
	o := NewOverrides()
	// 去除 BOM 与注释，避免注释里的括号干扰匹配
	clean := stripLuaComments(stripBOM(content))

	for _, loc := range modKeyRe.FindAllStringSubmatchIndex(clean, -1) {
		start, end := loc[2], loc[3]
		if start < 0 {
			start, end = loc[4], loc[5]
		}
		id := clean[start:end]

		// loc[1] 指向 '{' 之后，回退一格拿到左括号
		open := loc[1] - 1
		closeIdx := matchBrace(clean, open)
		if closeIdx < 0 {
			continue // 括号不配对，跳过这个模组而不是整份文件失败
		}
		body := clean[open+1 : closeIdx]
		o.Order = append(o.Order, id)

		// 先把 configuration_options 块摘出去，避免其中的同名键
		// （例如某个配置项就叫 enabled）污染外层的 enabled/priority
		scalar := body
		if coOpen := findKeyBlock(body, "configuration_options"); coOpen >= 0 {
			if coClose := matchBrace(body, coOpen); coClose > coOpen {
				scalar = body[:coOpen] + body[coClose+1:]
				if opts := parseTopLevelKV(body[coOpen+1 : coClose]); len(opts) > 0 {
					o.Config[id] = opts
				}
			}
		}

		if em := enabledRe.FindStringSubmatch(scalar); em != nil {
			o.Enabled[id] = em[1] == "true"
		}
		if pm := priorityRe.FindStringSubmatch(scalar); pm != nil {
			if n, err := strconv.Atoi(pm[1]); err == nil {
				o.Priority[id] = n
			}
		}
	}
	return o
}

// ParseOverridesChecked refuses partial/dynamic files before an edit can discard entries.
func ParseOverridesChecked(content string) (*Overrides, error) {
	clean := strings.TrimSpace(stripLuaComments(stripBOM(content)))
	if clean == "" {
		return NewOverrides(), nil
	}
	if !strings.HasPrefix(clean, "return") {
		return nil, fmt.Errorf("模组配置必须返回静态表")
	}
	body := strings.TrimSpace(strings.TrimPrefix(clean, "return"))
	end := matchBrace(body, 0)
	if end < 0 || strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(body[end+1:]), ";")) != "" {
		return nil, fmt.Errorf("模组配置括号不完整或包含动态代码")
	}
	entries := 0
	for _, item := range splitLuaTables(body[1:end]) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		loc := modKeyRe.FindStringIndex(item)
		if loc == nil || loc[0] != 0 || matchBrace(item, loc[1]-1) != len(item)-1 {
			return nil, fmt.Errorf("不支持的模组配置条目，未修改原文件")
		}
		entries++
	}
	o := ParseOverrides(content)
	if len(o.Order) != entries {
		return nil, fmt.Errorf("模组配置包含无法完整解析的嵌套条目")
	}
	return o, nil
}

// matchBrace 返回与 s[open] 处 '{' 配对的 '}' 下标，找不到返回 -1。
// 会跳过字符串字面量，因此 "}" 之类的转义内容不会干扰配对。
func matchBrace(s string, open int) int {
	if open < 0 || open >= len(s) || s[open] != '{' {
		return -1
	}
	depth := 0
	inStr := false
	var quote byte

	for i := open; i < len(s); i++ {
		c := s[i]
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
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// findKeyBlock 定位 `key = {` 中左大括号的下标，找不到返回 -1。
func findKeyBlock(s, key string) int {
	re := regexp.MustCompile(`\b` + key + `\s*=\s*\{`)
	loc := re.FindStringIndex(s)
	if loc == nil {
		return -1
	}
	return loc[1] - 1
}

// parseTopLevelKV 解析一个 Lua 表体中的顶层 key = value。
// 嵌套表（如某个配置项自带 options）不写入配置，直接跳过。
func parseTopLevelKV(body string) map[string]interface{} {
	opts := make(map[string]interface{})
	for _, item := range splitLuaTables(body) {
		eq := indexOutsideString(item, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(item[:eq])
		val := strings.TrimSpace(item[eq+1:])
		if key == "" || val == "" || strings.ContainsAny(val, "{}") {
			continue
		}
		opts[key] = parseLuaValue(val)
	}
	return opts
}

// indexOutsideString 返回 target 在 s 中首个不在字符串字面量内的下标。
func indexOutsideString(s string, target byte) int {
	inStr := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
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
		default:
			if c == target {
				return i
			}
		}
	}
	return -1
}

// parseLuaValue 解析 Lua 字面量为 Go 值。
func parseLuaValue(s string) interface{} {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ","))

	// 带引号的字符串：去引号并还原 formatLuaValue 写入的转义
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return unescapeLuaString(s[1 : len(s)-1])
	}

	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	case "nil", "none", "":
		return nil
	}
	// 尝试解析数字
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}
	return s
}

// unescapeLuaString 还原字符串字面量中的转义序列。
func unescapeLuaString(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			default:
				sb.WriteByte(s[i])
			}
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// formatLuaValue 将 Go 值渲染为 Lua 字面量。
func formatLuaValue(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "nil"
	case bool:
		if val {
			return "true"
		}
		return "false"
	case string:
		// 转义反斜杠、引号与换行，避免破坏 Lua 语法
		escaped := strings.ReplaceAll(val, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, "\r", `\r`)
		escaped = strings.ReplaceAll(escaped, "\n", `\n`)
		escaped = strings.ReplaceAll(escaped, "\t", `\t`)
		return `"` + escaped + `"`
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		// 复杂结构退化为 JSON 字面量，多数 mod_config 不会走到这里
		b, _ := json.Marshal(val)
		return string(b)
	}
}

// String 渲染回 modoverrides.lua。
func (o *Overrides) String() string {
	var sb strings.Builder
	sb.WriteString("return {\n")
	sb.WriteString("    -- 由 dst-windows 自动生成，请勿手工编辑\n")
	for i, id := range o.Order {
		enabled := o.Enabled[id]
		sb.WriteString(fmt.Sprintf("    [\"%s\"] = {\n", id))
		sb.WriteString(fmt.Sprintf("        enabled = %s", formatLuaValue(enabled)))

		if p, ok := o.Priority[id]; ok {
			sb.WriteString(fmt.Sprintf(",\n        priority = %d", p))
		}

		if cfg, ok := o.Config[id]; ok && len(cfg) > 0 {
			sb.WriteString(",\n        configuration_options = {\n")
			keys := slices.Sorted(maps.Keys(cfg)) // 稳定输出，便于 diff 与版本管理
			for j, k := range keys {
				comma := ","
				if j == len(keys)-1 {
					comma = ""
				}
				sb.WriteString(fmt.Sprintf("            %s = %s%s\n", k, formatLuaValue(cfg[k]), comma))
			}
			sb.WriteString("        }")
		}
		sb.WriteString(fmt.Sprintf(",\n    }%s\n", commaIfNotLast(i, len(o.Order))))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func commaIfNotLast(i, total int) string {
	if i < total-1 {
		return ","
	}
	return ""
}

// stripLuaComments 剥掉 Lua 单行注释（--...）与块注释（--[[...]]），保留字符串与换行。
func stripLuaComments(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	inStr := false
	var quote byte

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			sb.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				sb.WriteByte(s[i])
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
			sb.WriteByte(c)
			continue
		}
		if c == '[' && i+1 < len(s) && s[i+1] == '[' {
			end := strings.Index(s[i+2:], "]]")
			if end >= 0 {
				sb.WriteString(s[i : i+2+end+2])
				i += 2 + end + 1
				continue
			}
		}
		if c == '-' && i+1 < len(s) && s[i+1] == '-' {
			if i+3 < len(s) && s[i+2] == '[' && s[i+3] == '[' {
				end := strings.Index(s[i+4:], "]]")
				if end >= 0 {
					for _, ch := range s[i : i+4+end+2] {
						if ch == '\n' {
							sb.WriteByte('\n')
						}
					}
					i += 4 + end + 1
					continue
				}
			}
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				sb.WriteByte('\n')
			}
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

func stripBOM(s string) string {
	return strings.TrimPrefix(s, "\xEF\xBB\xBF")
}
