package modmgr

import "testing"

// TestParseOverridesRoundTrip 覆盖曾经的回归缺陷：
// 用非贪婪正则提取模组块时，configuration_options 的右大括号会提前截断模组体，
// 导致参数丢失并解析出一个名为 "configuration_options" 的伪配置项。
func TestParseOverridesRoundTrip(t *testing.T) {
	src := `return {
    ["workshop-1234567890"] = {
        enabled = true,
        priority = 3,
        configuration_options = {
            difficulty = 3,
            enable_boss = false,
            mode = "Easy"
        },
    },
    ["workshop-9876543210"] = {
        enabled = false,
    },
}`

	o := ParseOverrides(src)
	if len(o.Order) != 2 {
		t.Fatalf("模组数量错误: %v", o.Order)
	}
	if !o.Enabled["workshop-1234567890"] {
		t.Error("第一个模组应为启用")
	}
	if o.Enabled["workshop-9876543210"] {
		t.Error("第二个模组应为禁用")
	}
	if o.Priority["workshop-1234567890"] != 3 {
		t.Errorf("priority 解析错误: %v", o.Priority)
	}

	cfg := o.Config["workshop-1234567890"]
	if cfg == nil {
		t.Fatal("configuration_options 未解析")
	}
	if _, bogus := cfg["configuration_options"]; bogus {
		t.Errorf("解析出了伪配置项: %v", cfg)
	}
	if cfg["difficulty"] != float64(3) {
		t.Errorf("difficulty = %v", cfg["difficulty"])
	}
	if cfg["enable_boss"] != false {
		t.Errorf("enable_boss = %v", cfg["enable_boss"])
	}
	if cfg["mode"] != "Easy" {
		t.Errorf("mode = %v", cfg["mode"])
	}

	// 序列化后再解析一次，结果必须一致（写读幂等）
	o2 := ParseOverrides(o.String())
	if !o2.Enabled["workshop-1234567890"] || o2.Enabled["workshop-9876543210"] {
		t.Error("往返后启用状态不一致")
	}
	cfg2 := o2.Config["workshop-1234567890"]
	if cfg2 == nil {
		t.Fatal("往返后配置丢失")
	}
	if _, bogus := cfg2["configuration_options"]; bogus {
		t.Errorf("往返后出现伪配置项: %v", cfg2)
	}
	if cfg2["difficulty"] != float64(3) || cfg2["enable_boss"] != false || cfg2["mode"] != "Easy" {
		t.Errorf("往返后配置不一致: %v", cfg2)
	}
	if o2.Priority["workshop-1234567890"] != 3 {
		t.Errorf("往返后 priority 丢失: %v", o2.Priority)
	}
}

// TestParseOverridesIgnoresNonWorkshopKeys 确认本地模组（非 workshop- 前缀）也能识别。
func TestParseOverridesIgnoresNonWorkshopKeys(t *testing.T) {
	o := ParseOverrides(`return {
    ["my_local_mod"] = {
        enabled = true,
        configuration_options = { level = 5 },
    },
}`)
	if !o.Enabled["my_local_mod"] {
		t.Fatalf("本地模组未解析: %+v", o.Enabled)
	}
	if o.Config["my_local_mod"]["level"] != float64(5) {
		t.Errorf("本地模组配置错误: %v", o.Config["my_local_mod"])
	}
}

// TestParseConfigOptionNestedOptions 覆盖 description 被嵌套 options 污染的问题：
// options 里每个候选项都有 description，不能被当成配置项自身的描述。
func TestParseConfigOptionNestedOptions(t *testing.T) {
	item := `{
        name = "difficulty",
        label = "难度",
        options = {
            {description = "简单", data = 1},
            {description = "普通", data = 2},
            {description = "困难", data = 3},
        },
        default = 2,
    }`

	opt := parseConfigOption(item)
	if opt.Name != "difficulty" {
		t.Errorf("name = %q", opt.Name)
	}
	if opt.Label != "难度" {
		t.Errorf("label = %q", opt.Label)
	}
	if opt.Description != "" {
		t.Errorf("description 被 options 污染: %q", opt.Description)
	}
	if opt.Default != "2" {
		t.Errorf("default = %q（数字默认值应能解析）", opt.Default)
	}
	if len(opt.Options) != 3 || opt.Options[0] != "1" || opt.Options[2] != "3" {
		t.Errorf("options = %v（应取 data 值）", opt.Options)
	}
	if len(opt.Choices) != 3 || opt.Choices[0].Description != "简单" || opt.Choices[2].Description != "困难" {
		t.Errorf("choices = %+v（应保留中文候选项描述）", opt.Choices)
	}
}

