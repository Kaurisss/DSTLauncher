# DST 地形地图：源码复用调查

调查日期：2026-11-07。任务范围：公开源码、许可证和可移植实现；仅新增本文，不修改产品代码，不访问本地存档，不执行 Lua，不再委派代理。本文采用 research 技能的一手来源与单文件记录流程。

## 推荐实施方向

**复用 MIT 的 DMP 地形配色和 Canvas 渲染，实现真实 tile 地图；解码采用主会话已经用三个真实世界验证的 offset 9 / LE16。** 不再继续寻找一个可以整仓接入的 Python 工具，也不沿用随机 tile 配色或拓扑多边形作为主地图。

最适合当前 Go 标准库后端 + Vue 3 的来源是 `miracleEverywhere/dst-management-platform-api` 与 `dst-management-platform-web`。它们已有完整的地形位图、地标图层、图标图集、缩放和命中检测。可直接移植的最小片段是前端 `worldMap.js` 中的名称/颜色表和 `buildTerrainCanvas`，再从 `WorldMapCanvas.vue` 提取地标绘制；**不要整体移植其存档解析器或 tile 解码器**。

两个候选 Python 项目也已核实：`dst-seed-finder` 确有逐格渲染器，但 6 字节偏移有误；`DST-map-editor` 只有 Lua 文本读取器，仓库并无地图图像渲染代码。

## 已核对的项目、提交与许可证

以下均通过 GitHub API 的 `commits/HEAD`、完整仓库树、具体文件内容和 LICENSE 正文核对，链接固定到此次读取的提交。

