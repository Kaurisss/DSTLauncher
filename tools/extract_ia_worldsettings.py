"""把岛屿冒险（Island Adventures）的世界配置并入面板清单；需要游戏源码与模组源码，不执行 Lua。

输出 internal/worldsettings/island-adventures.json，结构与 options.json 不同：
按世界位置分节，因为同一个选项在海难与火山里的适用范围、默认值都不同。

数据来源（缺一不可，缺了直接报错，绝不猜）：
  游戏   scripts/map/customize.lua   选项定义与四张可选值表
         scripts/languages/chinese_s.po  原版可选值的中文标签
  模组   modservercreationmain.lua   海难的选项与图标
         modmain（ia-core）          船、溺水、中毒等核心选项
         scripts/map/sw_locations.lua     海难/火山的位置默认值
         scripts/map/levels/{shipwrecked,volcano}.lua  预设
         languages/ia_sc.po          模组中文标签

用法：
  python tools/extract_ia_worldsettings.py --game "<客户端目录>" \
      --ia <岛跱冒险仓库> --core <ia-core 仓库>
"""
import argparse
import json
import re
import sys
import zipfile
from pathlib import Path

from ia_order import (
    SHIPWRECKED_GROUP_ORDER,
    SHIPWRECKED_HIDDEN,
    SHIPWRECKED_SETTINGS_ORDER,
    SHIPWRECKED_VOLCANO_ISLAND_ONLY,
    SHIPWRECKED_VOLCANO_ISLAND_ONLY_SETTINGS,
    SHIPWRECKED_WORLDGEN_GROUP_ORDER,
    SHIPWRECKED_WORLDGEN_ORDER,
    VOLCANO_GROUP_ORDER,
    VOLCANO_SETTINGS_ORDER,
    VOLCANO_WORLDGEN_GROUP_ORDER,
    VOLCANO_WORLDGEN_ORDER,
)

# 位置 -> 中文名。与 internal/worldsettings 的世界类型常量保持一致。
LOCATION_LABEL = {"shipwrecked": "海难", "volcanoworld": "火山"}
# 模组代码里给 world 字段用的别名，展开成真实位置。
WORLD_ALIAS = {
    "shipwrecked": ["shipwrecked"],
    "volcano": ["volcanoworld"],
    "allshipwreckedworlds": ["shipwrecked", "volcanoworld"],
    "oceanworlds": ["forest", "shipwrecked", "porkland"],
    "allworlds": ["forest", "cave", "shipwrecked", "volcanoworld", "porkland"],
    "iaworlds": ["shipwrecked", "volcanoworld", "porkland"],
    "hamlet": ["porkland"],
}
# 模组用到的可选值表 -> customize.lua 里的表名。
DESC_TABLE = {
    "yesno_descriptions": "yesno_descriptions",
    "enableddisabled_descriptions": "enableddisabled_descriptions",
    "frequency_descriptions": "frequency_descriptions",
    "worldgen_frequency_descriptions": "worldgen_frequency_descriptions",
    "season_length_descriptions": "season_length_descriptions",
    "speed_descriptions": "speed_descriptions",
    "size_descriptions": "size_descriptions",
    # 出生保护用「自动检测 / 总是 / 无」三档，不是自由文本
    "autodetect": "autodetect",
    # 海难/火山的多数频率项用的是这张 ocean 派生表
    "ocean_worldgen_frequency_descriptions": "ocean_worldgen_frequency_descriptions",
}

# 由其他表派生出来的描述表名：它们不写字面量，要按派生规则现算。
DERIVED_TABLES = ("ocean_worldgen_frequency_descriptions",)


def strip_comments(src):
    """去掉 --[[ ]] 块注释与 -- 行注释，避免把示例代码当成真实定义。"""
    src = re.sub(r"--\[(=*)\[.*?\]\1\]", "", src, flags=re.S)
    return "\n".join(re.sub(r"--.*$", "", line) for line in src.splitlines())


