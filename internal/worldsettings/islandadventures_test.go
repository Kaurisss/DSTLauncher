package worldsettings

import "testing"

// 火山的项必须与游戏内一致：它是副世界，带 master_controlled 的
// 季节/活动/冒险家游戏根本不列。
func TestVolcanoHidesMasterOnlyGroups(t *testing.T) {
	groups := IAGroups(LocationVolcano, false)
	if len(groups) == 0 {
		t.Fatal("火山没有任何分组")
	}
	got := map[string]bool{}
	for _, g := range groups {
		for _, o := range g.Options {
			got[o.Name] = true
		}
	}
	// 这两个是 master_controlled（main world only）：火山页不该出现
	for _, name := range []string{"autumn", "specialevent", "spawnprotection", "basicresource_regrowth"} {
		if got[name] {
			t.Errorf("火山不该有主世界专属项 %s", name)
		}
	}
	// 截图里有的项必须都在
	for _, name := range []string{
		"dst_boats", "ia_boats", "dragoonegg", "lightning", "weather",
		"regrowth", "rock_charcoal_regrowth", "volcano_shrub_regrowth",
		"magmarock_regrowth", "rock_obsidian_regrowth",
		"bunnymen_setting", "fishermerm_setting", "pigs_setting",
		"primeape_setting", "bees_setting", "wildbores_setting", "moles_setting",
		"dragoon_setting", "snake_setting", "spiders_setting", "spider_warriors", "merms",
		"daywalker", "liefs", "fruitfly", "spiderqueen",
	} {
		if !got[name] {
			t.Errorf("火山缺少 %s", name)
		}
	}
}

// 世界生成里 prefabswaps_start 的 world 只有 forest/cave，模组没把它给火山。
func TestVolcanoWorldgenMatchesGame(t *testing.T) {
	want := []string{
		"task_set", "start_location",
		"coffeebush", "rock_charcoal", "volcano_shrub", "magma_rocks",
		"elephantcactus", "rock_obsidian",
		"dragoon",
	}
	got := map[string]bool{}
	for _, g := range IAGroups(LocationVolcano, false) {
		if g.Category != CategoryWorldGen {
			continue
		}
		for _, o := range g.Options {
			got[o.Name] = true
		}
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("火山世界生成缺少 %s", name)
		}
	}
	if got["prefabswaps_start"] {
		t.Error("火山世界生成不该有 prefabswaps_start（它的 world 只有 forest/cave）")
	}
	if len(got) != len(want) {
		t.Errorf("火山世界生成项数 %d，应为 %d：%v", len(got), len(want), got)
	}
}

// 组内顺序必须按截图：世界规则与世界生成都有 misc/resources/monsters 同名组，
// 两张顺序表不能互相覆盖。
func TestVolcanoOptionOrder(t *testing.T) {
	want := map[string][]string{
		"settings/misc": {
			"dst_boats", "ia_boats", "dragoonegg", "lightning", "weather",
		},
		"settings/resources": {
			"regrowth", "rock_charcoal_regrowth", "volcano_shrub_regrowth",
			"magmarock_regrowth", "rock_obsidian_regrowth",
		},
		"settings/animals": {
			"bunnymen_setting", "fishermerm_setting", "pigs_setting",
			"primeape_setting", "bees_setting", "wildbores_setting", "moles_setting",
		},
		"settings/monsters": {
			"dragoon_setting", "snake_setting", "spiders_setting",
			"spider_warriors", "merms",
		},
		"settings/giants": {"fruitfly", "liefs", "daywalker", "spiderqueen"},
		"worldgen/misc":   {"task_set", "start_location"},
		"worldgen/resources": {
			"coffeebush", "rock_charcoal", "volcano_shrub",
			"magma_rocks", "elephantcactus", "rock_obsidian",
		},
		"worldgen/monsters": {"dragoon"},
	}
	for _, g := range IAGroups(LocationVolcano, false) {
		key := g.Category + "/" + g.ID
		exp, ok := want[key]
		if !ok {
			continue
		}
		got := make([]string, len(g.Options))
		for i, o := range g.Options {
			got[i] = o.Name
		}
		if len(got) != len(exp) {
			t.Errorf("%s 项数 %d，应为 %d", key, len(got), len(exp))
			continue
		}
		for i := range exp {
			if got[i] != exp[i] {
				t.Errorf("%s 第 %d 项是 %s，应为 %s（全部：%v）", key, i+1, got[i], exp[i], got)
			}
		}
	}
}

// 森林的两项海洋资源取值表是游戏用循环派生的，原版提取器要能补上。
func TestOceanDerivedValues(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ocean_seastack", "ocean_waterplant"} {
		opt, ok := Lookup(name)
		if !ok {
			t.Fatalf("清单里没有 %s", name)
		}
		if len(opt.Values) == 0 {
			t.Errorf("%s 没有取值，会被当成自由输入", name)
		}
		if opt.Values[0].Data[:6] != "ocean_" {
			t.Errorf("%s 的取值应带 ocean_ 前缀，实际 %q", name, opt.Values[0].Data)
		}
	}
}
