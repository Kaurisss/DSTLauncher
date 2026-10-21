package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dst-windows/internal/room"
	"dst-windows/internal/server"
	"dst-windows/internal/worldsettings"
)

// newWorldConfigServer 造一个只有一个房间的面板，分片数据落在临时目录。
func newWorldConfigServer(t *testing.T, shards []string, files map[string]string) (*Server, *room.Room) {
	t.Helper()
	root := t.TempDir()
	work := t.TempDir()
	store, err := room.NewStore(room.DefaultStorePath(root))
	if err != nil {
		t.Fatalf("创建房间库失败: %v", err)
	}
	rm, err := store.Create(room.Defaults())
	if err != nil {
		t.Fatalf("创建房间失败: %v", err)
	}
	// 分片名按用例给定，世界类型故意都写成 forest：真实存档里也是这么错的，
	// 用来验证判定依据是磁盘上的 location 而不是 rooms.json。
	rm.Shards = nil
	for i, name := range shards {
		rm.Shards = append(rm.Shards, room.Shard{
			Name: name, WorldType: room.WorldTypeForest, GameID: i + 1, IsMaster: i == 0,
			ServerPort: 11000 + i, MasterServerPort: 31000 + i, AuthenticationPort: 41000 + i,
		})
	}
	if _, err := store.Update(rm); err != nil {
		t.Fatalf("保存房间失败: %v", err)
	}
	for shard, content := range files {
		dir := filepath.Join(root, "DST", rm.Key(), shard)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, worldsettings.LevelDataFileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mgr := server.NewManager(root, t.TempDir(), t.TempDir())
	for _, r := range store.List() {
		mgr.Register(server.NewCluster(r))
	}
	api := NewServer(Options{Mgr: mgr, Rooms: store, RootDir: root, ConfDir: "DST", WorkDir: work})
	return api, rm
}

func getWorldConfig(t *testing.T, api *Server, roomID string, shard string) WorldConfigView {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/worldconfig?room="+roomID+"&shard="+shard, nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	var body struct {
		Code int             `json:"code"`
		Msg  string          `json:"message"`
		Data WorldConfigView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v (%s)", err, rec.Body.String())
	}
	if body.Code != 200 {
		t.Fatalf("接口返回失败: %s", body.Msg)
	}
	return body.Data
}

// 海难分片必须按存档里的 location 识别，并带上模组选项。
func TestWorldConfigShipwrecked(t *testing.T) {
	const lua = `return {
  id="SHIPWRECKED_ENDLESS",
  location="shipwrecked",
  overrides={
    bamboo="default",
    basicresource_regrowth="always",
    has_ocean=true,
  },
}
`
	api, rm := newWorldConfigServer(t, []string{"Master"},
		map[string]string{"Master": lua})
	view := getWorldConfig(t, api, itoa(rm.ID), "Master")

	if view.Location != worldsettings.LocationShipwrecked {
		t.Fatalf("世界类型应为海难，实际 %q", view.Location)
	}
	if !view.LevelDataSet || view.Mod != "island-adventures" {
		t.Errorf("应识别为 leveldataoverride + 岛屿冒险，实际 %+v", view)
	}
	names := map[string]bool{}
	for _, g := range view.Groups {
		for _, o := range g.Options {
			names[o.Name] = true
		}
	}
	for _, want := range []string{"bamboo", "grass", "world_size"} {
		if !names[want] {
			t.Errorf("缺少选项 %s", want)
		}
	}
	if len(view.Presets[worldsettings.CategoryWorldGen]) == 0 {
		t.Error("缺少世界生成预设")
	}
	if view.Config.Overrides["bamboo"] != "default" {
		t.Errorf("应读出存档里的当前值，实际 %v", view.Config.Overrides)
	}
}

// 火山分片同样走模组清单，并且不能混进只在海难可用的项。
func TestWorldConfigVolcano(t *testing.T) {
	const lua = `return {
  id="SURVIVAL_VOLCANO_CLASSIC",
  location="volcanoworld",
  overrides={
    dragoonegg="default",
  },
}
`
	api, rm := newWorldConfigServer(t, []string{"Caves"},
		map[string]string{"Caves": lua})
	view := getWorldConfig(t, api, itoa(rm.ID), "Caves")

	if view.Location != worldsettings.LocationVolcano {
		t.Fatalf("世界类型应为火山，实际 %q", view.Location)
	}
	names := map[string]bool{}
	for _, g := range view.Groups {
		for _, o := range g.Options {
			names[o.Name] = true
		}
	}
	if !names["dragoonegg"] {
		t.Error("火山应包含 dragoonegg")
	}
	if names["bamboo"] {
		t.Error("火山不应包含只在海难可用的 bamboo")
	}
}

// 没有 leveldataoverride.lua 的分片仍然走 worldgenoverride 流程。
func TestWorldConfigFallsBackToWorldgenOverride(t *testing.T) {
	api, rm := newWorldConfigServer(t, []string{"Master"}, nil)
	view := getWorldConfig(t, api, itoa(rm.ID), "Master")
	if view.LevelDataSet {
		t.Error("没有 leveldataoverride.lua 时不应标记为已使用")
	}
	if view.Location != worldsettings.LocationForest {
		t.Errorf("应回落到房间配置的 forest，实际 %q", view.Location)
	}
	if len(view.Groups) == 0 {
		t.Error("原版选项不应为空")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
