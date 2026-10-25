package saveimport

import (
	"os"
	"path/filepath"
	"testing"

	"dst-windows/internal/config"
	"dst-windows/internal/modmgr"
)

func TestImportKeepsIndependentModsAndMissingVersusEmpty(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "cluster.ini"), "[NETWORK]\ncluster_name=import\n")
	for _, name := range []string{"Master", "Caves", "Empty", "Missing"} {
		writeFile(t, filepath.Join(source, name, "server.ini"), "[SHARD]\nname="+name+"\n")
	}
	writeFile(t, filepath.Join(source, "Master", "modoverrides.lua"), `return {["workshop-100"]={enabled=true,configuration_options={x=1}}}`)
	writeFile(t, filepath.Join(source, "Caves", "modoverrides.lua"), `return {["workshop-200"]={enabled=true,configuration_options={x=2}}}`)
	writeFile(t, filepath.Join(source, "Empty", "modoverrides.lua"), "")
	p, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rm := p.Room
	rm.ID = 1
	cluster := config.ClusterDir(root, "DST", 1)
	for _, sh := range rm.Shards {
		writeFile(t, filepath.Join(cluster, sh.Name, "modoverrides.lua"), `return {["workshop-old"]={enabled=true}}`)
	}
	missingPath := filepath.Join(cluster, "Missing", "modoverrides.lua")
	before, _ := os.ReadFile(missingPath)
	if _, err := p.WriteInto(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(missingPath)
	if string(before) != string(after) {
		t.Fatal("来源未带配置应保留目标原文件")
	}
	for _, tc := range []struct {
		shard, id string
		x         float64
	}{{"Master", "workshop-100", 1}, {"Caves", "workshop-200", 2}} {
		got, err := modmgr.NewManager(cluster, "", "", []string{tc.shard}).LoadOverrides()
		if err != nil || len(got.Order) != 1 || !got.Enabled[tc.id] || got.Config[tc.id]["x"] != tc.x {
			t.Fatal(tc.shard, got, err)
		}
	}
	empty, err := modmgr.NewManager(cluster, "", "", []string{"Empty"}).LoadOverrides()
	if err != nil || len(empty.Order) != 0 {
		t.Fatal("明确的空配置只清空对应分片", empty, err)
	}
	// The model also preserves disabled worlds' existing files.
	for i := range rm.Shards {
		if rm.Shards[i].Name == "Caves" {
			rm.Shards[i].Disabled = true
		}
	}
	cavesPath := filepath.Join(cluster, "Caves", "modoverrides.lua")
	before, _ = os.ReadFile(cavesPath)
	if _, err := p.WriteInto(root, "DST", rm); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(cavesPath)
	if string(before) != string(after) {
		t.Fatal("未导入分片的配置被修改")
	}
}
