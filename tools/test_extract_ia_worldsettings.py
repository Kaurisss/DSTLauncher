"""岛屿冒险配置提取的自检；不依赖游戏或模组文件，用手写样本跑解析规则。"""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import zipfile

import extract_ia_worldsettings as E


class StripCommentsTest(unittest.TestCase):
    def test_ignores_commented_out_items(self):
        src = """local ia_settings_customize_table = {
    misc = {
        -- dragoonegg = { value = "default", image = "x.tex" },
        floods = { value = "default", image = "floods.tex" },
    },
}
"""
        items = E.parse_item_tables(E.strip_comments(src), {"STRINGS.UI.CUSTOMIZATIONSCREEN.FLOODS": "洪水"})
        self.assertEqual(list(items), ["floods"])
        self.assertEqual(items["floods"]["category"], "settings")

    def test_block_comment_removed(self):
        src = """--[[
shipwrecked_season_start = { value = "default" },
]]
local ia_worldgen_customize_table = {
    global = {
        shipwreck = { value = "default", image = "wrecks.tex", world = shipwrecked },
    },
}
"""
        items = E.parse_item_tables(E.strip_comments(src), {"STRINGS.UI.CUSTOMIZATIONSCREEN.SHIPWRECK": "沉船"})
        self.assertEqual(list(items), ["shipwreck"])
        self.assertEqual(items["shipwreck"]["category"], "worldgen")
        self.assertEqual(items["shipwreck"]["worlds"], ["shipwrecked"])


class ParseItemTablesTest(unittest.TestCase):
    labels = {
        "STRINGS.UI.CUSTOMIZATIONSCREEN.DST_BOATS": "DST船",
        "STRINGS.UI.CUSTOMIZATIONSCREEN.DRAGOONEGG": "火山爆发",
    }

    def test_empty_worldgen_table_does_not_swallow_following_settings(self):
        # ia-core 的真实形态：世界生成表是空的，紧随其后是设置表。
        # 若正则把两者当成一个块，整批设置会被错标成 worldgen。
        src = """local ia_worldgen_customize_table = {}

local ia_settings_customize_table = {
    misc = {
        dst_boats = { value = "always", image = "cookieboats.tex", desc = enableddisabled_descriptions, order = 4, world = allworlds},
    },
}
"""
        items = E.parse_item_tables(src, self.labels)
        self.assertEqual(items["dst_boats"]["category"], "settings")
        self.assertEqual(items["dst_boats"]["group"], "misc")
        self.assertEqual(items["dst_boats"]["order"], 4)
        self.assertEqual(items["dst_boats"]["desc"], "enableddisabled_descriptions")
        self.assertEqual(items["dst_boats"]["worlds"], E.WORLD_ALIAS["allworlds"])

    def test_later_file_keeps_first_definition(self):
        core = """local ia_settings_customize_table = {
    misc = {
        dst_boats = { value = "always", world = allworlds},
    },
}
"""
        sw = """local ia_worldgen_customize_table = {
    misc = {
        dst_boats = { value = "none", world = shipwrecked},
    },
}
"""
        items = E.parse_item_tables(core, self.labels)
        E.parse_item_tables(sw, self.labels, into=items)
        self.assertEqual(items["dst_boats"]["category"], "settings")
        self.assertEqual(items["dst_boats"]["default"], "always")

    def test_missing_label_is_an_error(self):
        src = """local ia_settings_customize_table = {
    misc = {
        unknown_item = { value = "default" },
    },
}
"""
        with self.assertRaises(SystemExit):
            E.parse_item_tables(src, self.labels)


class LocationDefaultsTest(unittest.TestCase):
    def test_reads_string_and_non_string_values(self):
        src = """AddLocation({
    location = "volcanoworld",
    version = 2,
    overrides={
        task_set = "volcano",
        has_ocean = false,
        world_size = "small",
    },
    required_prefabs = {
        "multiplayer_portal",
    },
})
"""
        got = E.location_defaults(src)
        self.assertEqual(got["volcanoworld"], {
            "task_set": "volcano", "has_ocean": "false", "world_size": "small",
        })


class PresetsTest(unittest.TestCase):
    def test_settings_and_worldgen_are_separated(self):
        src = """AddSettingsPreset(LEVELTYPE.SURVIVAL, {id = "SURVIVAL_VOLCANO_CLASSIC",
    name = STRINGS.UI.CUSTOMIZATIONSCREEN.PRESETLEVELS.SURVIVAL_VOLCANO_CLASSIC,
    location = "volcanoworld",
})
AddWorldGenLevel(LEVELTYPE.SURVIVAL, {id = "SURVIVAL_VOLCANO_CLASSIC",
    name = STRINGS.UI.CUSTOMIZATIONSCREEN.PRESETLEVELS.SURVIVAL_VOLCANO_CLASSIC,
    location = "volcanoworld",
})
"""
        get = E.presets(src, {"STRINGS.UI.CUSTOMIZATIONSCREEN.PRESETLEVELS.SURVIVAL_VOLCANO_CLASSIC": "火山"})
        self.assertEqual(get["volcanoworld"]["settings"], [{"data": "SURVIVAL_VOLCANO_CLASSIC", "label": "火山"}])
        self.assertEqual(get["volcanoworld"]["worldgen"], [{"data": "SURVIVAL_VOLCANO_CLASSIC", "label": "火山"}])


