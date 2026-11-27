package modmgr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	luaMaxTokens     = 200000
	luaMaxSteps      = 1000000
	luaMaxLoopSteps  = 10000
	luaMaxCallDepth  = 32
	luaMaxTableItems = 100000
)

type luaToken struct {
	text string
	kind byte
}

type luaTable struct {
	fields map[string]any
	array  []any
}

type luaFunction struct {
	params []string
	body   []luaToken
	owner  *luaEnv
	parsed bool
	code   []luaStmt
}

type luaEnv struct {
	values map[string]any
	parent *luaEnv
	steps  *int
	depth  int
}

type luaExpr struct {
	kind     byte
	value    any
	name     string
	op       string
	left     *luaExpr
	right    *luaExpr
	operand  *luaExpr
	args     []*luaExpr
	receiver *luaExpr
	key      *luaExpr
	fields   []luaTableField
}

type luaTableField struct {
	key   *luaExpr
	value *luaExpr
	name  string
}

type luaStmt struct {
	kind     byte
	names    []string
	targets  []*luaExpr
	exprs    []*luaExpr
	body     []luaStmt
	branches []luaBranch
	elseBody []luaStmt
	forName  string
	forKey   string
	forValue string
	forStart *luaExpr
	forEnd   *luaExpr
	forStep  *luaExpr
	forExpr  *luaExpr
	function *luaFunction
}

type luaBranch struct {
	condition *luaExpr
	body      []luaStmt
}

type luaParser struct {
	tokens []luaToken
	pos    int
}

func parseDynamicConfigSchema(text string) ([]ConfigOption, bool, error) {
	// shortcut: only schema-relevant Lua is evaluated, unsupported code falls back to static parsing.
	if !strings.Contains(text, "configuration_options") {
		return nil, false, nil
	}
	if !dynamicConfigText(text) {
		return nil, false, nil
	}
	tokens, err := lexLua(text)
	if err != nil {
		return nil, true, err
	}
	env := &luaEnv{values: map[string]any{}, steps: new(int)}
	env.values["locale"] = "zh"
	env.values["folder_name"] = "workshop-unknown"
	env.values["table"] = &luaTable{fields: map[string]any{
		"insert": luaBuiltin(func(args []any) any {
			if len(args) < 2 {
				return nil
			}
			table, ok := args[0].(*luaTable)
			if !ok {
				return nil
			}
			index := len(table.array) + 1
			if len(args) > 2 {
				if n, ok := luaNumber(args[1]); ok {
					index = int(n)
				}
			}
			table.setIndex(float64(index), args[2])
			return nil
		}),
		"remove": luaBuiltin(func(args []any) any {
			if len(args) == 0 {
				return nil
			}
			table, ok := args[0].(*luaTable)
			if !ok || len(table.array) == 0 {
				return nil
			}
			index := len(table.array)
			if len(args) > 1 {
				if n, ok := luaNumber(args[1]); ok {
					index = int(n)
				}
			}
			if index < 1 || index > len(table.array) {
				return nil
			}
			value := table.array[index-1]
			table.array = append(table.array[:index-1], table.array[index:]...)
			return value
		}),
	}}
	env.values["tostring"] = luaBuiltin(func(args []any) any {
		if len(args) == 0 {
			return "nil"
		}
		return luaString(args[0])
	})
	env.values["tonumber"] = luaBuiltin(func(args []any) any {
		if len(args) == 0 {
			return nil
		}
		if n, ok := luaNumber(args[0]); ok {
			return n
		}
		return nil
	})
	env.values["ipairs"] = luaBuiltin(func(args []any) any {
		if len(args) == 0 {
			return nil
		}
		if _, ok := args[0].(*luaTable); ok {
			return args[0]
		}
		return nil
	})

	parser := &luaParser{tokens: tokens}
	statements, err := parser.parseBlock(nil)
	if err != nil {
		return nil, true, err
	}
	if err := executeLuaStatements(statements, env); err != nil {
		return nil, true, err
	}

	value, ok := env.get("configuration_options")
	if !ok || value == nil {
		if fn, ok := env.get("DoInit"); ok {
			if _, ok := fn.(*luaFunction); ok {
				if _, err := callLuaFunction(fn, []any{"zh", nil}, env); err != nil {
					return nil, true, err
				}
				value, ok = env.get("configuration_options")
			}
		}
	}
	if !ok || value == nil {
		return nil, false, nil
	}
	table, ok := value.(*luaTable)
	if !ok {
		return nil, true, fmt.Errorf("configuration_options 不是表")
	}
	options, err := configOptionsFromTable(table)
	if err != nil {
		return nil, true, err
	}
	return options, true, nil
}

func dynamicConfigText(text string) bool {
	assignment := findConfigOptionsAssignment(text)
	if assignment == nil {
		return strings.Contains(text, "function DoInit")
	}
	rest := strings.TrimSpace(text[assignment[1]:])
	if !strings.HasPrefix(rest, "{") {
		return true
	}
	open := strings.Index(rest, "{")
	if open < 0 {
		return true
	}
	close := matchBrace(rest, open)
	if close < 0 {
		return true
	}
	body := rest[open+1 : close]
	return strings.Contains(body, "(") ||
		strings.Contains(text[:assignment[0]], "function ") ||
		strings.Contains(text[:assignment[0]], "for ")
}

