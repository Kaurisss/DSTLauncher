package modmgr

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"dst-windows/internal/textutil"
)

// modInfoNameRe 匹配顶层的 name = 赋值行（捕获值部分）。
var modInfoNameRe = regexp.MustCompile(`^name\s*=\s*(.+?)\s*$`)

// ModInfo 读取一个模组的 modinfo.lua 名称、本地版本与纯客户端标记。
//
// modinfo 的写法差异极大，名字解析必须覆盖三种常见形态：
//
//	name = "Simple Health Bar DST"                    字符串字面量
//	name = L and "Show Me" or "Show Me (中文)"         双语表达式（中文 mod 惯用）
//	name = ChooseTranslationTable({ zh = "中文名" })    翻译表
//
// 并且绝不能全局搜第一个 name = "..."：configuration_options 里每个配置项
// 都有自己的 name 字段，抓到就会把配置项名当成模组名——
// 棱镜、神话人物等 mod 曾因此被显示成 "Title" / "language_switch"。
// 所以先摘掉 configuration_options 块，再逐行找顶层的 name 赋值。
// ModSummary 汇总单个模组列表展示所需的本地元数据。
type ModSummary struct {
	Name        string
	Version     string
	ClientOnly  bool
	IconVersion string
}

// InspectMod 单次读取 modinfo.lua 并在需要时流式计算图标版本，避免列表扫描重复读盘和全目录 WalkDir。
func (m *Manager) InspectMod(id, dir string) ModSummary {
	if dir == "" {
		return ModSummary{Name: id}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ModSummary{Name: id}
	}
	defer root.Close()

	data, err := readIconFile(root, "modinfo.lua", 4<<20)
	if err != nil {
		return ModSummary{Name: id}
	}
	text := string(data)
	name, version, clientOnly := parseModInfoSummary(id, text)
	if clientOnly {
		return ModSummary{Name: name, Version: version, ClientOnly: true}
	}
	iconVersion := ""
	if validateModIconID(id) == nil {
		if v, err := computeModIconVersion(root, text); err == nil {
			iconVersion = v
		}
	}
	return ModSummary{
		Name:        name,
		Version:     version,
		ClientOnly:  false,
		IconVersion: iconVersion,
	}
}

func parseModInfoSummary(id, text string) (name, version string, clientOnly bool) {
	name = extractModInfoName(text)
	// mod 作者常在名字里放自己图标字体的私用区字形（如 U+F000A），
	// 任何系统字体都没有，界面上是豆腐块——剥掉
	name = textutil.Clean(name)
	if name == "" {
		// modinfo 里确实没写名字（或写法太怪解析不出），
		// 退回模组 ID，至少保证列表里能对上号
		name = id
	}
	return name, extractModInfoVersion(text), extractModInfoLiteral(text, "client_only_mod") == "true"
}

func (m *Manager) ModInfo(id string) (name, version string, clientOnly bool) {
	dir := m.findModDir(id)
	if dir == "" {
		return id, "", false
	}

	data, err := os.ReadFile(filepath.Join(dir, "modinfo.lua"))
	if err != nil {
		return id, "", false
	}

	return parseModInfoSummary(id, string(data))
}

var modInfoTokenRe = regexp.MustCompile(`(?s)--\[=*\[.*?\]=*\]|--[^\r\n]*|\[=*\[.*?\]=*\]|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[A-Za-z_][A-Za-z_0-9]*|[0-9]+(?:\.[0-9]+)?|\S`)

func extractModInfoVersion(text string) string {
	value := extractModInfoLiteral(text, "version")
	if value == "true" || value == "false" || value == "nil" {
		return ""
	}
	version, _ := evalLiteralExpr(value)
	return strings.TrimSpace(textutil.Clean(version))
}