class DynamicOptionValuesTest(unittest.TestCase):
    def test_uses_explicit_non_sibling_core_for_labels(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            ia = root / "mods/islands"
            core = root / "elsewhere/ia-core"
            (ia / "scripts/map/tasksets").mkdir(parents=True)
            (core / "languages").mkdir(parents=True)
            (core / "languages/ia_sc.po").write_text(
                'msgctxt "STRINGS.IA.TASK"\nmsgid "Island"\nmsgstr "岛屿任务"\n', encoding="utf-8")
            (ia / "scripts/map/tasksets/islands.lua").write_text(
                'AddTaskSet("islands", {location="shipwrecked", name=STRINGS.IA.TASK})', encoding="utf-8")
            game_zip = root / "scripts.zip"
            with zipfile.ZipFile(game_zip, "w") as z:
                z.writestr("scripts/languages/chinese_s.po", "")
                z.writestr("scripts/map/startlocations/default.lua",
                           'AddStartLocation("default", {location="forest"})')
            self.assertEqual(E.dynamic_option_values(game_zip, ia, core), {
                "task_set": [{"data": "islands", "label": "岛屿任务", "location": "shipwrecked"}],
                "start_location": [{"data": "default", "label": "default", "location": "forest"}],
            })


class ExtractionContractTest(unittest.TestCase):
    def test_cli_retains_all_locations_and_merges_master_names(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            game, ia, core = root / "game", root / "mods/ia", root / "separate/core"
            (game / "data/databundles").mkdir(parents=True)
            (ia / "scripts/map/levels").mkdir(parents=True)
            (core / "languages").mkdir(parents=True)
            (game / "version.txt").write_text("test", encoding="utf-8")
            customize = '''local WORLDSETTINGS_GROUP = {
\t["global"] = {
\t\torder = 1,
\t\ttext = STRINGS.UI.SANDBOXMENU.GLOBAL,
\t\tdesc = yesno_descriptions,
\t\titems = {
\t\t\t["day"] = {value = "default", master_controlled = true},
\t\t\t["weather"] = {value = "default"},
\t\t\t["prefabswaps_start"] = {options_remap = {img="blank.tex"}, world=unknown_worlds},
\t\t},
\t},
}
'''
            for table in sorted(set(E.DESC_TABLE.values()) - set(E.DERIVED_TABLES)):
                customize += table + ' = {{text=STRINGS.YES, data="default"}}\n'
            customize += '''ocean_worldgen_frequency_descriptions = {}
for i, d in ipairs(worldgen_frequency_descriptions) do
    t[i] = {text = d.text, data = "ocean_"..d.data}
end
'''
            with zipfile.ZipFile(game / "data/databundles/scripts.zip", "w") as z:
                z.writestr("scripts/map/customize.lua", customize)
                z.writestr("scripts/languages/chinese_s.po", 'msgctxt "STRINGS.YES"\nmsgid "Yes"\nmsgstr "是"\n')
            labels = {"POISON": "中毒", "MILD": "温和季节", "SHIPWRECKED_SEASON_START": "开局季节"}
            po = "".join(f'msgctxt "STRINGS.UI.CUSTOMIZATIONSCREEN.{key}"\nmsgid "{key}"\nmsgstr "{label}"\n'
                         for key, label in labels.items())
            po += 'msgctxt "STRINGS.UI.CUSTOMIZATIONSCREEN.PRESETLEVELS.TEST"\nmsgid "Test"\nmsgstr "测试"\n'
            (core / "languages/ia_sc.po").write_text(po, encoding="utf-8")
            (core / "modservercreationmain.lua").write_text('''local ia_settings_customize_table = {
    global = {
        poison = {value = "always", master_controlled = true},
    },
}
''', encoding="utf-8")
            (ia / "modservercreationmain.lua").write_text('''local sw_settings_customize_table = {
    global = {
        mild = {value = "default", master_controlled = true},
    },
}
local sw_worldgen_customize_table = {
    global = {
        shipwrecked_season_start = {value = "default", master_controlled = true},
    },
}
local season_start = {{ text = STRINGS.UI.CUSTOMIZATIONSCREEN.MILD, data = "default" }}
local vanilla_options_in_islands = {
    "day",
}
local vanilla_options_in_volcano = {
    "day",
}
''', encoding="utf-8")
            (ia / "scripts/map/sw_locations.lua").write_text("\n".join(
                f'AddLocation({{location = "{loc}", overrides={{\n        day = "default",\n    }},\n}})'
                for loc in ("shipwrecked", "volcanoworld")), encoding="utf-8")
            for filename, loc in (("shipwrecked", "shipwrecked"), ("volcano", "volcanoworld")):
                (ia / f"scripts/map/levels/{filename}.lua").write_text("\n".join(
                    f'Add{kind}(LEVELTYPE.SURVIVAL, {{id = "TEST",\n'
                    f'    name = STRINGS.UI.CUSTOMIZATIONSCREEN.PRESETLEVELS.TEST,\n'
                    f'    location = "{loc}",\n}})'
                    for kind in ("SettingsPreset", "WorldGenLevel")), encoding="utf-8")
            out = root / "result.json"
            result = subprocess.run([sys.executable, "-B", "-X", "utf8", str(Path(E.__file__)),
                                     "--game", str(game), "--ia", str(ia), "--core", str(core), "--out", str(out)],
                                    capture_output=True, text=True, encoding="utf-8")
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            data = json.loads(out.read_text(encoding="utf-8"))
            self.assertEqual(data["masterControlled"], ["day", "mild", "poison", "shipwrecked_season_start"])
            for loc in data["locations"].values():
                self.assertEqual({item["name"] for item in loc["items"]},
                                 {"mild", "poison", "shipwrecked_season_start"})
                self.assertEqual(loc["vanilla"], ["day", "weather"])


class MasterControlledTest(unittest.TestCase):
    """提取保留主控候选项，实际分片的 IsMaster 过滤属于消费者。"""

    def test_collects_only_marked_items(self):
        src = """\t\t\t["autumn"] = {value = "default", desc = season_length_descriptions, master_controlled = true, order = 2},
\t\t\t["day"] = {value = "default", desc = day_descriptions, masteroption = true, master_controlled = true, order = 6},
\t\t\t["magma_rocks"] = {value = "default", world = allshipwreckedworlds},
"""
        got = E.master_controlled_items(src)
        self.assertEqual(got, {"autumn", "day"})

    def test_group_is_not_a_master_controlled_item(self):
        src = '''local WORLDSETTINGS_GROUP = {
    ["global"] = {
        items = {
            ["autumn"] = {value = "default", master_controlled = true},
            ["weather"] = {value = "default", options_remap = {master_controlled = true}},
            ["day"] = {options_remap = {image = "day.tex"}, master_controlled = true},
        },
    },
}'''
        self.assertEqual(E.master_controlled_items(src), {"autumn", "day"})

    def test_universal_items_retain_master_candidates(self):
        customize = """local WORLDSETTINGS_GROUP = {
\t["global"] = {
\t\torder = 1,
\t\ttext = STRINGS.UI.SANDBOXMENU.GLOBAL,
\t\tdesc = nil,
\t\titems = {
\t\t\t["autumn"] = {value = "default", master_controlled = true},
\t\t\t["lightning"] = {value = "default", world = {"forest", "cave"}},
\t\t\t["weather"] = {value = "default"},
\t\t},
\t},
}
"""
        labels = {
            "STRINGS.UI.CUSTOMIZATIONSCREEN.AUTUMN": "秋",
            "STRINGS.UI.CUSTOMIZATIONSCREEN.LIGHTNING": "闪电",
            "STRINGS.UI.CUSTOMIZATIONSCREEN.WEATHER": "雨",
        }
        self.assertEqual(E.master_controlled_items(customize), {"autumn"})
        got = E.universal_vanilla_items(customize, labels, {})
        self.assertEqual(set(got), {"autumn", "weather"})
        # 写了 world 的项不属于“通用”，必须排除（否则会盖进火山）
        self.assertNotIn("lightning", got)


class UniversalItemsNestedTableTest(unittest.TestCase):
    def test_nested_table_does_not_hide_world_field(self):
        """prefabswaps_start 里有嵌套的 options_remap，真正的 world 在它后面。

        用 `[^}]*` 取项块会截在嵌套表的右括号上，把 world 丢在块外，
        结果这项被当成“所有位置通用”而错现于火山。
        """
        customize = """local WORLDGEN_GROUP = {
\t["misc"] = {
\t\torder = 2,
\t\ttext = STRINGS.UI.SANDBOXMENU.WORLD,
\t\tdesc = nil,
\t\titems = {
\t\t\t["prefabswaps_start"] = {value = "default", options_remap = {img = "blank_grassy.tex", atlas = "images/customisation.xml"}, desc = starting_swaps_descriptions, order = 20, world={"forest", "cave"}},
\t\t\t["volcano"] = {value = "default", options_remap = {img = "blank.tex"}, desc = yesno_descriptions},
\t\t},
\t},
}
"""
        labels = {
            "STRINGS.UI.CUSTOMIZATIONSCREEN.PREFABSWAPS_START": "开始资源多样化",
            "STRINGS.UI.CUSTOMIZATIONSCREEN.VOLCANO": "火山",
        }
        got = E.universal_vanilla_items(customize, labels, {})
        self.assertNotIn("prefabswaps_start", got, "写了 world 的项不能当成通用项")
        self.assertIn("volcano", got)


if __name__ == "__main__":
    unittest.main()