func findConfigOptionsAssignment(text string) []int {
	for _, loc := range configOptsAssignRe.FindAllStringIndex(text, -1) {
		if loc[1] < len(text) && text[loc[1]] == '=' {
			continue
		}
		return loc
	}
	return nil
}

type luaBuiltin func([]any) any

func lexLua(text string) ([]luaToken, error) {
	tokens := make([]luaToken, 0, len(text)/3)
	for i := 0; i < len(text); {
		switch text[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '\'', '"':
			quote := text[i]
			i++
			var value strings.Builder
			for i < len(text) {
				if text[i] == '\\' && i+1 < len(text) {
					value.WriteString(text[i : i+2])
					i += 2
					continue
				}
				if text[i] == quote {
					tokens = append(tokens, luaToken{kind: 's', text: unescapeLuaString(value.String())})
					i++
					break
				}
				value.WriteByte(text[i])
				i++
			}
			if i > len(text) || (i == len(text) && (len(text) == 0 || text[len(text)-1] != quote)) {
				return nil, fmt.Errorf("字符串未闭合")
			}
		case '[':
			if i+1 < len(text) && text[i+1] == '[' {
				end := strings.Index(text[i+2:], "]]")
				if end < 0 {
					return nil, fmt.Errorf("长字符串未闭合")
				}
				tokens = append(tokens, luaToken{kind: 's', text: text[i+2 : i+2+end]})
				i += end + 4
				continue
			}
			tokens = append(tokens, luaToken{kind: text[i], text: text[i : i+1]})
			i++
		default:
			if isLuaIdentStart(text[i]) {
				start := i
				for i < len(text) && isLuaIdentPart(text[i]) {
					i++
				}
				tokens = append(tokens, luaToken{kind: 'i', text: text[start:i]})
				continue
			}
			if text[i] == '0' && i+1 < len(text) && (text[i+1] == 'x' || text[i+1] == 'X') {
				start := i + 2
				i = start
				for i < len(text) && ((text[i] >= '0' && text[i] <= '9') || (text[i] >= 'a' && text[i] <= 'f') || (text[i] >= 'A' && text[i] <= 'F')) {
					i++
				}
				if i == start {
					return nil, fmt.Errorf("十六进制数字无效")
				}
				value, err := strconv.ParseUint(text[start:i], 16, 64)
				if err != nil {
					return nil, fmt.Errorf("十六进制数字无效")
				}
				tokens = append(tokens, luaToken{kind: 'n', text: strconv.FormatUint(value, 10)})
				continue
			}
			if isLuaDigit(text[i]) || (text[i] == '.' && i+1 < len(text) && isLuaDigit(text[i+1])) {
				start := i
				i++
				for i < len(text) && (isLuaDigit(text[i]) || text[i] == '.') {
					i++
				}
				tokens = append(tokens, luaToken{kind: 'n', text: text[start:i]})
				continue
			}
			if i+1 < len(text) {
				two := text[i : i+2]
				if two == "==" || two == "~=" || two == "<=" || two == ">=" || two == ".." {
					tokens = append(tokens, luaToken{kind: 'o', text: two})
					i += 2
					continue
				}
			}
			if strings.ContainsRune("{}(),=.+-*/%^#[];<>:", rune(text[i])) {
				tokens = append(tokens, luaToken{kind: text[i], text: text[i : i+1]})
				i++
				continue
			}
			return nil, fmt.Errorf("不支持的 Lua 字符 %q", text[i])
		}
		if len(tokens) > luaMaxTokens {
			return nil, fmt.Errorf("modinfo.lua 过大")
		}
	}
	tokens = append(tokens, luaToken{kind: 0})
	return tokens, nil
}

func isLuaIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isLuaIdentPart(c byte) bool {
	return isLuaIdentStart(c) || isLuaDigit(c)
}

func isLuaDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func (p *luaParser) parseBlock(stop map[string]bool) ([]luaStmt, error) {
	var out []luaStmt
	for p.current().kind != 0 && (stop == nil || !stop[p.current().text]) {
		if p.accept(";") {
			continue
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		out = append(out, stmt)
	}
	return out, nil
}

func (p *luaParser) parseStatement() (luaStmt, error) {
	switch p.current().text {
	case "local":
		p.next()
		if p.accept("function") {
			return p.parseFunctionStatement(true)
		}
		return p.parseAssignment(true)
	case "function":
		p.next()
		return p.parseFunctionStatement(false)
	case "if":
		return p.parseIf()
	case "for":
		return p.parseFor()
	case "while":
		return p.parseWhile()
	case "do":
		p.next()
		body, err := p.parseBlock(map[string]bool{"end": true})
		if err != nil {
			return luaStmt{}, err
		}
		if !p.accept("end") {
			return luaStmt{}, p.errorf("do 块缺少 end")
		}
		return luaStmt{kind: 'd', body: body}, nil
	case "return":
		p.next()
		if p.current().kind == 0 || p.current().text == "end" || p.current().text == "else" || p.current().text == "elseif" {
			return luaStmt{kind: 'r'}, nil
		}
		expr, err := p.parseExpression(0)
		if err != nil {
			return luaStmt{}, err
		}
		return luaStmt{kind: 'r', exprs: []*luaExpr{expr}}, nil
	case "break":
		p.next()
		return luaStmt{kind: 'b'}, nil
	}
	return p.parseAssignment(false)
}

func (p *luaParser) parseFunctionStatement(local bool) (luaStmt, error) {
	name, err := p.expectIdentifier()
	if err != nil {
		return luaStmt{}, err
	}
	for p.accept(".") {
		part, err := p.expectIdentifier()
		if err != nil {
			return luaStmt{}, err
		}
		name += "." + part
	}
	params, err := p.parseParameters()
	if err != nil {
		return luaStmt{}, err
	}
	body, err := p.captureFunctionBody()
	if err != nil {
		return luaStmt{}, err
	}
	return luaStmt{kind: 'f', names: []string{name}, function: &luaFunction{params: params, body: body}}, nil
}

func (p *luaParser) parseParameters() ([]string, error) {
	if !p.accept("(") {
		return nil, p.errorf("函数缺少参数列表")
	}
	var params []string
	for !p.accept(")") {
		name, err := p.expectIdentifier()
		if err != nil {
			return nil, err
		}
		params = append(params, name)
		if !p.accept(",") && p.current().text != ")" {
			return nil, p.errorf("函数参数列表无效")
		}
	}
	return params, nil
}

func (p *luaParser) captureFunctionBody() ([]luaToken, error) {
	start := p.pos
	depth := 1
	pendingLoop := false
	for p.pos < len(p.tokens) {
		text := p.current().text
		switch text {
		case "if", "for", "while", "function", "repeat":
			if p.current().kind != 'i' {
				p.next()
				continue
			}
			depth++
			pendingLoop = text == "for" || text == "while"
		case "do":
			if p.current().kind != 'i' {
				p.next()
				continue
			}
			if pendingLoop {
				pendingLoop = false
			} else {
				depth++
			}
		case "end":
			if p.current().kind != 'i' {
				p.next()
				continue
			}
			depth--
			if depth == 0 {
				body := append([]luaToken{}, p.tokens[start:p.pos]...)
				p.next()
				return body, nil
			}
		case "until":
			if p.current().kind != 'i' {
				p.next()
				continue
			}
			depth--
		}
		p.next()
	}
	return nil, p.errorf("函数缺少 end")
}

func (p *luaParser) parseIf() (luaStmt, error) {
	p.next()
	condition, err := p.parseExpression(0)
	if err != nil {
		return luaStmt{}, err
	}
	if !p.accept("then") {
		return luaStmt{}, p.errorf("if 缺少 then")
	}
	body, err := p.parseBlock(map[string]bool{"elseif": true, "else": true, "end": true})
	if err != nil {
		return luaStmt{}, err
	}
	stmt := luaStmt{kind: 'i', branches: []luaBranch{{condition: condition, body: body}}}
	for p.accept("elseif") {
		condition, err := p.parseExpression(0)
		if err != nil {
			return luaStmt{}, err
		}
		if !p.accept("then") {
			return luaStmt{}, p.errorf("elseif 缺少 then")
		}
		body, err := p.parseBlock(map[string]bool{"elseif": true, "else": true, "end": true})
		if err != nil {
			return luaStmt{}, err
		}
		stmt.branches = append(stmt.branches, luaBranch{condition: condition, body: body})
	}
	if p.accept("else") {
		stmt.elseBody, err = p.parseBlock(map[string]bool{"end": true})
		if err != nil {
			return luaStmt{}, err
		}
	}
	if !p.accept("end") {
		return luaStmt{}, p.errorf("if 缺少 end")
	}
	return stmt, nil
}

func (p *luaParser) parseFor() (luaStmt, error) {
	p.next()
	first, err := p.expectIdentifier()
	if err != nil {
		return luaStmt{}, err
	}
	if p.accept("=") {
		start, err := p.parseExpression(0)
		if err != nil {
			return luaStmt{}, err
		}
		if !p.accept(",") {
			return luaStmt{}, p.errorf("数字 for 缺少逗号")
		}
		end, err := p.parseExpression(0)
		if err != nil {
			return luaStmt{}, err
		}
		var step *luaExpr
		if p.accept(",") {
			step, err = p.parseExpression(0)
			if err != nil {
				return luaStmt{}, err
			}
		}
		if !p.accept("do") {
			return luaStmt{}, p.errorf("for 缺少 do")
		}
		body, err := p.parseBlock(map[string]bool{"end": true})
		if err != nil {
			return luaStmt{}, err
		}
		if !p.accept("end") {
			return luaStmt{}, p.errorf("for 缺少 end")
		}
		return luaStmt{kind: 'n', forName: first, forStart: start, forEnd: end, forStep: step, body: body}, nil
	}
	var second string
	if p.accept(",") {
		second, err = p.expectIdentifier()
		if err != nil {
			return luaStmt{}, err
		}
	}
	if !p.accept("in") {
		return luaStmt{}, p.errorf("for 只支持数字循环或 in 循环")
	}
	iter, err := p.parseExpression(0)
	if err != nil {
		return luaStmt{}, err
	}
	if !p.accept("do") {
		return luaStmt{}, p.errorf("for 缺少 do")
	}
	body, err := p.parseBlock(map[string]bool{"end": true})
	if err != nil {
		return luaStmt{}, err
	}
	if !p.accept("end") {
		return luaStmt{}, p.errorf("for 缺少 end")
	}
	return luaStmt{kind: 'g', forKey: first, forValue: second, forExpr: iter, body: body}, nil
}

func (p *luaParser) parseWhile() (luaStmt, error) {
	p.next()
	condition, err := p.parseExpression(0)
	if err != nil {
		return luaStmt{}, err
	}
	if !p.accept("do") {
		return luaStmt{}, p.errorf("while 缺少 do")
	}
	body, err := p.parseBlock(map[string]bool{"end": true})
	if err != nil {
		return luaStmt{}, err
	}
	if !p.accept("end") {
		return luaStmt{}, p.errorf("while 缺少 end")
	}
	return luaStmt{kind: 'w', exprs: []*luaExpr{condition}, body: body}, nil
}

func (p *luaParser) parseAssignment(local bool) (luaStmt, error) {
	var names []string
	var targets []*luaExpr
	if local {
		name, err := p.expectIdentifier()
		if err != nil {
			return luaStmt{}, err
		}
		names = append(names, name)
		for p.accept(",") {
			name, err := p.expectIdentifier()
			if err != nil {
				return luaStmt{}, err
			}
			names = append(names, name)
		}
	} else {
		target, err := p.parseExpression(0)
		if err != nil {
			return luaStmt{}, err
		}
		targets = append(targets, target)
		for p.accept(",") {
			target, err := p.parseExpression(0)
			if err != nil {
				return luaStmt{}, err
			}
			targets = append(targets, target)
		}
	}
	if !p.accept("=") {
		if local {
			return luaStmt{kind: 'a', names: names}, nil
		}
		return luaStmt{kind: 'x', exprs: targets}, nil
	}
	var exprs []*luaExpr
	for {
		expr, err := p.parseExpression(0)
		if err != nil {
			return luaStmt{}, err
		}
		exprs = append(exprs, expr)
		if !p.accept(",") {
			break
		}
	}
	return luaStmt{kind: 'a', names: names, targets: targets, exprs: exprs}, nil
}

func (p *luaParser) parseExpression(minPrecedence int) (*luaExpr, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}
	for {
		op := p.current().text
		precedence, rightAssociative := luaPrecedence(op)
		if precedence < minPrecedence {
			break
		}
		p.next()
		next := precedence + 1
		if rightAssociative {
			next = precedence
		}
		right, err := p.parseExpression(next)
		if err != nil {
			return nil, err
		}
		left = &luaExpr{kind: 'b', op: op, left: left, right: right}
	}
	return left, nil
}

