package roomprofile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dst-windows/internal/room"
)

func TestCapturePortablePerShardConfiguration(t *testing.T) {
	dir := t.TempDir()
	rm := room.Defaults()
	rm.Token, rm.Password, rm.ClusterKey = "SECRET-TOKEN", "SECRET-PASSWORD", "SECRET-CLUSTER"
	rm.Normalize()
	rm.Shards[1].Disabled = true
	for _, shard := range rm.Shards {
		if err := os.MkdirAll(filepath.Join(dir, shard.Name, "save"), 0755); err != nil {
			t.Fatal(err)
		}
		content := `return {["workshop-123456"]={enabled=true,configuration_options={value="` + shard.Name + `"}}}`
		if err := os.WriteFile(filepath.Join(dir, shard.Name, "modoverrides.lua"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, shard.Name, "save", "world"), []byte("SECRET-SAVE"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Capture(dir, rm, "四世界配置")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "SECRET-") {
		t.Fatalf("方案包含凭据或存档: %s", b)
	}
	if !p.Room.Shards[1].Disabled || !strings.Contains(p.Files["Caves"]["modoverrides.lua"], "Caves") {
		t.Fatal("丢失停用分片的独立配置")
	}
	if rm.Token != "SECRET-TOKEN" || rm.Shards[1].Disabled != true {
		t.Fatal("修改了源房间")
	}
	store := New(t.TempDir())
	saved, err := store.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(saved.ID)
	if err != nil || loaded.Files["Master"]["modoverrides.lua"] != p.Files["Master"]["modoverrides.lua"] {
		t.Fatal(loaded, err)
	}
	if _, err := store.Load("../outside"); err == nil {
		t.Fatal("接受了越界 ID")
	}
}

func TestRejectUnsafeOrExecutableProfileFiles(t *testing.T) {
	for _, tc := range []struct{ shard, file, body string }{
		{"../outside", "modoverrides.lua", "return {}"},
		{"Master", "../cluster_token.txt", "token"},
		{"Master", "save/world", "world"},
		{"Master", "modoverrides.lua", `os.execute("touch bad"); return {}`},
		{"Master", "worldgenoverride.lua", `os.execute("touch bad"); return {override_enabled=true,overrides={}}`},
		{"Master", "leveldataoverride.lua", `return {location="forest",overrides={}}; os.execute("bad")`},
	} {
		t.Run(tc.shard+tc.file, func(t *testing.T) {
			p := &Profile{Version: 1, Name: "方案", Room: room.Defaults(), Files: map[string]map[string]string{tc.shard: {tc.file: tc.body}}}
			if err := p.Validate(); err == nil {
				t.Fatal("接受了不安全的方案")
			}
		})
	}
}
