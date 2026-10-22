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

func TestGameWrappedOverrideReadAndUpdate(t *testing.T) {
	for _, prefix := range []string{"KLEI     1 ", "\ufeffKLEI     1 "} {
		t.Run(prefix, func(t *testing.T) {
			root := t.TempDir()
			path := FilePath(root, "Master")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			original := prefix + "return {override_enabled=true, worldgen_preset='SURVIVAL_TOGETHER', overrides={weather='rare', world_size='small', custom={true, 42}}}"
			if err := os.WriteFile(path, []byte(original), 0644); err != nil {
				t.Fatal(err)
			}
			o, exists, err := ReadFile(root, "Master", "forest")
			if err != nil || !exists {
				t.Fatalf("读取游戏生成的配置失败: exists=%v err=%v", exists, err)
			}
			if o.WorldGenPreset != "SURVIVAL_TOGETHER" || o.Overrides["world_size"] != "small" {
				t.Fatalf("配置读取不完整: %+v", o)
			}
			if err := UpdateFile(root, "Master", o); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(path)
			if err != nil || string(b) != original {
				t.Fatalf("无修改时文件发生变化: %v %q", err, b)
			}
			o.Overrides["weather"] = "often"
			if err := UpdateFile(root, "Master", o); err != nil {
				t.Fatal(err)
			}
			b, err = os.ReadFile(path)
			if want := strings.Replace(original, "weather='rare'", "weather='often'", 1); err != nil || string(b) != want {
				t.Fatalf("局部更新破坏了文件头或未修改内容: %v %q", err, b)
			}
		})
	}
}

func TestKleiHeaderKeepsStaticLuaValidation(t *testing.T) {
	for _, source := range []string{
		"KLEI     2 return {overrides={weather='often'}}",
		"KLEI     1 return {overrides={weather=choose()}}",
		"KLEI     1 return {overrides={weather='often'}} os.exit()",
	} {
		if _, _, err := readOverride(source); err == nil {
			t.Errorf("接受了不支持的编码或动态 Lua: %q", source)
		}
	}
}