func (p *luaParser) parsePrefix() (*luaExpr, error) {
	var expr *luaExpr
	switch p.current().kind {
	case 's':
		expr = &luaExpr{kind: 'v', value: p.current().text}
		p.next()
	case 'n':
		value, err := strconv.ParseFloat(p.current().text, 64)
		if err != nil {
			return nil, p.errorf("数字无效")
		}
		expr = &luaExpr{kind: 'v', value: value}
		p.next()
	case 'i':
		switch p.current().text {
		case "true":
			expr = &luaExpr{kind: 'v', value: true}
			p.next()
		case "false":
			expr = &luaExpr{kind: 'v', value: false}
			p.next()
		case "nil":
			expr = &luaExpr{kind: 'v', value: nil}
			p.next()
		case "not":
			p.next()
			operand, err := p.parseExpression(7)
			if err != nil {
				return nil, err
			}
			expr = &luaExpr{kind: 'u', op: "not", operand: operand}
		default:
			expr = &luaExpr{kind: 'n', name: p.current().text}
			p.next()
		}
	case '-':
		p.next()
		operand, err := p.parseExpression(7)
		if err != nil {
			return nil, err
		}
		expr = &luaExpr{kind: 'u', op: "-", operand: operand}
	case '#':
		p.next()
		operand, err := p.parseExpression(7)
		if err != nil {
			return nil, err
		}
		expr = &luaExpr{kind: 'u', op: "#", operand: operand}
	case '(':
		p.next()
		var err error
		expr, err = p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		if !p.accept(")") {
			return nil, p.errorf("括号未闭合")
		}
	case '{':
		var err error
		expr, err = p.parseTable()
		if err != nil {
			return nil, err
		}
	default:
		return nil, p.errorf("不支持的表达式 %q", p.current().text)
	}

	for {
		switch {
		case p.accept("."):
			name, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			expr = &luaExpr{kind: 'q', receiver: expr, key: &luaExpr{kind: 'v', value: name}}
		case p.accept("["):
			key, err := p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			if !p.accept("]") {
				return nil, p.errorf("索引缺少 ]")
			}
			expr = &luaExpr{kind: 'q', receiver: expr, key: key}
		case p.accept(":"):
			name, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			if !p.accept("(") {
				return nil, p.errorf("方法调用缺少参数列表")
			}
			args, err := p.parseArgumentsAfterOpen()
			if err != nil {
				return nil, err
			}
			args = append([]*luaExpr{expr}, args...)
			expr = &luaExpr{kind: 'c', receiver: &luaExpr{kind: 'q', receiver: expr, key: &luaExpr{kind: 'v', value: name}}, args: args}
		case p.accept("("):
			args, err := p.parseArgumentsAfterOpen()
			if err != nil {
				return nil, err
			}
			expr = &luaExpr{kind: 'c', receiver: expr, args: args}
		default:
			return expr, nil
		}
	}
}

