# DST 世界快照地图格式调查

调查日期：2026-11-07。范围：单个 shard 的最新世界保存点，地表 `forest` 与洞穴 `cave`；读取静态数据，不执行 Lua，不读取玩家保存文件。本文只记录格式和实现边界，不引入产品代码。

## 可立即实施的结论

1. 世界快照解封装后是 Lua 数据序列化，地图在逻辑根字段 `map`，地标在根字段 `ents`。可直接返回表，也可使用 `savedata` / `tablefunctions` 包装（见末节的实际验证）。不能对整份文本用首个 `width=`、`tiles=` 或 `x=` 做正则匹配。
2. 已交叉核实的压缩文本封装是 11 字节 ASCII 头 `KLEI     1D`，后接标准 Base64；Base64 解码结果为 16 字节小端头，随后为 **zlib 流**。这是 zlib wrapper，不是 gzip，也不是裸 DEFLATE。
3. `map.width`、`map.height` 是 tile 格数；一个 tile 为 4 世界单位。实体世界坐标 `(x,z)` 转到以左上为原点且向下增加 z 的预览坐标为 `(width/2+x/4, height/2+z/4)`。图像和地标必须使用同一方向。
4. `map.tiles` 与 `map.tiledata` 是不同引擎字段。`tiledata` 不等于 RGB，也不能将它作为 `tiles` 的同义词。
5. **后续已通过三个真实世界验证 VRSN 地形流：9 字节头 + LE16，精确长度与地形映射全量匹配。** 下文第 70–91 行保留最初调查中发现的第三方错误与当时未知项；后续验证与现行契约见末节及 [源码复用调查](dst-map-reuse-research.md)。

## 世界保存点与字段来源

[saveindex.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/saveindex.lua) 的 `GetSaveData` 经 `TheNet:GetWorldSessionFile(session_id)` 获取世界文件，`OnLoadSaveDataFile` 用 `RunInSandbox` 得到保存表。`SaveIndex:Save` 注释说明它自 2020-09-09 起已停用写入，当前索引工作应同时参考 shard 系列，而不能仅依据这个旧入口。

文件选择的首版约束：在已选中的 shard/session 下，仅列举 `save/session/<session>/` 的直接普通文件，文件名必须完全由十进制数字组成，按数值选最大值。排除 `.meta`、目录、玩家目录中的数字文件；不能递归寻找数字文件。这是本项目的明确选择策略，Lua 中的 `GetWorldSessionFile` 是 native API，并未公开其排序算法。不同 session 不能以保存点数字大小判断哪个 session 有效，应由迁移/索引层先确定当前 session。

[mainfunctions.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/mainfunctions.lua) 的保存代码明确写入：

| 保存路径 | 生成入口 | 用途 |
| --- | --- | --- |
| `map.tiles` | `ground.Map:GetStringEncode()` | 地形 tile 流 |
| `map.tiledata` | `ground.Map:GetDataStringEncode()` | 地图附加数据流 |
| `map.width`, `map.height` | `ground.Map:GetSize()` | 格网尺寸 |
| `map.world_tile_map` | `GetWorldTileMap()` | tile 名称到保存 ID 的映射 |
| `map.nodeidtilemap` | `GetNodeIdTileMapStringEncode()` | 格子与拓扑节点的关联 |
| `map.topology` | `ground.topology` | 节点、多边形、颜色、标识等 |
| `map.prefab` | `ground.worldprefab` | `forest` 或 `cave` |
| `ents` | 按 prefab 分组的实体记录 | 地标位置等 |

[gamelogic.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/gamelogic.lua) 的 `PopulateWorld` 是关键读取契约：先 `map:SetSize(width,height)`；若 `tiledata` 不存在，调用 `SetFromStringLegacy(tiles)`；否则分别调用 `SetFromString(tiles)` 与 `SetMapDataFromString(tiledata)`。随后根据 `world_tile_map` 做 `DoDynamicTileConversion`。这些实际二进制解码器均在 native 引擎，不在 Lua 仓库中。

注意 `server_temp` 文件不是完整世界快照：`WriteServerSaveTempFile` 会清空 `ents` 和 `map.tiles`，并移除 `world_tile_map` 等字段。预览不能从这种文件生成。

## KLEI 压缩文本封装

所有偏移为从 0 起算的字节偏移。

