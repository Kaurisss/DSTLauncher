package modmgr

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func writeIconFixture(t *testing.T, dir, info, atlas string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0755); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		"modinfo.lua": []byte(info), "images/icon.xml": []byte(atlas),
		"images/atlas.tex": texFixture(4, 2, 2, []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255}),
	} {
		if err := os.WriteFile(filepath.Join(dir, path), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestModIconLayoutsAndAtlas(t *testing.T) {
	for _, layout := range []string{"ugc/content/322330/123", "ugc/steamapps/workshop/content/322330/123", "game/mods/workshop-123", "game/mods/local-test"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, filepath.FromSlash(layout))
			writeIconFixture(t, dir, `name='fixture'; icon_atlas="images/icon.xml"; icon='target.tex'; error('do not execute')`,
				`<Atlas><Texture filename="atlas.tex"/><Elements><Element name="other.tex" u1="0.5" u2="1" v1="0" v2="1"/><Element name="target.tex" u1="0" u2="0.5" v1="0.5" v2="1"/></Elements></Atlas>`)
			id := "workshop-123"
			if filepath.Base(dir) == "local-test" {
				id = "local-test"
			}
			m := NewManager("", filepath.Join(root, "game"), filepath.Join(root, "ugc"), nil)
			img, err := m.ModIcon(id)
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 || color.NRGBAModel.Convert(img.At(img.Bounds().Min.X, img.Bounds().Min.Y)) != (color.NRGBA{B: 255, A: 255}) {
				t.Fatalf("应仅返回图集左上角的蓝色图标：%v", img)
			}
		})
	}
}

func TestModIconStandaloneAndUnsafePaths(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "game", "mods", "local-test")
	writeIconFixture(t, dir, `icon_atlas='missing.xml'; icon='images/atlas.tex'`, "")
	m := NewManager("", filepath.Join(root, "game"), filepath.Join(root, "ugc"), nil)
	if img, err := m.ModIcon("local-test"); err != nil || img.Bounds().Dx() != 2 {
		t.Fatalf("缺少 XML 时读取独立 TEX：%v", err)
	}
	for _, id := range []string{"", "..", "../local-test", `..\local-test`, "/local-test", "local-test:", "local-test.", "workshop-../123"} {
		if _, err := m.ModIcon(id); err == nil {
			t.Fatalf("不应接受模组 ID：%q", id)
		}
	}
	for _, info := range []string{`icon_atlas='../../outside.xml'`, `icon_atlas='missing.xml'; icon='../../../outside.tex'`, `icon_atlas='C:/outside.xml'`} {
		if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte(info), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ModIcon("local-test"); err == nil {
			t.Fatalf("不应读取越界路径：%s", info)
		}
	}
	writeIconFixture(t, dir, `icon_atlas='images/icon.xml'; icon='target.tex'`, `<Atlas><Texture filename="atlas.tex"/><Elements><Element name="target.tex" u1="NaN" u2="1" v1="0" v2="1"/></Elements></Atlas>`)
	if _, err := m.ModIcon("local-test"); err == nil {
		t.Fatal("不应接受 NaN UV")
	}
}

func TestModIconRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "game", "mods", "local-test")
	writeIconFixture(t, dir, `icon_atlas='missing.xml'; icon='escape.tex'`, "")
	outside := filepath.Join(root, "outside.tex")
	if err := os.WriteFile(outside, texFixture(4, 1, 1, []byte{255, 0, 0, 255}), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape.tex")); err != nil {
		t.Skipf("当前系统不能创建符号链接：%v", err)
	}
	m := NewManager("", filepath.Join(root, "game"), "", nil)
	if _, err := m.ModIcon("local-test"); err == nil {
		t.Fatal("不能跟随符号链接读取模组目录外的纹理")
	}
}

func TestModIconLanguageTable(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "game", "mods", "local-test")
	writeIconFixture(t, dir, `local atlases = { [zh] = "images/icon.xml", [en] = "missing.xml" }
local icons = { [zh] = "atlas.tex", [en] = "missing.tex" }
icon_atlas = atlases[language]
icon = icons[language]`, `<Atlas><Texture filename="atlas.tex"/><Elements><Element name="atlas.tex" u1="0" u2="1" v1="0" v2="1"/></Elements></Atlas>`)
	m := NewManager("", filepath.Join(root, "game"), "", nil)
	if img, err := m.ModIcon("local-test"); err != nil || img.Bounds().Dx() != 2 {
		t.Fatalf("语言表图标：%v", err)
	}
}
