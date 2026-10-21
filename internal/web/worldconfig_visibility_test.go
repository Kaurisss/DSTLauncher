package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorldConfigUsesShardMasterFlag(t *testing.T) {
	for _, location := range []string{"shipwrecked", "volcanoworld"} {
		t.Run(location, func(t *testing.T) {
			lua := fmt.Sprintf("return {\n  location=\"%s\",\n  overrides={\n  },\n}\n", location)
			api, rm := newWorldConfigServer(t, []string{"First", "Second"}, map[string]string{"First": lua, "Second": lua})
			for _, shard := range []string{"First", "Second"} {
				view := getWorldConfig(t, api, itoa(rm.ID), shard)
				got := map[string]bool{}
				for _, g := range view.Groups {
					for _, o := range g.Options {
						got[o.Name] = true
					}
				}
				for _, name := range []string{"autumn", "specialevent", "spawnprotection", "basicresource_regrowth", "mild", "poison", "shipwrecked_season_start"} {
					if got[name] != (shard == "First") {
						t.Errorf("%s/%s 的主世界专属项 %s 显隐错误", location, shard, name)
					}
				}
				if !got["task_set"] || !got["dragoonegg"] {
					t.Error("不应隐藏世界自己的配置项")
				}
			}
		})
	}
}

// API 提供完整候选项和依赖名单，前端才能不丢配置地即时切换显隐。
func TestWorldConfigIncludesIslandVisibilityMetadata(t *testing.T) {
	api, rm := newWorldConfigServer(t, []string{"Master"}, map[string]string{"Master": `return {
  location="shipwrecked",
  overrides={
    volcanoisland="none",
    coffeebush="often",
  },
}
`})
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/worldconfig?room="+itoa(rm.ID)+"&shard=Master", nil))
	var body struct {
		Code int `json:"code"`
		Data struct {
			WorldConfigView
			VolcanoIslandOnly []string `json:"volcanoIslandOnly"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 {
		t.Fatalf("接口失败: %s", rec.Body.String())
	}
	dependent := map[string]bool{}
	for _, name := range body.Data.VolcanoIslandOnly {
		dependent[name] = true
	}
	if !dependent["coffeebush"] || !dependent["daywalker"] {
		t.Fatal("缺少火山岛显隐依赖名单")
	}
	found := false
	for _, g := range body.Data.Groups {
		for _, o := range g.Options {
			if o.Name == "coffeebush" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("火山岛关闭时也应提供候选项定义供前端切换")
	}
	if body.Data.Config.Overrides["coffeebush"] != "often" {
		t.Error("隐藏候选项的原配置丢失")
	}
}