func (p *luaParser) parseArgumentsAfterOpen() ([]*luaExpr, error) {
	var args []*luaExpr
	if p.accept(")") {
		return args, nil
	}
	for {
		arg, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.accept(")") {
			return args, nil
		}
		if !p.accept(",") {
			return nil, p.errorf("函数参数缺少逗号")
		}
	}
}

func (p *luaParser) parseTable() (*luaExpr, error) {
	p.next()
	expr := &luaExpr{kind: 't'}
	for !p.accept("}") {
		if p.current().kind == 0 {
			return nil, p.errorf("表缺少 }")
		}
		if p.accept(",") || p.accept(";") {
			continue
		}
		field := luaTableField{}
		if p.accept("[") {
			key, err := p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			if !p.accept("]") || !p.accept("=") {
				return nil, p.errorf("表索引赋值无效")
			}
			field.key = key
		} else if p.current().kind == 'i' && p.peek().text == "=" {
			field.name = p.current().text
			p.next()
			p.next()
		}
		value, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		field.value = value
		expr.fields = append(expr.fields, field)
		p.accept(",")
		p.accept(";")
	}
	return expr, nil
}

func luaPrecedence(op string) (int, bool) {
	switch op {
	case "or":
		return 1, false
	case "and":
		return 2, false
	case "==", "~=", "<", ">", "<=", ">=":
		return 3, false
	case "..":
		return 4, true
	case "+", "-":
		return 5, false
	case "*", "/", "%":
		return 6, false
	case "^":
		return 7, true
	default:
		return -1, false
	}
}

func (p *luaParser) current() luaToken {
	if p.pos >= len(p.tokens) {
		return luaToken{}
	}
	return p.tokens[p.pos]
}

func (p *luaParser) peek() luaToken {
	if p.pos+1 >= len(p.tokens) {
		return luaToken{}
	}
	return p.tokens[p.pos+1]
}

func (p *luaParser) next() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

func (p *luaParser) accept(text string) bool {
	if p.current().text != text {
		return false
	}
	p.next()
	return true
}

func (p *luaParser) expectIdentifier() (string, error) {
	if p.current().kind != 'i' {
		return "", p.errorf("预期标识符")
	}
	text := p.current().text
	p.next()
	return text, nil
}

func (p *luaParser) errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func executeLuaStatements(statements []luaStmt, env *luaEnv) error {
	for _, stmt := range statements {
		if err := countLuaStep(env); err != nil {
			return err
		}
		signal, err := executeLuaStatement(stmt, env)
		if err != nil {
			return err
		}
		if signal != "" {
			return nil
		}
	}
	return nil
}

func executeLuaStatement(stmt luaStmt, env *luaEnv) (string, error) {
	switch stmt.kind {
	case 'a':
		values := make([]any, len(stmt.exprs))
		for i, expr := range stmt.exprs {
			value, err := evalLuaExpr(expr, env)
			if err != nil {
				return "", err
			}
			values[i] = value
		}
		if len(stmt.names) > 0 {
			for i, name := range stmt.names {
				var value any
				if i < len(values) {
					value = values[i]
				}
				env.define(name, value)
			}
		} else {
			for i, target := range stmt.targets {
				var value any
				if i < len(values) {
					value = values[i]
				}
				if err := assignLuaTarget(target, value, env); err != nil {
					return "", err
				}
			}
		}
	case 'f':
		env.define(stmt.names[0], &luaFunction{params: stmt.function.params, body: stmt.function.body, owner: env})
	case 'i':
		for _, branch := range stmt.branches {
			condition, err := evalLuaExpr(branch.condition, env)
			if err != nil {
				return "", err
			}
			if luaTruthy(condition) {
				return executeLuaBlock(branch.body, env)
			}
		}
		return executeLuaBlock(stmt.elseBody, env)
	case 'n':
		start, err := evalLuaExpr(stmt.forStart, env)
		if err != nil {
			return "", err
		}
		end, err := evalLuaExpr(stmt.forEnd, env)
		if err != nil {
			return "", err
		}
		step := 1.0
		if stmt.forStep != nil {
			value, err := evalLuaExpr(stmt.forStep, env)
			if err != nil {
				return "", err
			}
			if n, ok := luaNumber(value); ok {
				step = n
			}
		}
		first, ok := luaNumber(start)
		if !ok {
			return "", fmt.Errorf("数字 for 的起点不是数字")
		}
		last, ok := luaNumber(end)
		if !ok || step == 0 {
			return "", fmt.Errorf("数字 for 的终点或步长无效")
		}
		iterations := 0
		for value := first; (step > 0 && value <= last) || (step < 0 && value >= last); value += step {
			iterations++
			if iterations > luaMaxLoopSteps {
				return "", fmt.Errorf("modinfo.lua 循环次数过多")
			}
			env.define(stmt.forName, value)
			signal, err := executeLuaBlock(stmt.body, env)
			if err != nil || signal == "return" {
				return signal, err
			}
			if signal == "break" {
				break
			}
		}
	case 'g':
		value, err := evalLuaExpr(stmt.forExpr, env)
		if err != nil {
			return "", err
		}
		table, ok := value.(*luaTable)
		if !ok {
			return "", fmt.Errorf("不支持的 Lua 迭代器")
		}
		for i, item := range table.array {
			env.define(stmt.forKey, float64(i+1))
			if stmt.forValue != "" {
				env.define(stmt.forValue, item)
			}
			signal, err := executeLuaBlock(stmt.body, env)
			if err != nil || signal == "return" {
				return signal, err
			}
			if signal == "break" {
				break
			}
		}
	case 'w':
		for iterations := 0; iterations < luaMaxLoopSteps; iterations++ {
			condition, err := evalLuaExpr(stmt.exprs[0], env)
			if err != nil {
				return "", err
			}
			if !luaTruthy(condition) {
				break
			}
			signal, err := executeLuaBlock(stmt.body, env)
			if err != nil || signal == "return" {
				return signal, err
			}
			if signal == "break" {
				break
			}
		}
	case 'r':
		if len(stmt.exprs) == 0 {
			return "return", nil
		}
		value, err := evalLuaExpr(stmt.exprs[0], env)
		if err != nil {
			return "", err
		}
		env.define("__return", value)
		return "return", nil
	case 'b':
		return "break", nil
	case 'd':
		return executeLuaBlock(stmt.body, env)
	case 'x':
		for _, expr := range stmt.exprs {
			if _, err := evalLuaExpr(expr, env); err != nil {
				return "", err
			}
		}
	}
	return "", nil
}