def extract_block(src, open_index):
    """从 src[open_index] 的 '{' 开始，返回配对花括号内的内容。

    用括号配对而不是正则，因为表体里有嵌套表（嵌套表不影响行级解析）。
    """
    if open_index >= len(src) or src[open_index] != "{":
        return "", False
    depth = 0
    in_str = False
    for i in range(open_index, len(src)):
        c = src[i]
        if c == '"' and src[i - 1] != "\\":
            in_str = not in_str
            continue
        if in_str:
            continue
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return src[open_index + 1:i], True
    return "", False


def read_po(path):
    text = Path(path).read_text(encoding="utf-8")
    out = {}
    for m in re.finditer(r'msgctxt "(.*?)"\s*\nmsgid "(.*?)"\s*\nmsgstr "(.*?)"', text, re.S):
        out[m.group(1)] = m.group(3)
    return out


def desc_tables(customize_src, labels):
    """解析 customize.lua 里的可选值描述表。

    两类：
      1. 字面量表，例如 `{ {text=STRINGS.UI.SANDBOXMENU.SLIDENEVER, data="never"}, ... }`
      2. 派生表，例如 `ocean_worldgen_frequency_descriptions` 由
         `worldgen_frequency_descriptions` 逐项加上 `ocean_` 前缀而 text 不变。

    注意 text 写作 `STRINGS.UI.SANDBOXMENU.X`，而正则已吃掉 `STRINGS.` 前缀，
    标签表的 key 带完整前缀，拼接时不能丢。
    """
    tables = {}
    for name in set(DESC_TABLE.values()) - set(DERIVED_TABLES):
        # 同名表在多个分支里各有定义，取第一处非空即可。
        # 必须用括号配对取整个表：非贪婪正则 `\{...(.*?)\n\}` 会一路冲到后面
        # 多个表的结尾，把 frequency / worldgen_frequency / speed 等表的取值全
        # 拼进一个表（表现为下拉框里重复十几项）。
        for m in re.finditer(r"\b" + name + r"\s*=\s*\{", customize_src):
            block, ok = extract_block(customize_src, m.end() - 1)
            if not ok:
                continue
            values = []
            for item in re.finditer(r"\{([^{}]*)\}", block):
                body = item.group(1)
                data = re.search(r'data\s*=\s*"([^"]*)"', body)
                if not data:
                    continue
                text = re.search(r"text\s*=\s*STRINGS\.([\w.]+)", body)
                # 正则已吃掉 `STRINGS.` 前缀，而标签表的 key 带完整前缀，拼接时不能丢
                label = labels.get("STRINGS." + text.group(1), "") if text else ""
                values.append({"data": data.group(1), "label": label})
            if values:
                tables[name] = values
                break
        if name not in tables:
            raise SystemExit(f"customize.lua 里找不到 {name}")

    # ocean_ 派生表：`for i, d in ipairs(base) do t[i] = {text = d.text, data = "ocean_"..d.data} end`
    for m in re.finditer(
        r'(\w+)\s*=\s*\{\}\s*\nfor\s+\w+,\s*(\w+)\s+in\s+ipairs\((\w+)\)\s+do\s*\n'
        r'\s*\w+\[\w+\]\s*=\s*\{text\s*=\s*\w+\.text,?\s*data\s*=\s*"([^"]*)"\s*\.\.\s*\w+\.data\}',
        customize_src,
    ):
        derived, base, prefix = m.group(1), m.group(3), m.group(4)
        if derived not in DERIVED_TABLES:
            continue
        if base not in tables:
            raise SystemExit(f"{derived} 的基底表 {base} 未解析到")
        tables[derived] = [{"data": prefix + v["data"], "label": v["label"]} for v in tables[base]]

    for name in DERIVED_TABLES:
        if name not in tables:
            raise SystemExit(f"customize.lua 里找不到派生表 {name}")
    return tables


