# 世界配置图标

## 目录约定

运行时图标位于 `web/public/world-icons/`：

- `common/`：基础世界、巨人国等可复用的通用图标，例如草、树、世界大小、季节和普通生物。
- `dst/`：联机版扩展内容与多人机制的图标，例如蜂后、月岛、裂隙、幽灵及节日活动。
- `island-adventures/`：从岛屿冒险 HUD 图集切出的完整素材，保留 XML 中的原名，包括尚未使用的变体。

目录按素材归属整理，不代表世界配置的适用范围；海难复用原版选项时可以继续引用 `common/` 或 `dst/`。选项是否适用于某个世界由配置元数据负责，不由图标目录推断。

`web/src/lib/worldIcons.js` 是配置键到素材路径的统一映射。调用方继续使用 `iconOf(name)`；新增图标无需修改视图。

- 配置键与图片名可能不同，例如 `dragoonegg` → `dragooneggs.png`。
- 多个配置项引用同一张图片，例如 `rock_obsidian` 与 `rock_obsidian_regrowth`。
- `cave_ponds` 与 `ponds` 原有图片像素完全一致，统一引用 `common/ponds.png`。
- 同名不代表同图：岛屿冒险的 `trees.png`、`birds.png`、`grass.png` 等与原版不同，不能相互覆盖。原版 `trees` 与海难 `shipwrecked_trees` 分别映射到各自素材。

## 原始素材与重新切图

`island-adventures/customization_shipwrecked.png` 和同名 XML 是用户提供的已解码 HUD 图集与 UV 索引。图集为 2048×1024，共 89 个 128×128 图标。原始图集放在工具目录，不随前端静态资源打包。

在仓库根目录执行（Python 环境需有 Pillow）：

```sh
python tools/extract_world_icons.py tools/world-icon-sources/island-adventures/customization_shipwrecked.png tools/world-icon-sources/island-adventures/customization_shipwrecked.xml web/public/world-icons/island-adventures
```

只校验已有切片与原图是否逐像素一致：

```sh
python tools/extract_world_icons.py tools/world-icon-sources/island-adventures/customization_shipwrecked.png tools/world-icon-sources/island-adventures/customization_shipwrecked.xml web/public/world-icons/island-adventures --check
python -m unittest discover -s tools -p test_extract_world_icons.py
node web/scripts/check-world-icons.mjs
```

切图脚本按 XML 的像素中心 UV 裁切并转换 V 轴方向，保留 RGBA；不缩放图片，也不执行模组 Lua。

## 来源

- `common/` 与 `dst/` 原版图标统一为 128×128 PNG：从本机《饥荒联机版》安装目录 `data/images/worldgen_customization.tex`、`worldsettings_customization.tex` 经 `tools/texcli.exe decode --unpremultiply` 解码为 PNG 后，按同名 `.xml` 的像素中心 UV（`tools/extract_world_icons.py`）和 `scripts/map/customize.lua` 映射切出。
- 岛屿冒险素材由用户从模组 HUD 提取；配置键与图片名称的对应关系参考模组的 [modservercreationmain.lua](https://gitlab.com/IslandAdventures/IslandAdventures/-/blob/main/modservercreationmain.lua)。

游戏与模组美术素材的权利归各自权利人所有。
