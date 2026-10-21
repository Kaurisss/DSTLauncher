package worldsettings

import (
	"regexp"
	"strings"
)

var (
	legacyEnabledRe = regexp.MustCompile(`\boverride_enabled\s*=\s*(true|false)`)
	legacyFieldRe   = regexp.MustCompile(`\b(worldgen_preset|settings_preset|preset)\s*=\s*"([^"]*)"`)
	legacyBlockRe   = regexp.MustCompile(`\boverrides\s*=\s*\{`)
	legacyItemRe    = regexp.MustCompile(`^(?:\["([^"]+)"\]|([A-Za-z_]\w*))\s*=\s*"([^"]*)"`)
)

// 仅保留历史导入器“跳过垃圾行”的兼容行为；面板读写绝不使用此回退。
func parseLegacyOverride(content string) *Override {
	o := &Override{Overrides: map[string]string{}}
	if m := legacyEnabledRe.FindStringSubmatch(content); m != nil {
		o.Enabled = m[1] == "true"
	}
	preset := ""
	for _, m := range legacyFieldRe.FindAllStringSubmatch(content, -1) {
		switch m[1] {
		case "worldgen_preset":
			o.WorldGenPreset = m[2]
		case "settings_preset":
			o.SettingsPreset = m[2]
		case "preset":
			preset = m[2]
		}
	}
	if o.WorldGenPreset == "" {
		o.WorldGenPreset = preset
	}
	if o.SettingsPreset == "" {
		o.SettingsPreset = preset
	}
	if loc := legacyBlockRe.FindStringIndex(content); loc != nil {
		if body, ok := extractBraces(content, loc[1]-1); ok {
			for _, line := range strings.Split(body, "\n") {
				if m := legacyItemRe.FindStringSubmatch(strings.TrimSpace(stripLuaComment(line))); m != nil {
					name := m[1]
					if name == "" {
						name = m[2]
					}
					o.Overrides[name] = m[3]
				}
			}
		}
	}
	return o
}