def parse_item_tables(src, labels, into=None):
    """解析模组 `xxx_table = { 分组 = { 选项 = {value=..., world=..., desc=...} } }`。

    into 传入已有字典时只补充不覆盖：core 先解析，海难后解析，
    同名项（如 dst_boats、poison）应保留 core 的定位，不能被后续文件错写。
    """
    items = into if into is not None else {}
    for block in re.finditer(r"local (\w+_customize_table)\s*=\s*\{", src):
        table_name = block.group(1)
        body, ok = extract_block(src, block.end() - 1)
        if not ok:
            # 空表（如 ia-core 的 `ia_worldgen_customize_table = {}`）没有内容，跳过
            continue
        # 表名决定类别：模组的约定是 xxx_worldgen_customize_table / xxx_settings_customize_table
        if "worldgen" in table_name:
            category = "worldgen"
        elif "settings" in table_name:
            category = "settings"
        else:
            raise SystemExit(f"无法判断 {table_name} 属于世界生成还是世界规则")
        group = None
        group_indent = None
        for line in body.splitlines():
            if not line.strip():
                continue
            indent = len(line) - len(line.lstrip())
            # 分组行：`名字 = {` 结尾。以它的缩进为基准，下一层才是选项。
            head = re.match(r"^(\s*)(\w+) = \{$", line)
            if head and (group_indent is None or len(head.group(1)) <= group_indent):
                group = head.group(2)
                group_indent = len(head.group(1))
                continue
            if group is None:
                continue
            # 行尾可能是 `},`、`} ,` 或 `}`，模组里三种写法都有
            row = re.match(r"^\s+(\w+)\s*=\s*\{(.*)\}\s*,?\s*$", line)
            if not row:
                continue
            name, fields = row.group(1), row.group(2)
            if indent <= group_indent:
                continue  # 与分组同级：不是分组内的选项
            if name in items:
                continue  # 已在更早的文件里定义过：保留先解析到的定位
            value = re.search(r'value\s*=\s*"([^"]*)"', fields)
            worlds = re.search(r"world\s*=\s*(\w+)", fields)
            desc = re.search(r"desc\s*=\s*(\w+)", fields)
            order = re.search(r"order\s*=\s*(\d+)", fields)
            label = labels.get("STRINGS.UI.CUSTOMIZATIONSCREEN." + name.upper(), "")
            if not label:
                raise SystemExit(f"缺少中文标签：{name}")
            items[name] = {
                "name": name,
                "label": label,
                "default": value.group(1) if value else "default",
                "order": int(order.group(1)) if order else None,
                "worlds": WORLD_ALIAS.get(worlds.group(1), []) if worlds else [],
                "desc": DESC_TABLE.get(desc.group(1)) if desc else None,
                "category": category,
                # master_controlled：非主世界（火山、洞穴）游戏不展示。
                # 模组自己的季节项也是这么标记的，不能只过滤原版项。
                "masterControlled": bool(re.search(r"\bmaster_controlled\s*=\s*true", fields)),
                "group": group,
            }
    return items


def adopted_vanilla(src):
    """模组把哪些原版选项追加到某个世界（table.insert(OPTIONS[x].world, ...) 的名单）。"""
    out = {}
    for m in re.finditer(r"local (vanilla_options_in_\w+) = \{(.*?)\n\}", src, re.S):
        out[m.group(1)] = re.findall(r'"(\w+)"', m.group(2))
    return out