func extractModInfoLiteral(text, field string) string {
	var tokens []string
	for _, token := range modInfoTokenRe.FindAllString(stripBOM(text), -1) {
		if !strings.HasPrefix(token, "--") {
			tokens = append(tokens, token)
		}
	}
	depth, blocks := 0, 0
	result := ""
	for i, token := range tokens {
		switch token {
		case "{", "(", "[":
			depth++
		case "}", ")", "]":
			depth--
		case "function", "if", "do", "repeat":
			blocks++
		case "end", "until":
			if blocks > 0 {
				blocks--
			}
		}
		if token != field || depth != 0 || blocks != 0 || i+2 >= len(tokens) || tokens[i+1] != "=" {
			continue
		}
		if i > 0 && (tokens[i-1] == "." || tokens[i-1] == ":" || tokens[i-1] == "local") {
			continue
		}
		result = ""
		value := tokens[i+2]
		// A literal must end the statement, not be part of a concatenation/operator.
		if i+3 < len(tokens) {
			next := tokens[i+3]
			statement := next == ";" || next == "local" || next == "function" || next == "if" || next == "for" || next == "while" || next == "do" || next == "repeat" || next == "return"
			if !statement && !(i+4 < len(tokens) && tokens[i+4] == "=") {
				continue
			}
		}
		if _, ok := evalLiteralExpr(value); ok {
			result = value
		}
	}
	// shortcut: 动态字段返回未知；需要支持时再扩展静态求值，不执行模组代码。
	return result
}

// extractModInfoName 从 modinfo.lua 文本中提取模组名，解析不出返回空串。
func extractModInfoName(text string) string {
	// configuration_options 里每个配置项都有 name 字段，
	// 必须把整个块摘掉，否则会抓到配置项的名字
	if a, b := braceBlockRange(text, "configuration_options"); a >= 0 && b > a {
		text = text[:a] + text[b+1:]
	}

	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		// 整行注释直接跳过：不少 mod 在注释里给出台名写法示例
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		mm := modInfoNameRe.FindStringSubmatch(line)
		if mm == nil {
			continue
		}
		if name := modInfoValue(mm[1]); name != "" {
			return name
		}
	}
	return ""
}

// modInfoValue 解析 name 赋值的右侧内容。
//
// 真实世界的 modinfo 有两种极端写法，取值策略必须同时照顾：
//   - 值以字符串开头：压缩成一行的 modinfo（name 后面同一行还跟着
//     version/description 等其它赋值），取**第一个**字符串才是名字；
//   - 双语表达式（L and "English" or "中文"）：优先取**含中文**的那个串，
//     没有中文时取最后一个兜底串。
func modInfoValue(expr string) string {
	expr = stripLuaTrailingComment(expr)
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(expr, ",")
	strs := luaStringsIn(expr)
	if len(strs) == 0 {
		return ""
	}

	if len(expr) > 0 && (expr[0] == '"' || expr[0] == '\'') {
		return strings.TrimSpace(strs[0])
	}
	for _, s := range strs {
		if containsCJK(s) {
			return strings.TrimSpace(s)
		}
	}
	return strings.TrimSpace(strs[len(strs)-1])
}

// containsCJK 判断字符串是否含中日韩字符。
// 双语 mod 的名字表达式里，含中文的那个串通常就是面板用户想要的名字。
func containsCJK(s string) bool {
	return cjkCount(s) > 0
}

func cjkCount(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			n++
		}
	}
	return n
}

// stripLuaTrailingComment 去掉行尾注释（-- 及其后内容），保留字符串字面量。
func stripLuaTrailingComment(s string) string {
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
		case '-':
			if i+1 < len(s) && s[i+1] == '-' {
				return s[:i]
			}
		}
	}
	return s
}

// luaStringsIn 按出现顺序提取表达式里的字符串字面量。
func luaStringsIn(s string) []string {
	var out []string
	inStr := false
	var quote byte
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				out = append(out, unescapeLuaString(s[start:i]))
				inStr = false
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = true
			quote = c
			start = i + 1
		}
	}
	return out
}