func executeLuaBlock(statements []luaStmt, env *luaEnv) (string, error) {
	for _, stmt := range statements {
		if err := countLuaStep(env); err != nil {
			return "", err
		}
		signal, err := executeLuaStatement(stmt, env)
		if err != nil || signal != "" {
			return signal, err
		}
	}
	return "", nil
}

func evalLuaExpr(expr *luaExpr, env *luaEnv) (any, error) {
	switch expr.kind {
	case 'v':
		return expr.value, nil
	case 'n':
		value, _ := env.get(expr.name)
		return value, nil
	case 't':
		table := &luaTable{fields: map[string]any{}}
		for _, field := range expr.fields {
			value, err := evalLuaExpr(field.value, env)
			if err != nil {
				return nil, err
			}
			if field.name != "" {
				table.fields[field.name] = value
				continue
			}
			if field.key != nil {
				key, err := evalLuaExpr(field.key, env)
				if err != nil {
					return nil, err
				}
				table.setIndex(key, value)
			} else {
				table.array = append(table.array, value)
			}
		}
		if len(table.array)+len(table.fields) > luaMaxTableItems {
			return nil, fmt.Errorf("modinfo.lua 表项过多")
		}
		return table, nil
	case 'q':
		receiver, err := evalLuaExpr(expr.receiver, env)
		if err != nil {
			return nil, err
		}
		key, err := evalLuaExpr(expr.key, env)
		if err != nil {
			return nil, err
		}
		return luaIndex(receiver, key), nil
	case 'c':
		fn, err := evalLuaExpr(expr.receiver, env)
		if err != nil {
			return nil, err
		}
		args := make([]any, len(expr.args))
		for i, arg := range expr.args {
			args[i], err = evalLuaExpr(arg, env)
			if err != nil {
				return nil, err
			}
		}
		return callLuaFunction(fn, args, env)
	case 'u':
		value, err := evalLuaExpr(expr.operand, env)
		if err != nil {
			return nil, err
		}
		switch expr.op {
		case "not":
			return !luaTruthy(value), nil
		case "-":
			n, ok := luaNumber(value)
			if !ok {
				return nil, fmt.Errorf("一元负号的值不是数字")
			}
			return -n, nil
		case "#":
			switch value := value.(type) {
			case *luaTable:
				return float64(len(value.array)), nil
			case string:
				return float64(len([]rune(value))), nil
			default:
				return float64(0), nil
			}
		}
	case 'b':
		if expr.op == "and" {
			left, err := evalLuaExpr(expr.left, env)
			if err != nil {
				return nil, err
			}
			if !luaTruthy(left) {
				return left, nil
			}
			return evalLuaExpr(expr.right, env)
		}
		if expr.op == "or" {
			left, err := evalLuaExpr(expr.left, env)
			if err != nil {
				return nil, err
			}
			if luaTruthy(left) {
				return left, nil
			}
			return evalLuaExpr(expr.right, env)
		}
		left, err := evalLuaExpr(expr.left, env)
		if err != nil {
			return nil, err
		}
		right, err := evalLuaExpr(expr.right, env)
		if err != nil {
			return nil, err
		}
		return evalLuaBinary(expr.op, left, right)
	}
	return nil, fmt.Errorf("未知表达式")
}