def group_desc_defaults(customize_src, labels):
    """原版分组的默认描述表：组内选项没写 desc 时继承它。

    customize.lua 里选项取值用的是 `option.desc or option.group.desc`，
    所以 `["spawnmode"] = {value=...}` 这种不写 desc 的项，实际可选值来自
    所属分组的 desc（如 survivors 组的 autodetect）。

    注意分组名会重名：世界生成与世界规则各有一套 resources / animals /
    monsters / misc，描述表并不相同（前者世界生成频率，后者频率/速度）。
    因此按 (类别, 组名) 两级索引，不能只用组名——用组名会造成后者覆盖前者，
    海难世界生成的那批项就拿不到取值表而退化成输入框。
    """
    out = {}
    # 两个顶层容器：WORLDGEN_GROUP 在前、WORLDSETTINGS_GROUP 在后
    for m in re.finditer(
        r'(WORLDGEN_GROUP|WORLDSETTINGS_GROUP)\s*=\s*\{', customize_src
    ):
        category = "worldgen" if m.group(1).startswith("WORLDGEN") else "settings"
        body, ok = extract_block(customize_src, m.end() - 1)
        if not ok:
            continue
        for g in re.finditer(
            r'\["(\w+)"\]\s*=\s*\{\s*\n\t\torder\s*=\s*\d+,\s*\n\t\ttext\s*=\s*STRINGS\.[\w.]+,\s*\n\t\tdesc\s*=\s*(\w+)',
            body,
        ):
            out[(category, g.group(1))] = g.group(2)
    if not out:
        raise SystemExit("没有解析到原版分组的默认描述表")
    return out


def dynamic_option_values(game_zip, ia_root, core_root):
    """推导“运行时取值”类选项的真实可选值。

    这些在 customize.lua 里没有 desc 表，值来自各处的注册函数：
      task_set    ← AddTaskSet(name, {location=..., name=标签})
      start_location ← AddStartLocation(name, {location=..., name=标签})
    只收源码里真实注册过的值，不凭空造；名字取游戏/模组自己的字符串。
    """
    out = {"task_set": [], "start_location": []}

    def collect(src, kind, names_map):
        fn = "AddTaskSet" if kind == "task_set" else "AddStartLocation"
        # 先把文件内 `local xxx = { ... }` 的变量收起来：
        # 游戏里的洞穴任务集写作 `AddTaskSet("cave_default", taskset_data)`，
        # 第二个参数是局部变量而不是内联表，不解析变量就拿不到它属于 cave。
        local_vars = {}
        for vm in re.finditer(r'local\s+(\w+)\s*=\s*\{', src):
            body, ok = extract_block(src, vm.end() - 1)
            if ok:
                local_vars[vm.group(1)] = body

        for m in re.finditer(fn + r'\(\s*"([\w]+)"\s*,\s*', src):
            name = m.group(1)
            rest = src[m.end():m.end() + 60]
            if rest.lstrip().startswith("{"):
                # 表体里有嵌套表（tasks / required_prefabs），必须括号配对取完整体，
                # 非贪婪正则会截在第一个 '}' 上、匹配失败。
                body, ok = extract_block(src, src.index("{", m.end()))
                if not ok:
                    continue
            else:
                var = re.match(r'(\w+)', rest.lstrip())
                body = local_vars.get(var.group(1)) if var else None
                if body is None:
                    continue
            loc = re.search(r'location\s*=\s*"(\w+)"', body)
            label_key = re.search(r'name\s*=\s*STRINGS\.([\w.]+)', body)
            label = names_map.get("STRINGS." + label_key.group(1), "") if label_key else ""
            out[kind].append({
                "data": name,
                "label": label or name,
                "location": loc.group(1) if loc else "",
            })

    # 游戏本体
    # 原版注册值（游戏本体的 tasksets / startlocations）
    with zipfile.ZipFile(game_zip) as z:
        labels = read_po_text(z.read("scripts/languages/chinese_s.po").decode("utf-8"))
        ia_labels_raw = read_po(Path(core_root) / "languages/ia_sc.po")
        for n in z.namelist():
            if not n.endswith(".lua"):
                continue
            if not (n.startswith("scripts/map/tasksets/") or n.startswith("scripts/map/startlocations")):
                continue
            src = strip_comments(z.read(n).decode("utf-8"))
            collect(src, "task_set", labels)
            collect(src, "start_location", labels)

    # 模组（岛跱冒险与 ia-core）
    # 名称用的是**游戏**的 SANDBOXMENU 字符串（Default / Plus / Dark / VolcanoStart），
    # 不在模组语言包里；只查模组包会把标签退化成原始 key。
    ia_labels = {**ia_labels_raw, **labels}
    for p in Path(ia_root).rglob("*.lua"):
        rel = str(p).replace("\\", "/")
        if "taskset" not in rel and "startlocation" not in rel:
            continue
        src = strip_comments(p.read_text(encoding="utf-8", errors="ignore"))
        collect(src, "task_set", {**ia_labels})
        collect(src, "start_location", {**ia_labels})

    # 去重（同名以先出现的为准）
    for k, vals in out.items():
        seen, uniq = set(), []
        for v in vals:
            if v["data"] in seen:
                continue
            seen.add(v["data"])
            uniq.append(v)
        out[k] = uniq
    return out


