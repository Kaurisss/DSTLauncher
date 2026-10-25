package modmgr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestShardEditsPreserveOtherWorldAndUnion(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "cluster"), filepath.Join(root, "game"), filepath.Join(root, "ugc"), []string{"Surface", "Below"})
	surface, below := manager.forShard("Surface"), manager.forShard("Below")
	a := ParseOverrides(`return {["workshop-100"]={enabled=true,configuration_options={amount=1}},["local"]={enabled=false,priority=7}}`)
	b := ParseOverrides(`return {["workshop-200"]={enabled=true,configuration_options={amount=9}},["workshop-100"]={enabled=false,configuration_options={amount=2}}}`)
	if err := surface.SaveOverrides(a); err != nil {
		t.Fatal(err)
	}
	if err := below.SaveOverrides(b); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(manager.clusterDir, "Below", "modoverrides.lua")
	before, _ := os.ReadFile(path)
	if err := surface.SetConfig("workshop-100", "amount", float64(3)); err != nil {
		t.Fatal(err)
	}
	if err := surface.SetEnabled("workshop-100", false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("编辑地表改写了洞穴文件")
	}
	got, err := surface.LoadOverrides()
	if err != nil || got.Enabled["workshop-100"] || got.Config["workshop-100"]["amount"] != float64(3) || !reflect.DeepEqual(got.Order, a.Order) || got.Priority["local"] != 7 {
		t.Fatal(got, err)
	}
	ids, err := manager.EnabledWorkshopIDs()
	if err != nil || !reflect.DeepEqual(ids, []string{"200"}) {
		t.Fatal(ids, err)
	}
	if err := below.SetEnabled("workshop-100", true); err != nil {
		t.Fatal(err)
	}
	ids, err = manager.EnabledWorkshopIDs()
	if err != nil || !reflect.DeepEqual(ids, []string{"100", "200"}) {
		t.Fatal(ids, err)
	}
	missing, err := manager.MissingMods()
	if err != nil || !reflect.DeepEqual(missing, []string{"workshop-100", "workshop-200"}) {
		t.Fatal(missing, err)
	}
	if _, err := manager.LoadOverrides(); err == nil {
		t.Fatal("多分片不能隐式读取第一份")
	}
	if err := manager.DeleteMod("workshop-100"); err != nil {
		t.Fatal(err)
	}
	got, err = below.LoadOverrides()
	if err != nil || !got.Enabled["workshop-200"] || got.Config["workshop-200"]["amount"] != float64(9) {
		t.Fatal(got, err)
	}
	if _, exists := got.Enabled["workshop-100"]; exists {
		t.Fatal("删除后洞穴残留配置")
	}
	newWorld := manager.forShard("NewWorld")
	got, err = newWorld.LoadOverrides()
	if err != nil || len(got.Order) != 0 {
		t.Fatal("新分片不应继承别的世界", got, err)
	}
}

func TestBrokenShardConfigurationFailsWithoutOverwriting(t *testing.T) {
	manager := NewManager(t.TempDir(), "", "", []string{"Master", "Caves"})
	path := filepath.Join(manager.clusterDir, "Caves", "modoverrides.lua")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{`return { ["workshop-100"] = {enabled=true`, `loadstring("bad")()`, `return {unknown=GetMods()}`} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.EnabledWorkshopIDs(); err == nil {
			t.Fatal("损坏配置不能静默当成空表")
		}
		if err := manager.forShard("Caves").SetEnabled("workshop-200", true); err == nil {
			t.Fatal("损坏配置不能被修改覆盖")
		}
		b, _ := os.ReadFile(path)
		if string(b) != content {
			t.Fatal("原文件被破坏")
		}
	}
}

func TestShardOverridesKeepQuotedLocalNames(t *testing.T) {
	for _, raw := range []string{`return {['local-name']={enabled=true}}`, `return {["don't-starve"]={enabled=true}}`} {
		o, err := ParseOverridesChecked(raw)
		if err != nil || len(o.Order) != 1 || !o.Enabled[o.Order[0]] {
			t.Fatal(o, err)
		}
	}
}