func evalLuaBinary(op string, left, right any) (any, error) {
	switch op {
	case "==", "~=":
		equal := luaEqual(left, right)
		if op == "~=" {
			return !equal, nil
		}
		return equal, nil
	case "<", ">", "<=", ">=":
		if a, ok := luaNumber(left); ok {
			if b, ok := luaNumber(right); ok {
				switch op {
				case "<":
					return a < b, nil
				case ">":
					return a > b, nil
				case "<=":
					return a <= b, nil
				case ">=":
					return a >= b, nil
				}
			}
		}
		return false, nil
	case "..":
		return luaString(left) + luaString(right), nil
	case "+", "-", "*", "/", "%", "^":
		a, ok := luaNumber(left)
		if !ok {
			return nil, fmt.Errorf("运算 %s 左值不是数字", op)
		}
		b, ok := luaNumber(right)
		if !ok {
			return nil, fmt.Errorf("运算 %s 右值不是数字", op)
		}
		switch op {
		case "+":
			return a + b, nil
		case "-":
			return a - b, nil
		case "*":
			return a * b, nil
		case "/":
			return a / b, nil
		case "%":
			return math.Mod(a, b), nil
		case "^":
			return math.Pow(a, b), nil
		}
	}
	return nil, fmt.Errorf("不支持的运算符 %s", op)
}

func assignLuaTarget(target *luaExpr, value any, env *luaEnv) error {
	if target.kind == 'n' {
		env.set(target.name, value)
		return nil
	}
	if target.kind != 'q' {
		return fmt.Errorf("赋值目标无效")
	}
	receiver, err := evalLuaExpr(target.receiver, env)
	if err != nil {
		return err
	}
	key, err := evalLuaExpr(target.key, env)
	if err != nil {
		return err
	}
	table, ok := receiver.(*luaTable)
	if !ok {
		return nil
	}
	table.setIndex(key, value)
	return nil
}

