# 岛屿冒险（Island Adventures）世界配置适配

## 为什么需要单独适配

原版只有 `forest` / `cave` 两种世界，配置项由游戏自己的 `scripts/map/customize.lua` 决定。岛屿冒险新增了 `shipwrecked`（海难）与 `volcanoworld`（火山）两种位置，并：

- 新注册约 120 个配置项（竹子、火山爆发、潮汐、溺水等）；
- 把一部分原版选项「追加」到海难/火山（`table.insert(OPTIONS[x].world, ...)`）；
- 用位置级默认值覆盖部分原版默认（火山默认小地图、关船、关陷阱）；
- 提供自己的世界规则预设与世界生成预设。

所以面板不能把原版清单按 `forest` 过滤后照搬，必须按位置组装：原版采纳项 + 模组新增项，默认值取位置覆盖值。

## 数据从哪来

`internal/worldsettings/island-adventures.json` 由 `tools/extract_ia_worldsettings.py` 生成；配置值与主控标记来自源码，显示顺序另按游戏截图记录在 `tools/ia_order.py`：

| 内容 | 来源 |
|---|---|
| 原版选项定义、四张可选值表 | 游戏 `data/databundles/scripts.zip` → `scripts/map/customize.lua` |
| 原版可选值中文 | 同上 → `scripts/languages/chinese_s.po` |
| 模组选项、分组、默认值、图标名 | 模组 `modservercreationmain.lua` 与 ia-core |
| 位置默认值 | 模组 `scripts/map/sw_locations.lua` |
| 世界规则/生成预设 | 模组 `scripts/map/levels/{shipwrecked,volcano}.lua` |
| 模组中文标签 | ia-core `languages/ia_sc.po` |

重新生成：

```sh
python tools/extract_ia_worldsettings.py \
  --game "C:\Program Files (x86)\Steam\steamapps\common\Don't Starve Together" \
  --ia <IslandAdventures 仓库> --core <ia-core 仓库>
```

脚本遇到缺失的中文标签、无法判断类别、解析不到预设时会直接报错退出，不会静默产出半份清单。

## 读写哪个文件

游戏在 `saveindex.lua` / `shardindex.lua` 里写死了两个文件的分工：

```
leveldataoverride — GAME USE，完整世界定义，完全覆盖已保存的世界数据
worldgenoverride  — USER USE，可选 preset 或部分 overrides，叠加在上面
```

客户端生成过世界的存档只有 `leveldataoverride.lua`，游戏只认它。因此：

- 分片目录里**有** `leveldataoverride.lua` → 面板读写它，世界位置（location）也从它来；
- **没有** → 沿用 `worldgenoverride.lua`，用于新建房间首次生成前配置。

写 `leveldataoverride.lua` 时的约束：

- 只替换 `overrides` 里被点名的键，其余行逐字保留（包括 `required_prefabs`、`substitutes`、`version` 等面板清单外的字段）；
- 已有键只替换其值区间并沿用类型与引号风格，缺的键补分隔符后追加到块尾；
- 按静态 Lua 表结构区分直接子项与嵌套内容，支持 Tab、注释与长字符串；函数调用、拼接等动态表达式明确拒绝，不执行 Lua；
- 写前把原文件复制为 `.bak`；
- 世界生成类选项拒绝写入——存档生成后改了也不会重建地图。

往返一致性由 `internal/worldsettings/leveldata_real_test.go` 保证，它用真实存档断言「原值写回后逐字节不变」：

```sh
DST_REAL_SAVE=<存档根>/DST/Cluster_N go test ./internal/worldsettings -run TestRealSave
```

## 前端行为

- 世界类型以接口返回的 `location` 为准，不看 `rooms.json` 里的 `worldType`——后者可能是旧数据（海难分片还写着 forest）。
- 使用 `leveldataoverride.lua` 时：隐藏预设下拉与「启用本文件」开关（游戏固定生效），世界规则可改，世界生成只读。
- 载入快照与待保存增量分开；恢复默认也显式写入，改回载入值才撤销增量。只有可见且偏离默认的选项计入“已改”数量。
- 普通 `worldgenoverride.lua` 提交保留快照中的未知模组键、隐藏项与只读生成项；后端对已有文件也仅替换发生变化的值区间，保留单引号、布尔、数字、嵌套表及同行配置的源字节。无法静态解析时拒绝编辑。`leveldataoverride.lua` 只提交真正编辑过的规则。
- 已有原版或模组存档均在 API 层拒绝修改生成参数；普通文件允许原生成值随规则请求一并提交，但不允许改变它或生成预设。
- 重置只恢复当前可见、可编辑的选项，不清除隐藏项、未知键或已生成世界的生成值。
- 接口在读取文件前统一房间与分片目标；正文与查询参数不一致时拒绝保存。
- `masterControlled` 名单由游戏和模组源码提取，运行时按实际分片 `IsMaster` 过滤，不按 location 推断主副世界。
- 接口返回完整候选分组及 `volcanoIslandOnly` 依赖名单；前端按草稿中的火山岛开关即时显隐，缺省使用源码默认值 `none`，不丢弃隐藏项原值。
- 火山副分片按截图展示 26 项规则和 9 项生成配置。
- 默认停在有内容的类别标签，避免打开就是空列表。

## 已知边界

- 目前只适配海难与火山；`porkland`（哈姆雷特）在模组里有注册项，但未提取。
- 模组的预设本身仍是数字与字符串直出，面板不解释其含义。
- 图标目录按素材来源分类（`common` / `dst` / `island-adventures` / `island-adventures-core`），不代表选项适用哪个世界。
