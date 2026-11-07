package worldsettings

import (
	"fmt"
	"strings"
)

// The game's dumper wraps literal tables in zero-argument functions. Read their
// literal return values and assignment paths; never evaluate a function or Lua.
func scanSavedMapRoot(text string) (*levelTable, error) {
	text = strings.TrimSuffix(strings.TrimPrefix(text, "\ufeff"), "\x00")
	s := &levelScanner{src: text}
	s.next()
	if s.tok.kind == 'i' && s.tok.text == "return" {
		return scanLevelRootLimit(text, 750000)
	}
	word := func(w string) {
		if s.tok.kind != 'i' || s.tok.text != w {
			s.fail("预期序列化字段 " + w)
			return
		}
		s.next()
	}
	key := func() string {
		s.take('[')
		if s.tok.kind != 's' {
			s.fail("序列化字段必须是字符串键")
			return ""
		}
		name := s.tok.text
		s.next()
		s.take(']')
		return name
	}
	for _, name := range []string{"savedata", "tablefunctions"} {
		word("local")
		word(name)
		s.take('=')
		s.take('{')
		s.take('}')
	}
	functions := map[string]string{}
	root, ents := &levelTable{}, &levelTable{}
	selected := map[string]bool{}
	for s.err == nil && !(s.tok.kind == 'i' && s.tok.text == "return") {
		if s.tok.kind == 'i' && s.tok.text == "tablefunctions" {
			s.next()
			name := key()
			s.take('=')
			word("function")
			s.take('(')
			s.take(')')
			word("return")
			if s.tok.kind != '{' {
				s.fail("序列化函数必须返回数据表")
				break
			}
			begin, depth := s.tok.start, 0
			end := begin
			for s.err == nil {
				if s.tok.kind == 0 {
					s.fail("序列化数据表未闭合")
					break
				}
				if s.tok.kind == '{' {
					depth++
				}
				if s.tok.kind == '}' {
					depth--
				}
				end = s.tok.end
				s.next()
				if depth == 0 {
					break
				}
			}
			word("end")
			if _, exists := functions[name]; exists || len(functions) >= 10000 {
				s.fail("序列化函数重复或数量过多")
				break
			}
			functions[name] = text[begin:end]
		} else if s.tok.kind == 'i' && s.tok.text == "savedata" {
			s.next()
			field := key()
			entity := ""
			if s.tok.kind == '[' {
				entity = key()
			}
			s.take('=')
			if s.tok.kind == '{' {
				// The dumper initializes the entity groups before assigning each prefab.
				if field != "ents" || entity != "" || selected["ents"] {
					s.fail("不支持的序列化表初始化")
					break
				}
				s.take('{')
				s.take('}')
				selected["ents"] = true
				continue
			}
			word("tablefunctions")
			name := key()
			s.take('(')
			s.take(')')
			literal, exists := functions[name]
			if !exists {
				s.fail("序列化函数尚未定义")
				break
			}
			_, landmark := mapLandmarks[entity]
			if field == "map" && entity == "" || field == "ents" && landmark {
				selection := field + "/" + entity
				if selected[selection] {
					s.fail("重复地图字段")
					break
				}
				selected[selection] = true
				table, err := scanLevelRootLimit("return "+literal, 750000)
				if err != nil {
					return nil, fmt.Errorf("地图字段 %s: %w", selection, err)
				}
				if field == "map" {
					root.fields = append(root.fields, levelField{name: "map", value: levelValue{table: table}})
				} else {
					ents.fields = append(ents.fields, levelField{name: entity, value: levelValue{table: table}})
				}
			}
		} else {
			s.fail("不支持的世界序列化语句")
		}
	}
	word("return")
	word("savedata")
	if s.tok.kind == ';' {
		s.next()
	}
	if s.tok.kind != 0 {
		s.fail("世界序列化返回后存在额外内容")
	}
	if s.err != nil {
		return nil, s.err
	}
	root.fields = append(root.fields, levelField{name: "ents", value: levelValue{table: ents}})
	return root, nil
}