```text
file[0:11] = ASCII "KLEI     1D"  // 五个空格
payload = standard_base64_decode(file[11:])

payload[0:4]   uint32 little-endian = 1   // 封装版本
payload[4:8]   uint32 little-endian = 16  // 头大小
payload[8:12]  uint32 little-endian = 解压后字节数
payload[12:16] uint32 little-endian = zlib 流字节数
payload[16:]  zlib stream -> Lua 文本，通常为 return {...}
```

证据交叉来源：

- [Klei 论坛 Reading save files](https://kleiforums.com/forums/topic/28369-reading-save-files/)：实际逆向与读写示例确认 11 字节外头、16 字节内头、标准 `+`/`/` Base64 字符、zlib，以及四个 32 位字段。
- [DontStarveSaveAPI/save.cpp](https://github.com/onidev/DontStarveSaveAPI/blob/master/save.cpp)：写入固定 `01 00 00 00 10 00 00 00`，并逐字节小端写入解压长度和压缩长度，读取按 11/16 偏移解码。此仓库的 GitHub 元数据未声明许可证，本调查仅用来核对事实，不复制其代码。

Go 可使用 `encoding/base64.StdEncoding`、`encoding/binary.LittleEndian`、`compress/zlib` 和 `io` 完成。先确认 payload 长度至少 16；只接受已支持的版本和头长；核对压缩长度与实际剩余字节数；限制解压上限，解压完成后核对输出长度和 zlib 校验结果。不能把无效的长度字段当作提示而忽略，也不能从任意偏移搜索 zlib 头尝试“恢复”。

论坛另记录 11 字节 `KLEI     1 `（最后一个字节为空格）后直接跟明文的形式。可将它作为独立、精确识别的明文格式；无 KLEI 的 `return {...}` 可用于明确允许的已解码导出文本/测试输入。

**未核实：其他 KLEI 二进制头或 mode 字节。** 不得将 mode 为 NUL、其他字符或者非 ASCII 版本头自动解释为压缩流。本轮没有这类字节样本或明确许可解码器来交叉核实；首版返回“暂不支持此 KLEI 保存格式”，并保留迁移打包功能。

## 静态 Lua 提取

现有 `internal/worldsettings/leveldata_scan.go` 已有注释、字符串、长括号、数字和静态表扫描能力，可复用其扫描思路。保存表应按层级定位：根 `map`，再读取其直接字段 `width/height/tiles/tiledata/world_tile_map/topology/prefab`；根 `ents` 内按已选 prefab 读取位置记录。

表字段顺序不稳定；字符串可能包含大括号、逗号或转义，不能用字符计数或 `strings.Split` 代替词法扫描。支持整数/小数/科学计数法坐标、显式 `[n]=...` 索引和普通数组项。对函数调用、表达式、循环、引用赋值等非字面量语法明确报错；不要执行 `loadstring`、Lua VM 或游戏文件。

体积较大的实体表可扫描跳过无关记录，避免为整个存档构造通用 AST。外部文件边界需要总字节、嵌套深度、尺寸乘积和实体数量上限；具体阈值由实现层统一定义。

## 地图 tile 流：证据等级和不能误用的结论

找到的直接公开解码实现是 MIT 许可的 [Silenttttttt/dst-seed-finder/src/dstseedfinder/tilemap.py](https://github.com/Silenttttttt/dst-seed-finder/blob/main/src/dstseedfinder/tilemap.py)，另有 [技术发现第 8 节](https://github.com/Silenttttttt/dst-seed-finder/blob/main/docs/FINDINGS.md) 与生成图示。它声称根据真实生成存档经验得到：

```text
raw = standard_base64_decode(map.tiles)
raw[0:4] = "VRSN"
raw[4:6] = uint16 LE version 1
raw[6:]  = width*height 个 uint16 LE，row-major
           然后忽略少量尾部字节
```

**这只能作为待验证假设，不能直接定为产品支持格式。** 原因：

- 实现并未检查版本，且不解释尾部长度与字节含义；仓库没有提供原始 `.save`/数字世界快照夹具。
- 文档声称格网中的大部分 ID 无法在 `world_tile_map` 中找到，只能随机着色；这与游戏用 `world_tile_map` 在读盘时重映射 ID 的契约存在明显核对需求。
- 游戏 [tilemanager.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/tilemanager.lua) 的 `GetTileID` 对已有 `GROUND` 名直接返回旧 ID。`AddTile` 将实际 ID 写入 `WORLD_TILES`；不能据第三方的异常大 ID 推断普通草地、森林、海洋的 ID 每个 seed 都随机变化。读取偏移、版本宽度、字节序或记录宽度有误也会造成这种现象。
- 没有独立证据能确定 VRSN 的版本宽度究竟是 2 字节，或者该实现把更宽字段/前缀的一部分当成 tile；同样没有证据证明这里使用 RLE，或证明所有版本都不使用 RLE。

因此首版不应支持“去掉 6 字节、取前 `2*w*h` 字节、忽略剩余”的宽松路径；也不应猜测 `(tile,count)`、`(count,tile)` 或 count+1 等 RLE 规则。必须先有一份来源合法的真实小型世界快照，对 `VRSN` 后完整头部、版本、总长度、tile 映射覆盖率及地标位置交叉验证，再确定唯一支持的流布局。未知版本、字节长度不符、未解释的尾部和 RLE 越界都要明确报错。

`tiledata` 的完整二进制结构本轮也未核实。可以原样提取保存其字符串供后续研究，但不能使用 `tiles` 的假设布局去解码它。地形预览如果只依赖已验证的 tile 类型，不需要模拟引擎全部附加地图数据。

## 坐标、地标和可用的拓扑预览

[worldgen_main.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/worldgen_main.lua) 的 `ShowDebug` 直接用 `pos.x/TILE_SCALE + width/2` 和 `pos.z/TILE_SCALE + height/2`。另见 [forest_map.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/map/forest_map.lua) 的实体生成过程，世界坐标为 `(tile_position - dimension/2)*TILE_SCALE`。这为坐标转换提供游戏自身证据，不只是第三方经验。

根 `ents` 的形状为 `ents={prefab_name={{x=...,z=...,data={...}},...}}`。`gamelogic.lua` 加载时显式补回 `v.prefab = v.prefab or prefab`，所以实体记录缺失 `prefab` 是正常的，外层分组键就是类型。

[entityscript.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/entityscript.lua) 的 `GetSaveRecord` 会把实体位置写入 `x/z`。有平台父对象的记录还可能带局部坐标和平台信息，静态地标首版应选择固定世界实体，避免把船上的对象当作固定地标。

可选地标 prefab 白名单（具体显示名由产品层定义）：`multiplayer_portal`、`multiplayer_portal_moonrock`、`pigking`、`resurrectionstone`、`wormhole`、`cave_entrance`、`cave_entrance_open`、`cave_exit`、`atrium_gate`、`ancient_altar`、`ancient_altar_broken`、`archive_switch`。并非每个保存点都存在每类实体，缺少某一地标应显示为空集合。不要从玩家记录推断出生点。

虫洞连接可参考 `data.teleporter.target` 与保存记录 `id`，但“两个最近虫洞相连”不是格式规则。首版可只标位置，连线需要实际记录 ID 的核实。

若先做拓扑预览：`map.topology.nodes[i].poly` 是一组二维世界坐标点，`cent` 为二维中心；`map.topology.ids[i]` 为对应节点名称；`nodes[i].c` 索引 `topology.colours`。游戏 `DrawDebugGraph` 就以 `graph.colours[node.c]` 的 `r/g/b/a` 绘制节点多边形。Lua 数组从 1 开始；不要误把它直接当 Go 0-based 下标。字段可能缺省，不能靠固定文本顺序拆分。拓扑多边形是区域示意，并非真实 tile 海岸线，UI 应明确称为区域图。

## 颜色

存档没有一张可直接拿来作为所有 tile RGBA 的通用颜色表。`world_tile_map` 是名称到 ID 的映射；`topology.colours` 是拓扑节点的颜色。地形纹理由 [tiledefs.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/tiledefs.lua) 与 [worldtiledefs.lua](https://github.com/taichunmin/dont-starve-together-game-scripts/blob/master/worldtiledefs.lua) 定义，其中 `minimap_tile_def.noise_texture` 引用游戏纹理资源。海洋 ground 另有 `colors.minimap_color`（如 coastal 为 `{23,51,62,102}`），它不构成所有陆地 tile 的颜色表。

首版可以自行设计按名称映射的固定预览配色，通过存档自己的 `world_tile_map` 找 ID。未知合法 mod tile 可显示为明确的未知地形颜色；流格式未知则应报错，不能用随机色掩盖解码错误。

## 样本、验证和许可记录

初步远程调研只访问公开源码及项目扫描器。检查的 GabdaZM/DST-map-editor 和 dst-seed-finder 仓库树均未包含可用的小型原始世界快照；后者只有输出图，不能作为字节格式测试。随后实施阶段的真实快照验证记录见末节。

应区分三种测试证据：

1. 人工构造封装夹具能验证 Base64、LE32 长度、zlib、拒绝未知头以及静态字段提取。
2. 人工构造 tile 数据只能验证假定算法的实现，不能证明实际 DST 编码与该假定一致。
3. 合法公开的真实地表与洞穴快照，或经授权生成的新世界保存点，才能验证格式兼容性。最有价值的检查是完整 payload 十六进制前 32 字节、准确总长度、映射覆盖率，以及若干地标附近 tile 与图像的位置对应。

引用来源及许可：

| 来源 | 许可/用途 |
| --- | --- |
| taichunmin/dont-starve-together-game-scripts | 游戏脚本镜像；以 Klei 游戏脚本为事实依据，不假定其为 MIT，不复制游戏实现到产品 |
| [Silenttttttt/dst-seed-finder/LICENSE](https://github.com/Silenttttttt/dst-seed-finder/blob/main/LICENSE) | MIT；用于核对公开经验算法，本文指出尚未交叉验证部分 |
| [GabdaZM/DST-map-editor/LICENSE](https://github.com/GabdaZM/DST-map-editor/blob/master/LICENSE) | MIT；`rawmap.py` 是已解码 Lua 文本读取器，不提供压缩封装或 tile 二进制解码 |

## 实施阶段的真实快照验证

只读检查项目现有 `dst_save/DST/Cluster_1/` 的直接世界保存点，未读取玩家子目录，未复制完整快照到代码或测试夹具。结果：

| 分片 | 尺寸 | 区域 | 已识别地标 |
| --- | --- | --- | --- |
| Master | 400 × 400 | 306 | 21 |
| Caves | 425 × 425 | 367 | 21 |
| Island | 540 × 540 | 148 | 3 |
| Volcano | 无世界保存点 | — | — |

这批实际文件使用以下序列化结构，而非直接 `return {...}`：

```lua
local savedata = {}
local tablefunctions = {}
tablefunctions["map_fn"] = function()
return { ... }
end
savedata["map"] = tablefunctions["map_fn"]()
return savedata
```

实体分组同样由独立字面量返回表赋给 `savedata["ents"][prefab]`。实现使用词法扫描读取包装结构、提取选定表，再按静态数据解析，不执行函数。游戏文件末尾还包含单个 NUL 终止符。真实颜色表中同时存在 0–1 浮点颜色和 0–255 字节颜色（例如 `a=255,r=255,g=0,b=0`）；需按每个颜色记录转换。

初步核对投影后的区域边界接近各自地图尺寸：森林约 `(14,11)–(386,389)`、洞穴约 `(17,12)–(408,413)`、海难约 `(13,23)–(527,526)`。

## 后续 VRSN 地形流验证与产品替换

继续对同三个真实世界的 `map.tiles` 检查后，确认头部为 `56 52 53 4e 00 01 00 00 00`，完整长度均为 `9+2*width*height`。从偏移 9 按 LE16 读取，森林 160000、洞穴 180625、海难 291600 个地块全部命中各自 `world_tile_map`，含 260、10504 等高 ID。偏移 6/8 会导致错位。当前只支持此完整已验证版本头，拒绝未解释的尾部。

地图底图已替换成逐格地形位图，采用 MIT DMP Web 的名称/配色及 ImageData 构建代码。保留原始地块行序与既有 `x/z` 投影；核对猪王为 WOODFLOOR、森林传送门为 GRASS、远古祭坛为 BRICK、档案馆为 ARCHIVE、海难传送门为 BEACH。地块与地标方向一致。`tiledata` 仍不作为地形 ID 使用。

MIT 来源、固定提交、两个候选解码器偏移缺陷及移植范围见 [源码复用调查](dst-map-reuse-research.md)。
| onidev/DontStarveSaveAPI | 未见明确许可证；只核对封装字段，不复制代码 |
| Klei 论坛 Reading save files | 社区逆向事实与示例，不作为可复制产品代码来源 |

本次没有复制 GPL 源码。建议落地顺序：支持确定的 KLEI 文本封装和静态根表；生成地标/拓扑区域图；在真实样本验证后单独开放确切的 VRSN 地形版本。这样未知格式会如实报错，迁移包仍能按原样保留完整世界文件。
