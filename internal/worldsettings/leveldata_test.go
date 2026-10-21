package worldsettings

import (
	"strings"
	"testing"
)

// 真实的 leveldataoverride.lua 片段：带多行字符串、嵌套表与布尔值。
const realLevelData = `return {
  background_node_range={ 0, 2 },
  desc="永不结束的海难沙盒模式。\
永远可以在绚丽之门复活。",
  id="SHIPWRECKED_ENDLESS",
  location="shipwrecked",
  overrides={
    bamboo="default",
    basicresource_regrowth="always",
    has_ocean=true,
    keep_disconnected_tiles=true,
    required_prefabs={ "multiplayer_portal" },
    substitutes={  },
  },
  playstyle="endless",
  version=4,
}
`

func TestParseLevelData(t *testing.T) {
	ld := ParseLevelData(realLevelData)
	if got := ld.Location(); got != "shipwrecked" {
		t.Fatalf("location 解析错误: %q", got)
	}
	if ld.Scalar["id"] != "SHIPWRECKED_ENDLESS" || ld.Scalar["playstyle"] != "endless" {
		t.Errorf("顶层标量解析错误: %v", ld.Scalar)
	}
	if ld.Overrides["bamboo"] != "default" || ld.Overrides["basicresource_regrowth"] != "always" {
		t.Errorf("overrides 字符串项解析错误: %v", ld.Overrides)
	}
	if ld.Overrides["has_ocean"] != "true" {
		t.Errorf("overrides 布尔项解析错误: %q", ld.Overrides["has_ocean"])
	}
	// 嵌套表内部不能混进 overrides，否则写回时会破坏结构
	for _, k := range []string{"multiplayer_portal", "override_level_string", "desc"} {
		if _, ok := ld.Overrides[k]; ok {
			t.Errorf("不应解析出 %s: %v", k, ld.Overrides)
		}
	}
}

// 只改点名的键：其他内容逐字保留，尤其是面板清单外的引擎内部字段。
func TestWithOverridesKeepsEverythingElse(t *testing.T) {
	ld := ParseLevelData(realLevelData)
	out, err := ld.WithOverrides(map[string]string{"bamboo": "often", "bush_vine": "never"})
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	for _, want := range []string{
		`bamboo="often"`,
		`bush_vine="never"`,
		`basicresource_regrowth="always"`, // 未点名的保持不变
		"has_ocean=true",
		`required_prefabs={ "multiplayer_portal" }`,
		`id="SHIPWRECKED_ENDLESS"`,
		`location="shipwrecked"`,
		"永远可以在绚丽之门复活",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("写回后丢失 %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `bamboo="default"`) {
		t.Error("被点名的项没有替换")
	}

	// 写回结果必须还能被自己解析回来，且 overrides 数量只按新增项增加
	again := ParseLevelData(out)
	if again.Overrides["bamboo"] != "often" || again.Overrides["bush_vine"] != "never" {
		t.Errorf("往返后取值丢失: %v", again.Overrides)
	}
	if again.Location() != "shipwrecked" || again.Scalar["id"] != "SHIPWRECKED_ENDLESS" {
		t.Error("往返后顶层字段丢失")
	}
	if len(again.Overrides) != len(ld.Overrides)+1 {
		t.Errorf("overrides 项数应只增加 1，实际 %d -> %d", len(ld.Overrides), len(again.Overrides))
	}
}

// 注入字符必须被拒绝，否则可以改写 lua 文件结构。
func TestWithOverridesRejectsInjection(t *testing.T) {
	ld := ParseLevelData(realLevelData)
	for _, bad := range []map[string]string{
		{"bamboo": `"; os.exit() --`},
		{"bad;name": "x"},
		{"1bad": "x"},
	} {
		if _, err := ld.WithOverrides(bad); err == nil {
			t.Errorf("应拒绝 %v", bad)
		}
	}
}

// 没有 overrides 块时明确报错，而不是写出一份损坏的文件。
func TestWithOverridesWithoutBlock(t *testing.T) {
	ld := ParseLevelData(`return { id="X", location="forest" }`)
	if _, err := ld.WithOverrides(map[string]string{"a": "b"}); err == nil {
		t.Error("缺少 overrides 块时应报错")
	}
}
