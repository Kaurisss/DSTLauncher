package web

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dst-windows/internal/modmgr"
	"dst-windows/internal/room"
	"dst-windows/internal/saveimport"
)

func modShardFixture(t *testing.T) (*Server, *room.Room) {
	t.Helper()
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ shard, content string }{
		{"Master", `return {["workshop-100"]={enabled=true,configuration_options={amount=1}}}`},
		{"Caves", `return {["workshop-200"]={enabled=true,configuration_options={amount=2}},["workshop-100"]={enabled=false,configuration_options={amount=9}}}`},
	} {
		if err := s.modsFor(rm, tc.shard).SaveOverrides(modmgr.ParseOverrides(tc.content)); err != nil {
			t.Fatal(err)
		}
	}
	return s, rm
}

func TestModShardAPIIsolationAndDefaultMaster(t *testing.T) {
	s, rm := modShardFixture(t)
	h := s.Handler()
	_, body := doJSON(t, h, http.MethodGet, "/api/mods", nil, nil)
	items := body["data"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["id"] != "workshop-100" {
		t.Fatal(body)
	}
	path := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Master", "modoverrides.lua")
	before, _ := os.ReadFile(path)
	for _, endpoint := range []string{"/api/mods/toggle?shard=Caves&id=workshop-100&enabled=true", "/api/mods/config?shard=Caves&id=workshop-100&key=amount&value=5"} {
		_, body = doJSON(t, h, http.MethodGet, endpoint, nil, nil)
		if body["code"] != float64(200) {
			t.Fatal(body)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("编辑洞穴修改了地表")
	}
	// Recreate the service and room store to verify persisted state, not an in-memory draft.
	store, err := room.NewStore(s.rooms.Path())
	if err != nil {
		t.Fatal(err)
	}
	loaded := NewServer(Options{Mgr: s.mgr, Rooms: store, Steam: s.steam, RootDir: s.rootDir, ConfDir: s.confDir, WorkDir: s.workDir})
	_, body = doJSON(t, loaded.Handler(), http.MethodGet, "/api/mods?shard=Caves", nil, nil)
	for _, item := range body["data"].([]interface{}) {
		mod := item.(map[string]interface{})
		if mod["id"] == "workshop-100" && (mod["enabled"] != true || mod["config"].(map[string]interface{})["amount"] != float64(5)) {
			t.Fatal(mod)
		}
	}
	for _, invalid := range []string{"Other", "../Master", `Caves\..\Master`, "Master.", ".."} {
		for _, endpoint := range []string{"/api/mods", "/api/mods/toggle", "/api/mods/config", "/api/mods/updates"} {
			_, body = doJSON(t, h, http.MethodGet, endpoint+"?shard="+url.QueryEscape(invalid)+"&id=workshop-100&key=x&value=1", nil, nil)
			if body["code"] == float64(200) {
				t.Fatalf("非法分片被接受: %s %s", endpoint, invalid)
			}
		}
	}
	// A disabled world's configuration remains editable but is excluded from the download union.
	rm.Shards[1].Disabled = true
	if _, err := s.rooms.Update(rm); err != nil {
		t.Fatal(err)
	}
	_, body = doJSON(t, h, http.MethodGet, "/api/mods/toggle?shard=Caves&id=workshop-300&enabled=true", nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	ids, err := s.modsFor(rm).EnabledWorkshopIDs()
	if err != nil || !reflect.DeepEqual(ids, []string{"100"}) {
		t.Fatal(ids, err)
	}
}

func TestModDownloadUnionAndStartScope(t *testing.T) {
	s, rm := modShardFixture(t)
	// Provide an isolated installation so the generated shared setup file can be inspected.
	root := fakeSteamInstall(t, s)
	if _, err := s.steam.Configure(root, "64", "test"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.steam.GameDir(), 0755); err != nil {
		t.Fatal(err)
	}
	s.syncModsSetup()
	setup, err := os.ReadFile(filepath.Join(s.steam.GameDir(), "mods", "dedicated_server_mods_setup.lua"))
	if err != nil || !strings.Contains(string(setup), `ServerModSetup("100")`) || !strings.Contains(string(setup), `ServerModSetup("200")`) {
		t.Fatal(string(setup), err)
	}
	ids, err := s.MissingModIDs()
	if err != nil || !reflect.DeepEqual(ids, []string{"workshop-100", "workshop-200"}) {
		t.Fatal(ids, err)
	}
	// Install only the surface dependency; the cave-only missing mod must not block it.
	dir := filepath.Join(s.mgr.UgcDir(), "content", "322330", "100")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte(`name="surface"`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.checkShardMods(rm, "Master"); err != nil {
		t.Fatal(err)
	}
	if err := s.checkShardMods(rm, "Caves"); err == nil || !strings.Contains(err.Error(), "workshop-200") {
		t.Fatal(err)
	}
	if err := s.checkShardMods(rm, ""); err == nil {
		t.Fatal("整房间启动必须检查洞穴依赖")
	}
	path := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Caves", "modoverrides.lua")
	if err := os.WriteFile(path, []byte(`return { broken`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.checkShardMods(rm, "Master"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MissingModIDs(); err == nil {
		t.Fatal("洞穴配置错误必须报告")
	}
	s.syncModsSetup()
	after, _ := os.ReadFile(filepath.Join(s.steam.GameDir(), "mods", "dedicated_server_mods_setup.lua"))
	if string(after) != string(setup) {
		t.Fatal("读取失败不得发布不完整下载清单")
	}
}

func TestDeleteModPreservesIndependentAndDisabledShardSettings(t *testing.T) {
	s, rm := modShardFixture(t)
	rm.Shards[1].Disabled = true
	if _, err := s.rooms.Update(rm); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, fmt.Sprintf("/api/mods/delete?room=%d&id=workshop-100", rm.ID), nil, nil)
	if body["code"] != float64(200) {
		t.Fatal(body)
	}
	o, err := s.modsFor(rm, "Caves").LoadOverrides()
	if err != nil || len(o.Order) != 1 || !o.Enabled["workshop-200"] || o.Config["workshop-200"]["amount"] != float64(2) {
		t.Fatal(o, err)
	}
}

func TestImportSummaryCountsOnlyImportedShardMods(t *testing.T) {
	rm := room.Defaults()
	p := &saveimport.Parsed{HasMods: true, Shards: []saveimport.ShardSave{
		{Name: "Master", Mods: modmgr.ParseOverrides(`return {["workshop-100"]={enabled=true}}`)},
		{Name: "Caves", Mods: modmgr.ParseOverrides(`return {["workshop-100"]={enabled=true},["workshop-200"]={enabled=true}}`)},
		{Name: "Other", Mods: modmgr.ParseOverrides(`return {["workshop-300"]={enabled=true}}`)},
	}}
	if sum := buildImportSummary(p, rm, nil, true, nil); sum.ModsTotal != 2 {
		t.Fatal(sum)
	}
	rm.Shards[1].Disabled = true
	if sum := buildImportSummary(p, rm, nil, true, nil); sum.ModsTotal != 1 {
		t.Fatal(sum)
	}
}

type modUpdatesTransport func(*http.Request) (*http.Response, error)

func (fn modUpdatesTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestAutomaticUpdateUsesAllEnabledShardsAndManualCheckUsesSelected(t *testing.T) {
	s, rm := modShardFixture(t)
	for shard, id := range map[string]string{"Master": "1000", "Caves": "2000"} {
		o := modmgr.ParseOverrides(fmt.Sprintf(`return {["workshop-%s"]={enabled=true}}`, id))
		if err := s.modsFor(rm, shard).SaveOverrides(o); err != nil {
			t.Fatal(err)
		}
	}
	acf := `"AppWorkshop" { "WorkshopItemsInstalled" { "1000" { "timeupdated" "10" } "2000" { "timeupdated" "10" } } }`
	if err := os.WriteFile(filepath.Join(s.mgr.UgcDir(), "appworkshop_322330.acf"), []byte(acf), 0644); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	var queried []string
	http.DefaultTransport = modUpdatesTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.steampowered.com" {
			return nil, fmt.Errorf("不允许真实网络请求: %s", r.URL)
		}
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		queried = nil
		for i := 0; i < 2; i++ {
			if id := r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i)); id != "" {
				queried = append(queried, id)
			}
		}
		body := `{"response":{"publishedfiledetails":[{"publishedfileid":"1000","result":1,"time_updated":10},{"publishedfileid":"2000","result":1,"time_updated":20}]}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	ids, err := s.pendingMods([]*room.Room{rm})
	if err != nil || !reflect.DeepEqual(ids, []string{"2000"}) || !reflect.DeepEqual(queried, []string{"1000", "2000"}) {
		t.Fatal(ids, queried, err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/mods/updates?shard=Master", nil, nil)
	if body["code"] != float64(200) || !reflect.DeepEqual(queried, []string{"1000"}) {
		t.Fatal(body, queried)
	}
	rm.Shards[1].Disabled = true
	ids, err = s.pendingMods([]*room.Room{rm})
	if err != nil || len(ids) != 0 || !reflect.DeepEqual(queried, []string{"1000"}) {
		t.Fatal(ids, queried, err)
	}
}
