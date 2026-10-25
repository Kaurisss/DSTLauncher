package web

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"dst-windows/internal/modmgr"
	"dst-windows/internal/room"
)

func TestModListHidesClientOnlyWithoutChangingFilesOrOverrides(t *testing.T) {
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for id, info := range map[string]string{
		"101":        `name="client" client_only_mod=true api_version=10`,
		"2555936751": `local strings={name="种植计算器"} name=strings.name all_clients_require_mod=false client_only_mod=true local keys={"K"} for i=1,12 do keys[#keys+1]="F"..i end configuration_options={}`,
		"102":        `name="server" server_only_mod=true client_only_mod=false`,
		"103":        `name="shared" all_clients_require_mod=true`,
		"104":        `name="legacy"`,
	} {
		dir := filepath.Join(s.mgr.UgcDir(), "content", "322330", id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte(info), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mods := s.modsFor(rm)
	o := modmgr.NewOverrides()
	o.Order = []string{"workshop-101", "workshop-105"}
	o.Enabled["workshop-101"], o.Enabled["workshop-105"] = true, true
	if err := mods.SaveOverrides(o); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/mods", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	items := body["data"].([]interface{})
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.(map[string]interface{})["id"].(string)] = true
	}
	if len(seen) != 4 || seen["workshop-101"] || seen["workshop-2555936751"] {
		t.Fatalf("客户端模组未过滤: %v", seen)
	}
	for _, id := range []string{"workshop-102", "workshop-103", "workshop-104", "workshop-105"} {
		if !seen[id] {
			t.Errorf("误隐藏服务端/未知/待下载模组: %s", id)
		}
	}
	stored, err := mods.LoadOverrides()
	if err != nil || !stored.Enabled["workshop-101"] {
		t.Fatal("列表过滤不应改写现有配置", err)
	}
	installed, err := mods.InstalledMods()
	if err != nil || !installed["workshop-101"] {
		t.Fatal("客户端模组文件必须保留", err)
	}
}