func callLuaFunction(value any, args []any, env *luaEnv) (any, error) {
	switch fn := value.(type) {
	case luaBuiltin:
		return fn(args), nil
	case *luaFunction:
		if env.depth >= luaMaxCallDepth {
			return nil, fmt.Errorf("modinfo.lua 函数调用层级过深")
		}
		if !fn.parsed {
			tokens := append([]luaToken{}, fn.body...)
			tokens = append(tokens, luaToken{kind: 0})
			parser := &luaParser{tokens: tokens}
			code, err := parser.parseBlock(nil)
			if err != nil {
				return nil, err
			}
			fn.code = code
			fn.parsed = true
		}
		child := &luaEnv{values: map[string]any{}, parent: fn.owner, steps: env.steps, depth: env.depth + 1}
		for i, name := range fn.params {
			if i < len(args) {
				child.define(name, args[i])
			} else {
				child.define(name, nil)
			}
		}
		signal, err := executeLuaBlock(fn.code, child)
		if err != nil {
			return nil, err
		}
		if signal == "return" {
			value, _ := child.get("__return")
			return value, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("不支持的 Lua 函数调用")
	}
}

func configOptionsFromTable(table *luaTable) ([]ConfigOption, error) {
	var options []ConfigOption
	for _, value := range table.array {
		entry, ok := value.(*luaTable)
		if !ok {
			continue
		}
		name, _ := entry.fieldString("name")
		if name == "" {
			continue
		}
		option := ConfigOption{
			Name:        name,
			Label:       entry.fieldText("label"),
			Type:        entry.fieldText("type"),
			Description: entry.fieldText("description"),
		}
		if option.Description == "" {
			option.Description = entry.fieldText("hover")
		}
		if value, exists := entry.field("default"); exists {
			var known bool
			option.Default, known = luaScalarString(value)
			option.DefaultUnresolved = !known
		} else {
			option.DefaultUnresolved = true
		}
		if value, exists := entry.field("min"); exists {
			option.Min, _ = luaScalarString(value)
		}
		if value, exists := entry.field("max"); exists {
			option.Max, _ = luaScalarString(value)
		}
		if value, exists := entry.field("options"); exists {
			choices, err := configChoicesFromValue(value)
			if err != nil {
				return nil, err
			}
			option.Choices = choices
			for _, choice := range choices {
				option.Options = append(option.Options, choice.Data)
			}
		}
		if len(option.Choices) == 1 && option.Choices[0].Description == "" {
			continue
		}
		if option.Type == "" && len(option.Options) == 0 {
			switch option.Default {
			case "true", "false":
				option.Type = "boolean"
			default:
				if numberLiteralRe.MatchString(option.Default) {
					option.Type = "number"
				}
			}
		}
		options = append(options, option)
	}
	return options, nil
}

func configChoicesFromValue(value any) ([]OptionChoice, error) {
	table, ok := value.(*luaTable)
	if !ok {
		return nil, nil
	}
	choices := make([]OptionChoice, 0, len(table.array))
	for _, item := range table.array {
		if entry, ok := item.(*luaTable); ok {
			data, exists := entry.field("data")
			if !exists {
				continue
			}
			dataText, ok := luaScalarString(data)
			if !ok {
				continue
			}
			description := entry.fieldText("description")
			choices = append(choices, OptionChoice{Data: dataText, Description: description})
			continue
		}
		dataText, ok := luaScalarString(item)
		if ok {
			choices = append(choices, OptionChoice{Data: dataText, Description: dataText})
		}
	}
	return choices, nil
}

func (e *luaEnv) get(name string) (any, bool) {
	if value, ok := e.values[name]; ok {
		return value, true
	}
	if e.parent != nil {
		return e.parent.get(name)
	}
	return nil, false
}

func (e *luaEnv) define(name string, value any) {
	e.values[name] = value
}

func (e *luaEnv) set(name string, value any) {
	if _, ok := e.values[name]; ok {
		e.values[name] = value
		return
	}
	if e.parent != nil {
		e.parent.set(name, value)
		return
	}
	e.values[name] = value
}

func (e *luaEnv) step() error {
	(*e.steps)++
	if *e.steps > luaMaxSteps {
		return fmt.Errorf("modinfo.lua 执行步数过多")
	}
	return nil
}

func countLuaStep(env *luaEnv) error {
	return env.step()
}

func (t *luaTable) field(name string) (any, bool) {
	value, ok := t.fields[name]
	return value, ok
}

func (t *luaTable) fieldString(name string) (string, bool) {
	value, ok := t.field(name)
	if !ok {
		return "", false
	}
	text, ok := luaScalarString(value)
	return text, ok
}

func (t *luaTable) fieldText(name string) string {
	value, ok := t.field(name)
	if !ok {
		return ""
	}
	return luaString(value)
}

func (t *luaTable) setIndex(key, value any) {
	if name, ok := key.(string); ok {
		t.fields[name] = value
		return
	}
	if number, ok := luaNumber(key); ok && number >= 1 && number == math.Trunc(number) {
		index := int(number)
		if index > luaMaxTableItems {
			return
		}
		for len(t.array) < index {
			t.array = append(t.array, nil)
		}
		t.array[index-1] = value
		return
	}
}

func luaIndex(value, key any) any {
	table, ok := value.(*luaTable)
	if ok {
		if name, ok := key.(string); ok {
			return table.fields[name]
		}
		if number, ok := luaNumber(key); ok && number >= 1 && number <= float64(len(table.array)) {
			return table.array[int(number)-1]
		}
	}
	if text, ok := value.(string); ok {
		if name, ok := key.(string); ok {
			switch name {
			case "upper":
				return luaBuiltin(func([]any) any { return strings.ToUpper(text) })
			case "format":
				return luaBuiltin(func(args []any) any { return formatLuaString(text, args[1:]) })
			case "find":
				return luaBuiltin(func(args []any) any {
					if len(args) > 1 && strings.Contains(text, luaString(args[1])) {
						return true
					}
					return nil
				})
			case "sub":
				return luaBuiltin(func(args []any) any {
					runes := []rune(text)
					start, end := 1, len(runes)
					if len(args) > 1 {
						if n, ok := luaNumber(args[1]); ok {
							start = int(n)
						}
					}
					if len(args) > 2 {
						if n, ok := luaNumber(args[2]); ok {
							end = int(n)
						}
					}
					if start < 1 {
						start = 1
					}
					if end < 0 {
						end = len(runes) + end + 1
					}
					if start > len(runes) || end < start {
						return ""
					}
					if end > len(runes) {
						end = len(runes)
					}
					return string(runes[start-1 : end])
				})
			}
		}
	}
	return nil
}

func formatLuaString(format string, args []any) string {
	var out strings.Builder
	arg := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			continue
		}
		start := i
		i++
		if format[i] == '%' {
			out.WriteByte('%')
			continue
		}
		for i < len(format) && strings.ContainsRune("-+ #0", rune(format[i])) {
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		if i < len(format) && format[i] == '.' {
			i++
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		if i >= len(format) {
			out.WriteString(format[start:])
			break
		}
		var value any
		if arg < len(args) {
			value = args[arg]
			arg++
		}
		spec := format[start : i+1]
		switch format[i] {
		case 's':
			out.WriteString(fmt.Sprintf(spec, luaString(value)))
		case 'd', 'i':
			number, ok := luaNumber(value)
			if !ok {
				out.WriteString(spec)
				continue
			}
			if format[i] == 'i' {
				spec = spec[:len(spec)-1] + "d"
			}
			out.WriteString(fmt.Sprintf(spec, int64(number)))
		case 'f', 'g', 'e':
			number, ok := luaNumber(value)
			if !ok {
				out.WriteString(spec)
				continue
			}
			out.WriteString(fmt.Sprintf(spec, number))
		default:
			out.WriteString(spec)
		}
	}
	return out.String()
}

func luaNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case string:
		number, err := strconv.ParseFloat(value, 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func luaScalarString(value any) (string, bool) {
	switch value := value.(type) {
	case nil:
		return "", false
	case string:
		return value, true
	case bool:
		if value {
			return "true", true
		}
		return "false", true
	default:
		if number, ok := luaNumber(value); ok {
			if number == math.Trunc(number) {
				return strconv.FormatInt(int64(number), 10), true
			}
			return strconv.FormatFloat(number, 'f', -1, 64), true
		}
		return "", false
	}
}

func luaString(value any) string {
	if text, ok := luaScalarString(value); ok {
		return text
	}
	return ""
}

func luaTruthy(value any) bool {
	return value != nil && value != false
}

func luaEqual(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if a, ok := luaNumber(left); ok {
		if b, ok := luaNumber(right); ok {
			return a == b
		}
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}