def master_controlled_items(customize_src):
    """只收选项直接字段的 master_controlled；实际分片过滤交给运行时。"""
    out = set()
    for item in re.finditer(r'\["(\w+)"\]\s*=\s*\{', customize_src):
        fields, ok = extract_block(customize_src, item.end() - 1)
        if not ok:
            continue
        # 去掉子表，父分组和 options_remap 里的标记不属于该选项。
        while "{" in fields:
            start = fields.index("{")
            nested, ok = extract_block(fields, start)
            if not ok:
                break
            fields = fields[:start] + fields[start + len(nested) + 2:]
        if ok and re.search(r'\bmaster_controlled\s*=\s*true\b', fields):
            out.add(item.group(1))
    if not out:
        raise SystemExit("没有解析到 master_controlled 选项")
    return out


def universal_vanilla_items(customize_src, labels, group_desc):
    """原版里没有 world 字段的选项（所有位置通用），带上分组与可选值。"""
    items = {}
    # 两个顶层容器分开遍历：同名分组在世界生成与世界规则里各有一份，
    # 描述表也不同，不能混在一个字典里。
    for cont in re.finditer(r'(WORLDGEN_GROUP|WORLDSETTINGS_GROUP)\s*=\s*\{', customize_src):
        category = "worldgen" if cont.group(1).startswith("WORLDGEN") else "settings"
        container, ok = extract_block(customize_src, cont.end() - 1)
        if not ok:
            continue
        for group in re.finditer(r'\["(\w+)"\]\s*=\s*\{', container):
            gid = group.group(1)
            body, ok = extract_block(container, group.end() - 1)
            if not ok:
                continue
            for item in re.finditer(r'\["(\w+)"\]\s*=\s*\{', body):
                name = item.group(1)
                # 必须按括号配对取完整块：prefabswaps_start 等项里有嵌套表
                # `options_remap = {img=..., atlas=...}`，用 `[^}]*` 会截在嵌套表的
                # 右括号上，把真正的 `world={...}` 丢在块外，进而误判为“所有位置通用”。
                fields, ok = extract_block(body, item.end() - 1)
                if not ok:
                    continue
                if re.search(r"\bworld\s*=", fields):
                    continue
                desc = re.search(r"desc\s*=\s*(\w+)", fields)
                label = labels.get("STRINGS.UI.CUSTOMIZATIONSCREEN." + name.upper(), "") or labels.get(
                    "STRINGS.UI.SANDBOXMENU." + name.upper(), ""
                )
                items[name] = {
                    "group": gid,
                    "desc": desc.group(1) if desc else group_desc.get((category, gid)),
                    "label": label,
                    "category": category,
                }
    return items


def location_defaults(src):
    """sw_locations.lua 的 AddLocation overrides：这些是位置级默认值，优先于选项注册默认值。"""
    out = {}
    for m in re.finditer(r'location\s*=\s*"(\w+)",.*?overrides=\{(.*?)\n    \},', src, re.S):
        loc, body = m.group(1), m.group(2)
        values = {}
        for row in re.finditer(r"^\s*(\w+)\s*=\s*(\"([^\"]*)\"|[\w.]+),", body, re.M):
            raw = row.group(2)
            values[row.group(1)] = row.group(3) if row.group(3) is not None else raw
        out[loc] = values
    if not out:
        raise SystemExit("sw_locations.lua 里没有解析到位置默认值")
    return out