| 项目 / 固定提交 | 实际相关代码 | 许可与署名 | 本项目处理 |
| --- | --- | --- | --- |
| [DMP API，13b414f](https://github.com/miracleEverywhere/dst-management-platform-api/commit/13b414f4925f6edd640cae79ea90c7be2554df92) | [dst/map.go](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go) | [MIT](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/LICENSE)，`Copyright (c) 2024 Miracle` | 复用 prefab 元数据、几何标记、图集组织；解码另按真实验证实现 |
| [DMP Web，161908b](https://github.com/miracleEverywhere/dst-management-platform-web/commit/161908b3acb531b5e9639a43b7be17ba6fbd307e) | [worldMap.js](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/worldMap.js)、[WorldMapCanvas.vue](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/WorldMapCanvas.vue) | [MIT](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/LICENSE)，`Copyright (c) 2024 Miracle` | 首选渲染代码来源，抽取小片段适配现有 Vue 对话框 |
| [Silenttttttt/dst-seed-finder，a6a0ee5](https://github.com/Silenttttttt/dst-seed-finder/commit/a6a0ee521772a88e9043ae2c10ce48e5c9ab55ac) | [src/dstseedfinder/tilemap.py](https://github.com/Silenttttttt/dst-seed-finder/blob/a6a0ee521772a88e9043ae2c10ce48e5c9ab55ac/src/dstseedfinder/tilemap.py) | [MIT](https://github.com/Silenttttttt/dst-seed-finder/blob/a6a0ee521772a88e9043ae2c10ce48e5c9ab55ac/LICENSE)，`Copyright (c) 2026 Silenttttttt` | 理解逐格成图和实体叠加，不采用其偏移/随机色/文本搜索 |
| [GabdaZM/DST-map-editor，72a8e84](https://github.com/GabdaZM/DST-map-editor/commit/72a8e8447fb81b38743cd10638dfd2757532ddcc) | [rawmap.py](https://github.com/GabdaZM/DST-map-editor/blob/72a8e8447fb81b38743cd10638dfd2757532ddcc/rawmap.py)；完整仓库只有 README、LICENSE、rawmap.py 和 .gitignore | [MIT](https://github.com/GabdaZM/DST-map-editor/blob/72a8e8447fb81b38743cd10638dfd2757532ddcc/LICENSE)，`Copyright (c) 2018 Gabda87` | 不引入；现有严格静态提取器已经更适用 |

另搜索了实际面板和地图查看器候选。`tmpprojects/dst-map` 的 [README](https://github.com/tmpprojects/dst-map/blob/844856fc53bf1eb5e2440fb62fb96a85cfc65a63/README.md) 说明其为 Digital Services Tax 世界地图，排除；`Zerovate/KLTilemap` 是 Godot 地形实现，无明确根 LICENSE，未作为 DST 存档解码或可复制代码来源。检索到 GPL 面板 `fornax-d/dst-fish-manager`，未复制其代码。已有 DMP 的完整 MIT 实现后停止扩大检索。

## 二进制布局：真实验证优先于有缺陷的第三方解码

### 主会话提供的真实样本验证

以下是主会话使用现有严格静态解析器的验证结果，由主会话提供给本调查；本调查代理没有打开存档。

| 世界 | tile 数 | offset 9 按 LE16 解码后命中该存档 world_tile_map |
| --- | --- | --- |
| Master | 160000 | 160000 / 160000 |
| Caves | 180625 | 180625 / 180625 |
| Island | 291600 | 291600 / 291600 |

三个 `map.tiles` 的 Base64 解码后长度均**精确**为 `9 + 2*width*height`，前九字节均为：

```text
56 52 53 4e 00 01 00 00 00
 V  R  S  N 00 01 00 00 00
```

据此可实施的读取契约是：标准 Base64 解码，核对上述已验证头部、精确长度，然后 `raw[9+2*i : 11+2*i]` 按 uint16 little-endian 读取，行优先，行宽为 `width`。实现可以识别 `raw[:5] == "VRSN\x00"` 与 `LE32(raw[5:9]) == 1`，或比较九字节完整已知头；**五字节 magic + LE32 version 是与样本一致的头部语义解释**，外部 native 格式规范尚未找到，不把其他版本也纳入支持。

主会话同时报告：`tiledata` 长度相同，但内容不是 tile ID。本轮地形预览只需 `tiles`，不把两个字段混用；相同长度不构成相同语义。

Go 落地核心读法（本次根据验证结果表达的最小算法，不来源于有缺陷的第三方解码）：

```go
// 外层已检查尺寸、Base64、完整九字节头和 len(raw) == 9+2*width*height。
ids := make([]uint16, width*height)
for i := range ids {
    offset := 9 + 2*i
    ids[i] = binary.LittleEndian.Uint16(raw[offset : offset+2])
}
```

### 为什么不复制候选项目的 VRSN 解码

`dst-seed-finder/tilemap.py` 的 `decode_tilemap` 明确使用 `body_bytes = raw[6:]`、`struct.unpack("<...H", body_bytes[:n*2])`，忽略尾部，且没有校验版本。注释称大多数 tile 不在 `world_tile_map` 中，于是用哈希 HSV 配色。主会话的 100% 名称映射覆盖率直接推翻了“无法从存档得知地形名”的解释。该项目可证明逐格渲染路径真实存在，但它的二进制布局不能移植。

DMP 的 [parseMapSection，第 672–747 行](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L672) 使用 `offset := 8+i*2` 与 `binary.BigEndian.Uint16(...)`，仅检查前四字节 `VRSN`，仅检查最短长度；也不符合已验证布局。它解析出 ID 后 [encodeMapTiles，第 346–357 行](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L346) 再反转行序、按 BE16 输出自己的 API 载荷。因此前端 `decodeTiles` 的 BE16 是 **DMP 私有 API 协议**，不是证明 DST 原始流使用 BE16。

更精确地说，设真正的 tile 为低字节 `L[i]`、高字节 `H[i]`，DMP 第 i 项读取到 `256*H[i-1]+L[i]`，第一项前置字节来自头的末尾 0。当所有地形 ID 小于 256 时，这个错位读取恰好得到正确值；遇到高 ID，就丢失当前高字节并污染下一格。这解释了为何错误解码器可能仍能画出看似正常的原版地图。不要据图像外观跳过高 ID 与映射覆盖率检查。

本轮没有找到另一份明确使用九字节 VRSN 头的独立公开源码。当前实现选择有三个真实样本的直接字节证据；外部来源负责提供许可清楚的渲染代码，不用它们覆盖已验证的格式事实。

## world_tile_map、方向与 tiledata 的边界

`world_tile_map` 是 `{NAME=id}`，应反转成 `id -> NAME` 后查按名称维护的颜色表。DMP 的 [parseTileNames](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L732) 和 [buildMapTileSpecs](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L414) 也是这个方向；未知 ID 被标为 `TILE_<id>`。当前项目可保留现有静态表提取，只复用映射和显示数据的组织方式，不需要引入 DMP 的逗号拆分或正则。

本项目现有投影是 `(width/2+x/4, height/2+z/4)`，与原始行序一起使用即可。DMP 采用北向上的另一种展示约定：后端先反转地形行序，前端 [worldToMap / projectWorld](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/worldMap.js#L1005) 使用 `-z`。移植 Canvas 时应保留当前项目整套方向，或将 tile 行序与实体 z 投影同时翻转；不要只复制 DMP 的 `-z`。

`rawmap.py` 不解码 `tiles`、`tiledata` 或 KLEI 压缩，仅把明文 Lua 式结构拆成 Python 列表/字典。`tilemap.py` 与 DMP `parseMapSection` 都只为预览读取 `tiles`，本轮没有从这些来源获得 `tiledata` native 结构。游戏写入/读取字段的已有一手证据见 [dst-map-format.md](dst-map-format.md) 中 `mainfunctions.lua` 与 `gamelogic.lua` 的引用。

## 可以直接移植的渲染片段

### 1. 地形名称、颜色和底图

首选 [worldMap.js，第 79–225 行](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/worldMap.js#L79) 的 `GROUND_LABELS` / `TILE_COLORS`。这是一张现成的按名字维护的配色表，不依赖 NumPy、Matplotlib、Lua、WebGL 或游戏纹理。它包含地表、洞穴、海洋和大量新地皮。代表色如下（全部来源于同表）：

| 地形 | 色值 | 地形 | 色值 |
| --- | --- | --- | --- |
| GRASS | `#4e9a5f` | FOREST | `#2f6b46` |
| SAVANNA | `#b9b86a` | ROCKY | `#a9a293` |
| MARSH | `#5b4a6b` | DESERT_DIRT | `#d9c18a` |
| CAVE | `#3d4753` | MUD | `#6b5540` |
| FUNGUS | `#3f7fa8` | FUNGUSRED | `#a8524f` |
| FUNGUSGREEN | `#4f8a55` | FUNGUSMOON | `#5aa3a0` |
| OCEAN_COASTAL | `#2a7fb8` | OCEAN_SWELL | `#20689a` |
| OCEAN_ROUGH | `#174d75` | OCEAN_BRINEPOOL | `#4fa3b8` |

移植时按存档名称查表，不固定数字 ID。Island 的模组地形若未收录，使用一种明确的未知地形颜色，并在图例保留其内部名；不要把 seed-finder 的随机多色替代地形识别。DMP 自身对未知名会哈希成颜色，本项目无需照搬这个选择。

底图移植 [buildTerrainCanvas，第 733–801 行](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/worldMap.js#L733)：创建 `width × height` 离屏 canvas；对 `tiles[y*width+x]` 查缓存 RGB；写入单个 `ImageData`；最后只 `putImageData` 一次。邻格 ID 不同则亮度乘 0.9，用 `hashCell` 作 0.96–1.04 的轻微明度变化。移植该函数时同时带走实际用到的 `hashCell`、`rgbToHex`、颜色转换；不需要纹理时删掉 `averageColor` / textures 分支。

这能直接保留真实海岸线、道路、岛屿和地皮边界，地标叠加在底图上。数据变化时重建底图；缩放/拖动只重绘缓存图像与标记，避免 Vue 为几十万个格子创建 DOM 或 SVG 元素。低缩放使用位图，高缩放也可继续放大它；首版无需逐格纹理。

### 2. 地标选择与图标分层

DMP API 的 [mapCuratedPrefabs，第 1308–1405 行](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L1308) 提供真实的 prefab 名、中文名和 layer，可移植其中高价值静态项：`pigking`、`multiplayer_portal`、`multiplayer_portal_moonrock`、`moonbase`、`oasislake`、`beequeenhivegrown`、`hermithouse`、`cave_entrance_open`、`monkeyisland_portal`、`terrarium` 等。与已有洞穴地标（洞口/出口、远古祭坛、档案馆等）合并为明确白名单即可。

该表也包含会移动或季节性出现的 Boss；如果显示，应标成“保存时位置”，不能把它当永久生成点。表中文字还有 `twiggytree` 被标成白桦树等错误，不应连带复制全部资源翻译。当前范围优先复用静态地标子集。

前端 [WorldMapCanvas.vue，第 467–564 行](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/WorldMapCanvas.vue#L467) 的可移植策略：

- 地标常驻，默认圆点加白描边；建筑为方块、巢穴为三角形，资源为小点。
- 有 atlas 且 zoom ≥ 6 时，用图集矩形 `drawImage` 替代形状；图标大小限在 8–96 像素。
- 只处理视口内标记；`other` 在 zoom < 3.5 时隐藏，掉落物在 zoom < 8 时隐藏。
- [名称标注，第 566 行起](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/WorldMapCanvas.vue#L566) 使用屏幕空间防重叠；地标在 zoom ≥ 6、建筑与巢穴在 zoom ≥ 12 才显示名称。首版也可让现有旁侧地标列表承担查名，点击后定位。

虫洞连接应使用保存记录 ID 与 `data.teleporter.target`。DMP 的 [parseWormholes，第 1100 行起](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L1100) 按引用配对；其 Lua 表执行路径不适合本项目，字段关系可用现有静态解析器实现。没有目标引用时仅画入口，不按距离猜配对。

### 3. 游戏风格图标和纹理的后续来源

DMP 并非把一批 PNG 名称写死后直接分发全部游戏美术。它的 [ExtractMapAssets，第 2026–2145 行](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L2026) 从本机游戏 `data/images` / `data/minimap` 的 XML/TEX 图集中提取需要的资源；调用 `utils.Unpack` 转图，再 [BuildMapIconAtlas，第 1709 行起](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L1709) 合成 PNG 图集，返回每个 icon 的矩形。

其 [mapPrefabIconName](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L1484) 解决 `deciduoustree -> tree_leaf`、`flower -> flowers`、`flower_cave -> bulb_plant` 等名字不一致；资源查找还尝试 `minimap_icon_<prefab>`、`minimap_<prefab>`、`<prefab>_icon`，且排除 inventoryimages，防止误用背包图标。这些是以后扩充真实图标时有用的具体实现。

地形贴图通过 [loadMapTileDefs / parseMapTileDefs，第 1792–1902 行](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go#L1792) 取得 `noise_texture` 与 `colors.minimap_color`，高缩放 [drawPatternTerrain](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/tools/world-map/WorldMapCanvas.vue#L316) 将同 tile 的可见矩形合成 `Path2D` 后填纹理；低缩放使用平均色位图。

**本轮不建议整体移植资源链。** DMP 的 `parseMapTileDefs` 调用 Lua `DoString`，`ExtractMapAssets` 依赖其 TEX 工具代码；本项目当前明确只做静态 Lua 解析并使用 Go 标准库。首版移植纯色地形 + 几何地标就能完成替换；需要游戏原图时，再独立核对 TEX 解码器来源及本地资源读取实现。仓库 MIT 许可证覆盖其代码，不能将它当成 Klei 美术素材的再许可；优先从用户已安装的游戏读取资源，不把第三方仓库游戏 PNG 当作 MIT 图标包抄入。

## 主会话可直接执行的最小移植清单

1. 后端在现有安全 KLEI/Lua 提取之后，按已验证九字节头 + LE16 读 `map.tiles`。保留 `width/height/world_tile_map`，不读玩家存档，不解释 `tiledata`。
2. 后端输出紧凑 tile 数据和按 ID 索引的名称/颜色，或用 Go `image/png` 生成 `width × height` 地形 PNG。若选择 PNG，可把 DMP 配色表翻译成 Go 字典；若选择前端 Canvas，直接移植上述 JS 表与位图构造函数。沿用现有接口组织，首版无需照搬 DMP 自定义压缩物体协议。
3. Vue 现有地图对话框用地形位图替换拓扑多边形底图，继续用同一坐标变换叠加已有地标。抽取 DMP 几何标记/标签防重叠；首版只显示静态高价值地标。
4. 导入代码处注明来源提交与 MIT；发行物的第三方许可中包含 Miracle 的完整 MIT 正文。若确实导入 seed-finder 或 Gabda 的实质代码，分别增加其版权通知及 MIT 正文；只采用事实、没有采用代码时无需把未使用项目声明为代码依赖。
5. 验证地表、洞穴和 Island 的完整长度与映射命中率；增加一个 ID > 255 的最小行为用例，确保不退回 DMP 的错位 BE16；用非对称地图与地标检查上下方向。主会话真实样本验证已完成读取算法部分，渲染落地后再做图像和交互验证。

## 本文引用的 DMP 片段许可正文

上文颜色值及移植来源为 DMP MIT 代码；导入产品代码时也应将以下通知随副本或实质部分保留，而不只留下 GitHub 链接。

```text
MIT License

Copyright (c) 2024 Miracle

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
