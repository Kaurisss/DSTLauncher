package worldsettings

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 仅识别字面量表；函数、运算和动态键明确拒绝，不执行 Lua。
type levelToken struct {
	kind       byte
	start, end int
	text       string
}
type levelField struct {
	name      string
	value     levelValue
	separated bool
}
type levelValue struct {
	levelToken
	table *levelTable
}
type levelTable struct {
	open, close int
	fields      []levelField
}
type levelScanner struct {
	src string
	pos int
	tok levelToken
	err error
}

var levelNumber = regexp.MustCompile(`^-?(?:0[xX][0-9a-fA-F]+|(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?)$`)

func levelKeyword(s string) bool {
	return strings.Contains("|and|break|do|else|elseif|end|false|for|function|if|in|local|nil|not|or|repeat|return|then|true|until|while|goto|", "|"+s+"|")
}
func levelSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\v' || c == '\f'
}
func levelLetter(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func levelDigit(c byte) bool  { return c >= '0' && c <= '9' }

func (s *levelScanner) fail(msg string) {
	if s.err == nil {
		s.err = fmt.Errorf("字节 %d: %s（仅支持静态字面量表）", s.pos, msg)
	}
}

// long 返回长括号内容起点和对应结束符。
func (s *levelScanner) long(at int) (int, string) {
	if at >= len(s.src) || s.src[at] != '[' {
		return 0, ""
	}
	i := at + 1
	for i < len(s.src) && s.src[i] == '=' {
		i++
	}
	if i < len(s.src) && s.src[i] == '[' {
		return i + 1, "]" + s.src[at+1:i] + "]"
	}
	return 0, ""
}
func (s *levelScanner) longText(at int) (string, bool) {
	begin, end := s.long(at)
	if end == "" {
		return "", false
	}
	n := strings.Index(s.src[begin:], end)
	if n < 0 {
		s.fail("长字符串或块注释未闭合")
		s.pos = len(s.src)
		return "", true
	}
	s.pos = begin + n + len(end)
	text := strings.ReplaceAll(strings.ReplaceAll(s.src[begin:begin+n], "\r\n", "\n"), "\r", "\n")
	return strings.TrimPrefix(text, "\n"), true
}
func (s *levelScanner) quoted() string {
	quote := s.src[s.pos]
	s.pos++
	var b strings.Builder
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		s.pos++
		if c == quote {
			return b.String()
		}
		if c == '\n' || c == '\r' {
			s.fail("短字符串含未转义换行")
			return ""
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if s.pos == len(s.src) {
			break
		}
		c = s.src[s.pos]
		s.pos++
		switch c {
		case '\\', '\'', '"':
			b.WriteByte(c)
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\r', '\n':
			if c == '\r' && s.pos < len(s.src) && s.src[s.pos] == '\n' {
				s.pos++
			}
			b.WriteByte('\n')
		case 'z':
			for s.pos < len(s.src) && levelSpace(s.src[s.pos]) {
				s.pos++
			}
		case 'x':
			if s.pos+2 > len(s.src) {
				s.fail("十六进制转义不完整")
				return ""
			}
			n, err := strconv.ParseUint(s.src[s.pos:s.pos+2], 16, 8)
			if err != nil {
				s.fail("十六进制转义无效")
				return ""
			}
			s.pos += 2
			b.WriteByte(byte(n))
		default:
			if !levelDigit(c) {
				s.fail("不支持的字符串转义")
				return ""
			}
			begin := s.pos - 1
			for s.pos < len(s.src) && s.pos-begin < 3 && levelDigit(s.src[s.pos]) {
				s.pos++
			}
			n, err := strconv.ParseUint(s.src[begin:s.pos], 10, 8)
			if err != nil {
				s.fail("十进制转义超出范围")
				return ""
			}
			b.WriteByte(byte(n))
		}
	}
	s.fail("短字符串未闭合")
	return ""
}
func (s *levelScanner) next() {
	if s.err != nil {
		return
	}
	for s.pos < len(s.src) {
		if levelSpace(s.src[s.pos]) {
			s.pos++
			continue
		}
		if strings.HasPrefix(s.src[s.pos:], "--") {
			s.pos += 2
			if _, ok := s.longText(s.pos); !ok {
				for s.pos < len(s.src) && s.src[s.pos] != '\n' && s.src[s.pos] != '\r' {
					s.pos++
				}
			}
			if s.err != nil {
				return
			}
			continue
		}
		break
	}
	start := s.pos
	s.tok = levelToken{start: start, end: start}
	if start == len(s.src) {
		return
	}
	c := s.src[s.pos]
	switch {
	case c == '\'' || c == '"':
		s.tok.kind = 's'
		s.tok.text = s.quoted()
	case c == '[':
		if v, ok := s.longText(start); ok {
			s.tok.kind = 's'
			s.tok.text = v
		} else {
			s.tok.kind = c
			s.pos++
		}
	case levelLetter(c):
		s.pos++
		for s.pos < len(s.src) && (levelLetter(s.src[s.pos]) || levelDigit(s.src[s.pos])) {
			s.pos++
		}
		s.tok.kind = 'i'
		s.tok.text = s.src[start:s.pos]
	case levelDigit(c) || c == '-' || c == '.':
		s.pos++
		for s.pos < len(s.src) {
			if strings.HasPrefix(s.src[s.pos:], "--") {
				break
			}
			ch := s.src[s.pos]
			if !(levelLetter(ch) || levelDigit(ch) || ch == '.' || ch == '+' || ch == '-') {
				break
			}
			s.pos++
		}
		s.tok.kind = 'n'
		s.tok.text = s.src[start:s.pos]
		if !levelNumber.MatchString(s.tok.text) {
			s.fail("数字或表达式不受支持")
		}
	default:
		s.pos++
		s.tok.kind = c
	}
	s.tok.end = s.pos
}
func (s *levelScanner) take(kind byte) {
	if s.err != nil {
		return
	}
	if s.tok.kind != kind {
		s.fail(fmt.Sprintf("预期 %q", kind))
		return
	}
	s.next()
}
func (s *levelScanner) value(depth int) levelValue {
	v := levelValue{levelToken: s.tok}
	switch s.tok.kind {
	case '{':
		v.table = s.table(depth + 1)
		if s.err == nil && v.table != nil {
			v.end = v.table.close + 1
			v.text = s.src[v.start:v.end]
		}
	case 's', 'n':
		s.next()
	case 'i':
		if s.tok.text != "true" && s.tok.text != "false" && s.tok.text != "nil" {
			s.fail("不支持的动态表达式")
			return v
		}
		s.next()
	default:
		s.fail("缺少字面量值")
	}
	return v
}
func (s *levelScanner) table(depth int) *levelTable {
	if depth > 128 {
		s.fail("表嵌套过深")
		return nil
	}
	t := &levelTable{open: s.tok.start}
	s.take('{')
	seen := map[string]bool{}
	for s.err == nil && s.tok.kind != '}' {
		f := levelField{}
		switch s.tok.kind {
		case 'i':
			if !levelKeyword(s.tok.text) {
				f.name = s.tok.text
				s.next()
				s.take('=')
				f.value = s.value(depth)
			} else {
				f.value = s.value(depth)
			}
		case '[':
			s.next()
			if s.tok.kind != 's' && s.tok.kind != 'n' {
				s.fail("不支持的动态表键")
				break
			}
			if s.tok.kind == 's' {
				f.name = s.tok.text
			}
			s.next()
			s.take(']')
			s.take('=')
			f.value = s.value(depth)
		default:
			f.value = s.value(depth)
		}
		if f.name != "" {
			if seen[f.name] {
				s.fail("重复表键: " + f.name)
			}
			seen[f.name] = true
		}
		if s.tok.kind == ',' || s.tok.kind == ';' {
			f.separated = true
			s.next()
		} else if s.tok.kind != '}' {
			s.fail("表项之间缺少分隔符或表达式不受支持")
		}
		t.fields = append(t.fields, f)
	}
	t.close = s.tok.start
	s.take('}')
	return t
}
func scanLevelRoot(src string) (*levelTable, error) {
	pos := len(src) - len(strings.TrimPrefix(src, "\ufeff"))
	// 游戏会给明文持久化文件加此文件头。保留原文与偏移，局部更新时才能
	// 逐字节保留文件头、注释和未修改的字段；其它压缩/编码版本仍拒绝解析。
	const kleiPlainHeader = "KLEI     1 "
	if strings.HasPrefix(src[pos:], kleiPlainHeader) {
		pos += len(kleiPlainHeader)
	}
	s := &levelScanner{src: src, pos: pos}
	s.next()
	if s.tok.kind != 'i' || s.tok.text != "return" {
		s.fail("文件必须返回一张表")
	}
	s.next()
	var root *levelTable
	if s.err == nil && s.tok.kind == '{' {
		root = s.table(0)
	} else {
		s.fail("return 后必须是表")
	}
	if s.tok.kind == ';' {
		s.next()
	}
	if s.tok.kind != 0 {
		s.fail("根表后有多余内容")
	}
	return root, s.err
}

func scanLevelData(src string) (*LevelData, *levelTable, error) {
	ld := &LevelData{Raw: src, Scalar: map[string]string{}, Overrides: map[string]string{}}
	root, err := scanLevelRoot(src)
	if err != nil {
		return ld, nil, err
	}
	var overrides *levelTable
	for _, f := range root.fields {
		if f.name == "overrides" {
			overrides = f.value.table
		} else if f.name != "" && f.value.table == nil {
			ld.Scalar[f.name] = f.value.text
		}
	}
	if overrides == nil {
		return ld, nil, fmt.Errorf("配置里没有静态 overrides 表")
	}
	for _, f := range overrides.fields {
		if f.name != "" {
			ld.Overrides[f.name] = f.value.text
		}
	}
	return ld, overrides, nil
}