def presets(src, po):
    """AddSettingsPreset / AddWorldGenLevel：id + 显示名 + 位置。"""
    out = {}
    for m in re.finditer(
        r'Add(SettingsPreset|WorldGenLevel)\(LEVELTYPE\.\w+,\s*\{(.*?)\n\}\)', src, re.S
    ):
        kind, body = m.group(1), m.group(2)
        pid = re.search(r'\bid\s*=\s*"([A-Z_0-9]+)"', body)
        name = re.search(r"\bname\s*=\s*STRINGS\.UI\.CUSTOMIZATIONSCREEN\.PRESETLEVELS\.(\w+)", body)
        loc = re.search(r'\blocation\s*=\s*"(\w+)"', body)
        if not (pid and name and loc):
            raise SystemExit(f"预设定义不完整：{body[:80]}")
        key = "worldgen" if kind == "WorldGenLevel" else "settings"
        label = po.get("STRINGS.UI.CUSTOMIZATIONSCREEN.PRESETLEVELS." + name.group(1), "")
        if not label:
            raise SystemExit(f"预设缺少中文标签：{name.group(1)}")
        bucket = out.setdefault(loc.group(1), {"settings": [], "worldgen": []})[key]
        if all(p["data"] != pid.group(1) for p in bucket):
            bucket.append({"data": pid.group(1), "label": label})
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--game", required=True, help="《饥荒联机版》客户端目录（含 data/databundles/scripts.zip）")
    ap.add_argument("--ia", required=True, help="Island Adventures 仓库根目录")
    ap.add_argument("--core", required=True, help="ia-core 仓库根目录")
    ap.add_argument("--out", default="internal/worldsettings/island-adventures.json")
    args = ap.parse_args()

    game = Path(args.game)
    zpath = game / "data/databundles/scripts.zip"
    with zipfile.ZipFile(zpath) as z:
        customize = strip_comments(z.read("scripts/map/customize.lua").decode("utf-8"))
        vanilla_po = read_po_text(z.read("scripts/languages/chinese_s.po").decode("utf-8"))

    ia, core = Path(args.ia), Path(args.core)
    ia_po = read_po(core / "languages/ia_sc.po")
    main_src = strip_comments((ia / "modservercreationmain.lua").read_text(encoding="utf-8"))
    core_src = strip_comments((core / "modservercreationmain.lua").read_text(encoding="utf-8"))

    descriptions = desc_tables(customize, vanilla_po)
    # 运行时取值项（task_set / start_location）的可选值：从注册点推导
    dyn_values = dynamic_option_values(zpath, ia, core)
    for k, v in dyn_values.items():
        print(f"运行时取值 {k}: {[x['data'] for x in v]}")
    # 模组会引用游戏自己的描述表（如 bermudatriangle 用 worldgen_frequency_descriptions），
    # 这里把取不到中文的值记下来，末尾统一报一次，不阻断生成。
    warnings = []
    items = {}
    # 先核心（决定船、溺水、中毒等共有项的定位）、后海难（补充自己的选项）。
    # 反过来会被海难文件里的同名项覆盖，把 settings 错归成 worldgen。
    master_controlled = master_controlled_items(customize)
    for src in (core_src, main_src):
        parsed = parse_item_tables(src, ia_po)
        master_controlled.update(name for name, item in parsed.items() if item["masterControlled"])
        for name, item in parsed.items():
            items.setdefault(name, item)

    season_start_values = []
    for m in re.finditer(r'\{ text = STRINGS\.UI\.CUSTOMIZATIONSCREEN\.(\w+), data = "([^"]*)" \}', main_src):
        season_start_values.append(
            {"data": m.group(2), "label": ia_po.get("STRINGS.UI.CUSTOMIZATIONSCREEN." + m.group(1), "")}
        )
    if not season_start_values:
        raise SystemExit("没有解析到海难开局季节的可选值")

    adopted = adopted_vanilla(main_src)
    defaults = location_defaults(
        strip_comments((ia / "scripts/map/sw_locations.lua").read_text(encoding="utf-8"))
    )
    preset_map = presets(
        strip_comments((ia / "scripts/map/levels/shipwrecked.lua").read_text(encoding="utf-8"))
        + "\n"
        + strip_comments((ia / "scripts/map/levels/volcano.lua").read_text(encoding="utf-8")),
        ia_po,
    )

    # 模组自己没写 desc 的项，按游戏规则继承所在分组的描述表。
    # 海难与 ia-core 的分组和原版同名（global / resources / animals / monsters / giants），
    # 因此直接沿用原版的分组描述表；没有同名分组的（如 misc）保持手填。
    ia_group_desc = group_desc_defaults(customize, vanilla_po)

    adopt_for = {
        "shipwrecked": adopted["vanilla_options_in_islands"],
        "volcanoworld": adopted["vanilla_options_in_volcano"],
    }

    # 原版里没有 world 字段的选项对所有位置生效（游戏就是这么判定的），
    # 必须并入模组世界，否则会漏掉活动、季节、冒险家、基础资源再生等整组项。
    universal = universal_vanilla_items(customize, vanilla_po, ia_group_desc)
    # 保留所有适用位置的候选项；由消费者按实际分片 IsMaster 过滤。
    for loc in LOCATION_LABEL:
        merged = list(adopt_for[loc])
        for name in universal:
            if name not in merged:
                merged.append(name)
        adopt_for[loc] = merged

    locations = {}
    for loc, label in LOCATION_LABEL.items():
        loc_items = []
        # 火山岛专属项不在这里藏：游戏是「开启火山岛才显示」，
        # 需要由后端看当前值决定，删掉会导致用户开启后仍无法调整。
        hidden = set()
        for item in items.values():
            # 游戏内页面不展示的项不进视图（它们仍在存档里，保存时原样保留）
            if item["name"] in hidden:
                continue
            # 没写 world 的视为所有世界可用（core 的 poison 就是这样）
            if item["worlds"] and loc not in item["worlds"]:
                continue
            out = {
                "name": item["name"],
                "label": item["label"],
                "default": defaults.get(loc, {}).get(item["name"], item["default"]),
                "category": item["category"],
                "group": item["group"],
            }
            if item["order"] is not None:
                out["order"] = item["order"]
            if item["name"] == "shipwrecked_season_start":
                # 这项用的是模组自己内联的取值表（海难四季开局），
                # 表名不在 DESCRIPTION 白名单里，必须单独处理。
                values = season_start_values
            elif item["desc"]:
                values = descriptions[item["desc"]]
            elif item["group"] in ia_group_desc or (item["category"], item["group"]) in ia_group_desc:
                # 没写 desc 时继承所在分组的描述表（游戏就是这个规则）。
                # 分组名重名（世界生成与世界规则都有 resources/animals/monsters），
                # 所以先按 (类别, 组名) 查。
                key = (item["category"], item["group"])
                desc_name = ia_group_desc.get(key) or ia_group_desc.get(item["group"])
                # 源码里 `desc = nil` 的组会解析出字符串 "nil"，它不是表名
                if desc_name in (None, "", "nil"):
                    desc_name = None
                values = descriptions[desc_name] if desc_name else None
            else:
                values = None
            if values is None and item["name"] in dyn_values:
                # task_set / start_location：取该位置可用的注册值，
                # 并保留用户当前值（旧存档可能写着已删除的值）。
                vals = [v for v in dyn_values[item["name"]] if v["location"] in ("", loc)]
                cur = (defaults.get(loc, {}) or {}).get(item["name"], item["default"])
                if cur and all(v["data"] != cur for v in vals):
                    vals.append({"data": cur, "label": cur, "location": loc})
                if vals:
                    values = [{"data": v["data"], "label": v.get("label") or v["data"]} for v in vals]
            if values is not None:
                missing = [v["data"] for v in values if not v.get("label")]
                if missing:
                    # 模组少数项用的是游戏运行时才拼出的描述表，源码里确实没有
                    # 对应中文；提醒并保留 data，由前端按原值展示。
                    print(f"[!] {item['name']} 的部分取值无中文标签，将展示原值：{missing}", file=sys.stderr)
                out["values"] = values
            loc_items.append(out)
        buckets = preset_map.get(loc, {"settings": [], "worldgen": []})
        # 世界规则按游戏内页面的顺序排（截图：全局→活动→冒险家→世界→资源再生→
        # 生物→敌对生物→巨兽）；其他类别保持原排序。
        # 排序表保留截图顺序；master_controlled 候选项仍完整输出。
        ORDER = {
            "shipwrecked": (
                SHIPWRECKED_GROUP_ORDER, SHIPWRECKED_SETTINGS_ORDER,
                SHIPWRECKED_WORLDGEN_GROUP_ORDER, SHIPWRECKED_WORLDGEN_ORDER,
            ),
            "volcanoworld": (
                VOLCANO_GROUP_ORDER, VOLCANO_SETTINGS_ORDER,
                VOLCANO_WORLDGEN_GROUP_ORDER, VOLCANO_WORLDGEN_ORDER,
            ),
        }
        group_order, order, wg_group_order, worldgen_order = ORDER[loc]

        locations[loc] = {
            "label": label,
            "vanilla": adopt_for[loc],
            "defaults": defaults.get(loc, {}),
            "presets": {k: sorted(v, key=lambda p: p["data"]) for k, v in buckets.items()},
            "items": sorted(loc_items, key=lambda o: (o["category"], o.get("order", 0), o["name"])),
            # 世界规则页的分组顺序与组内顺序（按游戏内页面）。
            # 前端把原版采纳项与模组新增项合并后，用这两张表定显示顺序。
            "settingsGroupOrder": group_order,
            "settingsOrder": order,
            "worldgenGroupOrder": wg_group_order,
            "worldgenOrder": worldgen_order,
            # 火山岛开关只控制海难页：火山本身就是那座岛，不存在“开不开”。
            "volcanoIslandOnly": (
                SHIPWRECKED_VOLCANO_ISLAND_ONLY + SHIPWRECKED_VOLCANO_ISLAND_ONLY_SETTINGS
                if loc == "shipwrecked"
                else []
            ),
            # 原版采纳项里运行时取值的项（task_set / start_location）。
            # 除了该位置自己的注册值，也把通用值（location 为空）带上：
            # 否则海难只剩 1 个 start_location，方向键翻不动。
            "dynamicValues": {
                name: [
                    {"data": x["data"], "label": x.get("label") or x["data"]}
                    for x in dyn_values[name]
                    if x["location"] in ("", loc)
                ]
                for name in ("task_set", "start_location")
            },
        }
        if not locations[loc]["presets"]["worldgen"]:
            raise SystemExit(f"{loc} 没有解析到世界生成预设")

    data = {
        "version": 1,
        "gameVersion": (game / "version.txt").read_text(encoding="utf-8").strip(),
        "source": "IslandAdventures scripts/map/levels + modservercreationmain.lua",
        # 游戏 + 模组注册的全部运行时取值（带 location），
        # 原版森林/洞穴页也从这里取，否则它们的 task_set 无法列值（会错拿海难的）。
        "dynamicValues": dyn_values,
        # 游戏 + core/ia 真实标记的完整选项名，去重排序，不按位置裁剪。
        "masterControlled": sorted(master_controlled),
        "locations": locations,
    }
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    for loc, v in locations.items():
        print(f"{loc}: {len(v['items'])} 项，采纳原版 {len(v['vanilla'])} 项，预设 "
              f"{len(v['presets']['settings'])}/{len(v['presets']['worldgen'])}")
    print("已写入:", out)


def read_po_text(text):
    out = {}
    for m in re.finditer(r'msgctxt "(.*?)"\s*\nmsgid "(.*?)"\s*\nmsgstr "(.*?)"', text, re.S):
        out[m.group(1)] = m.group(3)
    return out


if __name__ == "__main__":
    sys.exit(main())