func TestParseConfigSchemaChinesePatterns(t *testing.T) {
	src := `
local L = locale ~= "zh"
local function en_zh(en, zh) return zh end
local options_enable = {
    {description = en_zh("Disabled", "关闭"), data = false},
    {description = en_zh("Enabled", "开启"), data = true},
}
configuration_options = L and {
    {name = "Title", label = "Section", options = {{description = "", data = ""}}, default = ""},
    {
        name = "lang",
        label = "Language",
        hover = "Set mod language.",
        options = {
            {description = "English", data = "en"},
            {description = "Chinese", data = "zh"},
        },
        default = "en",
    },
} or {
    {name = "Title", label = "特殊设置", options = {{description = "", data = ""}}, default = ""},
    {
        name = "lang",
        label = "设置语言",
        hover = "设置模组语言",
        options = {
            -- {description = "自动", data = "auto"},
            {description = "英文", data = "en"},
            {description = "中文", data = "zh"},
        },
        default = "zh",
    },
    {
        name = "newloot",
        label = en_zh("Additional Treasures", "额外宝藏"),
        hover = en_zh("Treasures from SW", "来自海难的额外宝藏"),
    },
}
for i = 1, #configuration_options do
    configuration_options[i].options = configuration_options[i].options or options_enable
    configuration_options[i].default = configuration_options[i].default == nil and true or configuration_options[i].default
end
`
	opts, err := parseConfigSchemaText(src)
	if err != nil {
		t.Fatalf("parseConfigSchemaText 失败: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("应跳过伪标题并解析出 2 个配置项，实际: %+v", opts)
	}
	if opts[0].Label != "设置语言" || opts[0].Description != "设置模组语言" {
		t.Errorf("应选中 or 后的中文分支并提取 hover: %+v", opts[0])
	}
	if len(opts[0].Choices) != 2 || opts[0].Choices[0].Description != "英文" || opts[0].Choices[1].Description != "中文" {
		t.Errorf("应跳过注释掉的候选项并保留中文描述: %+v", opts[0].Choices)
	}
	if opts[1].Label != "额外宝藏" || opts[1].Description != "来自海难的额外宝藏" || opts[1].Default != "true" {
		t.Errorf("应解析 en_zh 与尾部默认回填: %+v", opts[1])
	}
	if len(opts[1].Choices) != 2 || opts[1].Choices[0].Description != "关闭" || opts[1].Choices[1].Description != "开启" {
		t.Errorf("应回填顶层 options_enable 中文候选: %+v", opts[1].Choices)
	}
}

// TestParseConfigOptionScalarTypes 覆盖数字/布尔/上下限的解析。
func TestParseConfigOptionScalarTypes(t *testing.T) {
	item := `{
        name = "spawn_rate",
        label = "刷新倍率",
        type = "number",
        default = 1.5,
        min = 0,
        max = 10,
        description = "世界刷新倍率",
    }`
	opt := parseConfigOption(item)
	if opt.Default != "1.5" || opt.Min != "0" || opt.Max != "10" {
		t.Errorf("default/min/max = %q/%q/%q", opt.Default, opt.Min, opt.Max)
	}
	if opt.Description != "世界刷新倍率" {
		t.Errorf("description = %q", opt.Description)
	}
	if opt.Type != "number" {
		t.Errorf("type = %q", opt.Type)
	}
}

func TestConfigSchemaLiteralDefaultsAndNilFallback(t *testing.T) {
	src := `configuration_options = {
    {name="font", default=FONTS[1]},
    {name="font_name", default=FONTS["zh"]},
    {name="computed", default=not false},
    {name="empty", default=""},
    {name="disabled", default=false},
    {name="zero", default=0},
    {name="escaped", default="say \"hi\""},
    {name="missing"},
    {name="nil_default", default=nil},
}
for i=1,#configuration_options do
    configuration_options[i].default = configuration_options[i].default == nil and true or configuration_options[i].default
end`
	opts, err := parseConfigSchemaText(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		value      string
		unresolved bool
	}{
		{"", true}, {"", true}, {"", true}, {"", false}, {"false", false},
		{"0", false}, {`say "hi"`, false}, {"true", false}, {"true", false},
	}
	if len(opts) != len(want) {
		t.Fatalf("配置项丢失: %+v", opts)
	}
	for i, expected := range want {
		if opts[i].Default != expected.value || opts[i].DefaultUnresolved != expected.unresolved {
			t.Errorf("%s: default=%q unresolved=%v，期望 %q/%v", opts[i].Name, opts[i].Default, opts[i].DefaultUnresolved, expected.value, expected.unresolved)
		}
		if expected.unresolved && opts[i].Type != "" {
			t.Errorf("未解析表达式不应推断类型: %+v", opts[i])
		}
	}
}

func TestParseDynamicConfigSchemaFunctionCalls(t *testing.T) {
	src := `
local function AddOption(data, description)
    return {data = data, description = description}
end
local BoolOpt = {
    AddOption(false, "关闭"),
    AddOption(true, "开启"),
}
local function AddConfig(name, label, default, options)
    return {name = name, label = label, default = default, options = options}
end
configuration_options = {
    AddConfig("mode", "模式", 1, {
        AddOption(1, "简单"),
        AddOption(2, "困难"),
    }),
    AddConfig("enabled", "启用", true, BoolOpt),
}`
	options, err := parseConfigSchemaText(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 {
		t.Fatalf("配置项数量错误: %+v", options)
	}
	if options[0].Name != "mode" || options[0].Default != "1" || len(options[0].Choices) != 2 {
		t.Fatalf("函数生成的配置项解析错误: %+v", options[0])
	}
	if options[1].Name != "enabled" || options[1].Type != "" || len(options[1].Choices) != 2 || options[1].Choices[1].Description != "开启" {
		t.Fatalf("局部选项表解析错误: %+v", options[1])
	}
}

func TestParseDynamicConfigSchemaLoopOptions(t *testing.T) {
	src := `
local options = {}
for i = 0, 2 do
    options[i + 1] = {description = "" .. (i * 5), data = i * 5}
end
configuration_options = {
    {
        name = "radius",
        label = "半径",
        options = options,
        default = 0,
    },
}`
	options, err := parseConfigSchemaText(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || len(options[0].Choices) != 3 {
		t.Fatalf("循环生成的选项解析错误: %+v", options)
	}
	if options[0].Choices[2].Data != "10" || options[0].Choices[2].Description != "10" {
		t.Fatalf("循环生成的最后一个选项错误: %+v", options[0].Choices)
	}
}

func TestParseDynamicConfigSchemaInitFunction(t *testing.T) {
	src := `
function Option(name, default, options)
    return {name = name, label = STRINGS[name][1], options = options, default = default or false}
end
function Boolean(name, default)
    return Option(name, default, {{data = false, description = STRINGS.FALSE}, {data = true, description = STRINGS.TRUE}})
end
function DoInit(locale)
    STRINGS = LOCALE[locale:upper():sub(0, 2)]
    local options = {
        Boolean("enabled", false),
        Option("mode", 1, {{data = 1, description = STRINGS.MODE[1]}, {data = 2, description = STRINGS.MODE[2]}}),
    }
    if configuration_options == nil then
        configuration_options = options
    end
end
LOCALE = {ZH = {FALSE = "关闭", TRUE = "开启", MODE = {"简单", "困难"}}}
DoInit("zh")`
	options, err := parseConfigSchemaText(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 || options[0].Name != "enabled" || options[0].Choices[1].Description != "开启" {
		t.Fatalf("初始化函数生成的布尔项错误: %+v", options)
	}
	if options[1].Name != "mode" || options[1].Default != "1" || options[1].Choices[1].Description != "困难" {
		t.Fatalf("初始化函数生成的下拉项错误: %+v", options)
	}
}

func TestParseDynamicConfigSchemaStringKeywordAndUnknownDefault(t *testing.T) {
	src := `
function GetDefault()
    return nil
end
local function AddConfig(name)
    return {
        name = name,
        label = "end",
        options = {{data = true, description = "end"}},
        default = GetDefault(),
    }
end
configuration_options = {AddConfig("enabled")}`
	options, err := parseConfigSchemaText(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].Label != "end" || !options[0].DefaultUnresolved {
		t.Fatalf("字符串关键字或未解析默认值错误: %+v", options)
	}
}

func TestParseDynamicConfigSchemaIpairs(t *testing.T) {
	src := `
local values = {}
for i, value in ipairs({1, 2}) do
    values[i] = {data = value, description = tostring(value)}
end
configuration_options = {{name = "mode", options = values, default = 1}}`
	options, err := parseConfigSchemaText(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || len(options[0].Choices) != 2 || options[0].Choices[1].Description != "2" {
		t.Fatalf("ipairs 生成的配置项错误: %+v", options)
	}
}

func TestParseDynamicConfigSchemaReportsUnsupportedFactory(t *testing.T) {
	_, err := parseConfigSchemaText(`configuration_options = unknown_factory()`)
	if err == nil {
		t.Fatal("未知动态工厂应返回解析错误")
	}
}

func TestParseDynamicConfigSchemaKeepsLiteralEmptyConfig(t *testing.T) {
	options, err := parseConfigSchemaText(`unknown_call()
configuration_options = {}`)
	if err != nil || len(options) != 0 {
		t.Fatalf("字面量空配置不应被无关代码影响: options=%+v err=%v", options, err)
	}
}

func TestFormatLuaString(t *testing.T) {
	if got := formatLuaString("%s %d %.2f %%", []any{"value", float64(3), 1.5}); got != "value 3 1.50 %" {
		t.Fatalf("formatLuaString = %q", got)
	}
}

// TestStringEscapesQuotes 确认含引号/反斜杠的取值不会破坏 Lua 语法。
func TestStringEscapesQuotes(t *testing.T) {
	o := NewOverrides()
	o.Order = []string{"workshop-1"}
	o.Enabled["workshop-1"] = true
	o.Config["workshop-1"] = map[string]interface{}{
		"title": `say "hi" \ ok`,
	}
	back := ParseOverrides(o.String())
	if back.Config["workshop-1"]["title"] != `say "hi" \ ok` {
		t.Errorf("转义往返失败: %v", back.Config["workshop-1"])
	}
}

// TestExtractModInfoName 覆盖 modinfo.lua 的三种名字写法，
// 以及曾经修掉的 bug：全局搜第一个 name = "..." 会抓到
// configuration_options 里配置项的名字。
func TestExtractModInfoName(t *testing.T) {
	cases := []struct {
		desc string
		text string
		want string
	}{
		{
			desc: "字面量",
			text: "name = \"Simple Health Bar DST\"\nauthor = \"x\"",
			want: "Simple Health Bar DST",
		},
		{
			desc: "return 表内、带逗号",
			text: "return {\n    name = \"Show Me\",\n    configuration_options = {\n        { name = \"lang\", label = \"语言\" },\n    },\n}",
			want: "Show Me",
		},
		{
			desc: "双语表达式，应取 or 兜底的中文",
			text: "local L = locale ~= \"zh\"\nname = L and \"Show Me (New)\" or \"Show Me (中文)\"\nconfiguration_options = {\n    { name = \"language_switch\", label = \"语言切换\" },\n}",
			want: "Show Me (中文)",
		},
		{
			desc: "条件变量反着写（isCh and 中文 or 英文）",
			text: "local isCh = locale == \"zh\"\nname = isCh and \"能力勋章\" or \"Functional Medal\"",
			want: "能力勋章",
		},
		{
			desc: "整份 modinfo 压成一行，只取 name 的第一个字符串",
			text: "name = \" Simple Health Bar DST\" version = \"2.16\" description = \"Version:DST\"..version..\"\\nHealth bar\" author = \"DYC\"",
			want: "Simple Health Bar DST",
		},
		{
			desc: "配置项写在 name 之前，也不能抓到",
			text: "configuration_options = {\n    { name = \"Title\", label = \"标题\" },\n}\nname = \"真正的名字\"",
			want: "真正的名字",
		},
		{
			desc: "整行注释里的写法示例要跳过",
			text: "-- name = ChooseTranslationTable({ zh = \"注释里的\" })\nname = L and \"A\" or \"棱镜\"",
			want: "棱镜",
		},
		{
			desc: "行尾注释不影响取值",
			text: "name = \"X\" -- 英文名",
			want: "X",
		},
		{
			desc: "配置项的 name 是嵌套表也不能误抓",
			text: "configuration_options = {\n    {\n        name = \"lang\",\n        options = {\n            { description = \"中文\", data = 1 },\n        },\n    },\n}\nname = \"神话书说\"",
			want: "神话书说",
		},
		{
			desc: "没有 name 字段时返回空串（由调用方退回 ID）",
			text: "author = \"x\"\nversion = \"1.0\"",
			want: "",
		},
	}

	for _, c := range cases {
		if got := extractModInfoName(c.text); got != c.want {
			t.Errorf("[%s] extractModInfoName = %q, 期望 %q", c.desc, got, c.want)
		}
	}
}
