package main

import "testing"

// 预设只从 AddSettingsPreset / AddWorldGenLevel 两处取：游戏自己的
// GetSettingsList / GetWorldGenList 也只读这两张表。
// hideinfrontend = true 的项不能进下拉框：MOD_MISSING 是模组丢失时的兜底占位，
// 玩家选中它毫无意义（游戏自己也不列）。deepcopy 派生项要继承基底的隐藏状态。
func TestParsePresetsSkipsHidden(t *testing.T) {
	const src = `
local survival_together = {
	id = "SURVIVAL_TOGETHER",
	location = "forest",
}
AddLevel(LEVELTYPE.SURVIVAL, survival_together)

local settings_survival_together = {}
settings_survival_together.id = "SURVIVAL_TOGETHER"
settings_survival_together.location = "forest"
AddSettingsPreset(LEVELTYPE.SURVIVAL, settings_survival_together)

local settings_mod_missing = deepcopy(settings_survival_together)
settings_mod_missing.id = "MOD_MISSING"
settings_mod_missing.hideinfrontend = true
AddSettingsPreset(LEVELTYPE.SURVIVAL, settings_mod_missing)

AddSettingsPreset(LEVELTYPE.SURVIVAL, {
	id = "SURVIVAL_TOGETHER_CLASSIC",
	hideinfrontend = true,
	location = "forest",
})

local lightsout = deepcopy(survival_together)
lightsout.id = "LIGHTS_OUT"
AddWorldGenLevel(LEVELTYPE.SURVIVAL, lightsout)
`
	got := parsePresets(src, map[string]string{})

	settings := []string{}
	for _, p := range got["forest_settings"] {
		settings = append(settings, p.Data)
	}
	if len(settings) != 1 || settings[0] != "SURVIVAL_TOGETHER" {
		t.Errorf("settings 预设错误（隐藏项未过滤或 AddLevel 混入）: %v", settings)
	}
	if list := got["forest_worldgen"]; len(list) != 1 || list[0].Data != "LIGHTS_OUT" {
		t.Errorf("worldgen 预设错误: %v", list)
	}
}
