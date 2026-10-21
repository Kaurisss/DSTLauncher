package worldsettings

import (
	"os"
	"path/filepath"
	"testing"
)

// 拿真实存档做往返验证：解析后把原值原样写回，文件必须逐字节不变。
//
// 这类文件是游戏自己维护的，任何“顺手格式化”都会让用户下次开服时看到配置漂移，
// 所以这里断言的是字节级一致，而不是语义一致。
func TestRealSaveRoundTripUnchanged(t *testing.T) {
	root := os.Getenv("DST_REAL_SAVE")
	if root == "" {
		t.Skip("设置 DST_REAL_SAVE 指向 <存档根>/DST/Cluster_N 才跑这个用例")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("读取存档目录失败: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), LevelDataFileName)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		ld := ParseLevelData(string(b))
		if ld.Location() == "" {
			t.Errorf("%s 没有解析到 location", path)
			continue
		}
		// 用文件里已有的值原样写回，结果必须与原文完全一致
		out, err := ld.WithOverrides(ld.Overrides)
		if err != nil {
			t.Errorf("%s 写回失败: %v", path, err)
			continue
		}
		if out != string(b) {
			t.Errorf("%s 往返后内容发生变化（%d -> %d 字节）", path, len(b), len(out))
			continue
		}
		if len(ld.Overrides) == 0 {
			t.Errorf("%s 没有解析到任何 overrides", path)
		}
		t.Logf("%s: location=%s overrides=%d", e.Name(), ld.Location(), len(ld.Overrides))
		checked++
	}
	if checked == 0 {
		t.Fatal("没有找到任何 leveldataoverride.lua")
	}
}
