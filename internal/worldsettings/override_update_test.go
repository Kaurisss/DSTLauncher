package worldsettings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrdinaryUpdateAddsTableAndChangesPresets(t *testing.T) {
	root := t.TempDir()
	path := FilePath(root, "Master")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	const original = `return {override_enabled=true, preset='LEGACY', unknown_top={count=4}}`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	o, _, err := ReadFile(root, "Master", "forest")
	if err != nil {
		t.Fatal(err)
	}
	o.Overrides["weather"] = "often"
	if err := UpdateFile(root, "Master", o); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "preset='LEGACY', unknown_top={count=4}") {
		t.Fatalf("未修改的根字段丢失: %s", b)
	}
	o.WorldGenPreset = "SURVIVAL_TOGETHER"
	if err := UpdateFile(root, "Master", o); err != nil {
		t.Fatal(err)
	}
	got, _, err := ReadFile(root, "Master", "forest")
	if err != nil {
		t.Fatal(err)
	}
	if got.WorldGenPreset != "SURVIVAL_TOGETHER" || got.SettingsPreset != "LEGACY" || got.Overrides["weather"] != "often" {
		t.Fatalf("更新预设或新表失败: %+v", got)
	}
}

func TestOrdinaryUpdateRejectsUnsafeFileWithoutWriting(t *testing.T) {
	root := t.TempDir()
	path := FilePath(root, "Master")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	const original = `return {override_enabled=true, overrides={unknown=choose()}}`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadFile(root, "Master", "forest"); err == nil {
		t.Error("不能静默忽略动态表达式")
	}
	if err := UpdateFile(root, "Master", NewOverrideFor("forest")); err == nil {
		t.Error("不能覆盖无法安全解析的文件")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != original {
		t.Fatal("拒绝时源文件改变")
	}
}

func TestImportedUnknownLiteralsKeepTypes(t *testing.T) {
	const source = `return {override_enabled=true,overrides={unknown_bool=false,unknown_num=42,unknown_table={"a",true},["dotted.key"]='custom'}}`
	parsed := ParseOverride(source)
	out := parsed.String()
	for _, want := range []string{`["unknown_bool"] = false`, `["unknown_num"] = 42`, `["unknown_table"] = {"a",true}`, `["dotted.key"] = 'custom'`} {
		if !strings.Contains(out, want) {
			t.Errorf("导入后重生成未保留 %s: %s", want, out)
		}
	}
	if _, _, err := readOverride(out); err != nil {
		t.Fatal(err)
	}
}
