# DST 开源管理工具调研

调研日期：2026-11-07（环境日期）。本次只研究并记录，不修改产品代码。

覆盖四个独立项目：DMP（含对应前端，桌面客户端仅作多实例能力补充）、dst-admin-go、DstServerHelper、dst-admin。检索用于发现仓库，功能结论以官方 README 与实际源文件为依据；以下源码链接固定到本次读取的提交。未运行这些项目，结论表示源码中已有实现或 README 明确声明，不表示已在当前游戏版本完成端到端验证。

## 结论与本项目基线

最值得借鉴的是 DMP 的**配置收藏并新建房间、按事件发送 Webhook、云端存档迁移**，其次是 dst-admin-go 的**地图预览和长期玩家统计**。它们对应复用开服配置、离开面板后获知故障、宿主机损坏后恢复，以及观察世界和社区的实际需要。启动故障诊断和工坊 ID 归因也值得做，但本次未找到足够证据将其写成这些项目已经完成的完整诊断产品。

对照本项目 [README](../README.md) 和 [已有 dst-admin-go 对比](dst-admin-go-comparison.md)，并纳入主会话核实的实现基线：

- 已有多房间、任意分片、模组独立配置、共享下载与版本检查、本地 ZIP 备份轮转/下载/恢复、存档导入与游戏原生快照回档。
- 已有定时备份、每日重启、服务端及模组更新检查，更新支持 `notify/auto/idle` 三模式；维护前有 60/30 秒公告，保存后停服，再恢复此前运行的分片。
- 已有崩溃指数退避和次数上限；已有持久化玩家身份、聊天、进出、死亡等历史；已读取世界 `.meta` 的天数与季节。
- 当前登录为共享密码与 Cookie，没有独立用户/角色；备份存储目前为本机 ZIP。

这些能力均不作为新增建议。下文将“外部已实现事实”和“我方产品建议”分开。

## 仓库与可见维护记录

通过 GitHub 仓库 API 及默认分支最新提交核实真实仓库；日期均为提交的 UTC 日期。`archived=false` 只表示未归档。最新提交时间只是可见记录，不据此推导维护质量或未来承诺。

| 项目 | 默认分支及本次读取提交 | 最新提交日期 | 可见状态与入口 |
| --- | --- | --- | --- |
| DMP API | `master` / `13b414f4925f6edd640cae79ea90c7be2554df92` | 2026-11-07 | 未归档；[仓库 API](https://api.github.com/repos/miracleEverywhere/dst-management-platform-api)、[该提交](https://github.com/miracleEverywhere/dst-management-platform-api/commit/13b414f4925f6edd640cae79ea90c7be2554df92) |
| DMP Web（同一产品） | `master` / `161908b3acb531b5e9639a43b7be17ba6fbd307e` | 2026-11-06 | 未归档；[仓库 API](https://api.github.com/repos/miracleEverywhere/dst-management-platform-web)、[该提交](https://github.com/miracleEverywhere/dst-management-platform-web/commit/161908b3acb531b5e9639a43b7be17ba6fbd307e) |
| dst-admin-go | `main` / `d189b9e215f073de2973242f72aacc3bd8747016` | 2026-08-27 | 未归档；[仓库 API](https://api.github.com/repos/carrot-hu23/dst-admin-go)、[该提交](https://github.com/carrot-hu23/dst-admin-go/commit/d189b9e215f073de2973242f72aacc3bd8747016) |
| DstServerHelper | `master` / `6bfcb0a5f412124dd021f41e56ccd4dedd676515` | 2024-02-28 | 未归档；最近默认分支提交为 README 更新；[仓库 API](https://api.github.com/repos/sukeme/DstServerHelper)、[该提交](https://github.com/sukeme/DstServerHelper/commit/6bfcb0a5f412124dd021f41e56ccd4dedd676515) |
| dst-admin | `master` / `0925bdccc28f3ce53b9878752861b58b393950a3` | 2024-03-04 | 未归档；该提交合并 Docker 构建修复；[仓库 API](https://api.github.com/repos/qinming99/dst-admin)、[该提交](https://github.com/qinming99/dst-admin/commit/0925bdccc28f3ce53b9878752861b58b393950a3) |

## 1. DMP：配置复用、协作权限与外部集成最有参考价值

官方入口：[API README](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/README.md)、[Web README](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/README.md)。后者明确静态构建产物供 API 项目的 `embedFS/dist` 使用，二者算同一个面板。API README 声明 Linux/macOS 完整支持、Windows 通过 WSL 支持。

| 核查维度 | 外部已实现及边界 | 一手证据 |
| --- | --- | --- |
| 多用户、按房间权限 | 用户模型含 `Role`、`Rooms`、`RoomCreation`、`MaxWorlds`、`MaxPlayers`。`hasRoomPermission` 允许管理员或房间 ID 被分配给该用户；创建权限单独判断。这是面板用户权限，区别于游戏 `adminlist.txt`。 | [用户模型](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/database/models/user.go)、[权限检查](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/room/utils.go#L88) |
| 配置/模组方案 | 房间“收藏”保存 `RoomData`、`WorldData`、`RoomSettingData`，重置本地关联 ID；世界结构保留世界配置与模组配置。应用收藏实际创建新房间、分配端口、写配置并注册任务。属于配置模板，不是复制运行中世界存档。源码明确收藏保留 token，照搬到我方时应另行设计凭据处理。 | [快照结构](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/room/utils.go#L820)、[应用收藏](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/room/handler.go#L1361)、[前端收藏页](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/views/game/favorite.vue) |
| Webhook 通知 | 有房间级、全局级配置，可按事件订阅；全局还可筛选房间。事件包括启动、停止、更新、备份、守护触发、玩家变化、配置变化。发送器 POST JSON，可用 HMAC-SHA256 签名，并有测试发送。发送为异步一次提交，源码没有持久化投递队列。 | [事件和配置类型](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/webhook/types.go)、[实际发送与签名](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/webhook/sender.go)、[前端 API](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/api/room.js#L73) |
| 外部存档存储/迁移 | 有云存档上传、列表、下载和导入链路；上传打包房间并附房间/世界/设置元数据，导入可新建房间。上传明确要求 `link.Plan == "ultimate"`；云账号依赖 DMP 自有服务。这是有源码的云服务集成，不应写成免费通用对象存储备份。未在本次核查路径中发现 S3/WebDAV 配置适配，也没有将此接口证明为自动定时异地备份。 | [云路由](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/cloud/router.go)、[上传和导入](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/cloud/archive.go#L34)、[云服务固定地址](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/constant/cloud.go)、[前端云存档 API](https://github.com/miracleEverywhere/dst-management-platform-web/blob/161908b3acb531b5e9639a43b7be17ba6fbd307e/src/api/cloud.js#L60) |
| 自动维护、版本更新 | 有定时备份及清理、守护、通知任务，游戏更新完成触发 Webhook；模组有查询、下载状态、启停和配置 API。属于面板调度并调用游戏/Steam 更新机制。 | [任务注册](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/scheduler/jobs.go)、[房间任务](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/scheduler/room.go)、[游戏更新调度](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/scheduler/global.go)、[模组路由](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/mod/router.go) |
| 玩家管理/统计 | 在线玩家、名单、身份映射、聊天，另有在线时长甘特图、在线时长统计和玩家数曲线接口。面板统计分析不同于已经存在的原始聊天记录。 | [玩家路由](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/player/router.go)、[处理器](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/player/handler.go) |
| 地图 | 有地图和地图玩家接口，服务端有实际地图解析/渲染及地标筛选逻辑。 | [工具路由](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/tools/router.go#L23)、[地图实现](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go) |

**远程多节点的准确表述：**独立官方 [DMP Desktop README](https://github.com/miracleEverywhere/dst-management-platform-desktop/blob/9c86cd09cc6440868fb1b063ecc8b5f910b60fb2/README.md) 写明“同时管理多个 DMP”，并要求先部署 DMP。该仓库本次可见最新提交为 2026-10-14，未归档（[API](https://api.github.com/repos/miracleEverywhere/dst-management-platform-desktop)）。因此可以确认客户端聚合多个面板实例；本次证据没有建立 API 后端拥有统一远程 Agent 注册、节点调度与跨节点资源池的架构。游戏分片配置中的主节点地址、Web SSH 终端也不能单独作为这种架构的证据。

README 中合作云服务商广告的“节点缓存”“每日备份”“SLA”等，未计入 DMP 开源代码能力。

## 2. dst-admin-go：地图、统计与任务模型

官方入口：[README](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/README.md)。声明 Windows/Linux、多集群与多世界、模组配置、备份与快照、玩家名单、日志控制台、更新检测。与本项目已有比较使用同一提交，便于交叉核对。

| 核查维度 | 外部已实现及边界 | 一手证据 |
| --- | --- | --- |
| 长期玩家统计 | 按日期计算活跃用户、登录用户，活跃/登录次数排行、死亡排行、角色使用比例。直接处理采集后的数据，而非只展示游戏日志。指标名不能一律理解为精确在线时长。 | [统计路由与查询实现](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/handler/statistics_handler.go#L54)、[后台采集](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/collect/collect.go) |
| 地图预览 | 生成地图 PNG、按集群/世界获取图像，并提供海象平原判断与存档读取接口。地图图像生成是面板实现。 | [地图处理器](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/handler/dst_map_handler.go#L33)、[渲染服务目录](https://github.com/carrot-hu23/dst-admin-go/tree/d189b9e215f073de2973242f72aacc3bd8747016/internal/service/dstMap) |
| 大厅查询 | 有服务器列表/详情接口。至少一条实现代理 `dst.liuyh.com` 第三方查询服务；不能将所有大厅数据来源称为自研发现协议或官方直连，更不能据此保证公网端口可达。 | [查询路由与第三方请求](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/handler/dst_api_handler.go#L52) |
| 自动化/更新 | 持久化任务并注册 Cron；策略含备份、更新、启停、重启、重新生成世界等，另有公告发送逻辑。 | [任务服务](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/service/schedule/schedule_service.go)、[执行策略](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/service/schedule/strategy.go) |
| 备份外部存储 | 读取备份目录，创建、上传、下载、恢复和清理快照。这里的“上传”是浏览器传入面板，不是面板上传异地对象存储；本次备份服务中未找到 S3/WebDAV 目的地。 | [备份服务](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/service/backup/backup_service.go) |
| 用户/权限、多节点 | 登录服务从单份密码文件取得 `UserInfo`，允许修改账号密码；统一路由装配认证和集群上下文。本次核查未发现多用户表、按房间授权或远程 Agent 调度。多集群指此面板管理的集群，不直接等于多宿主机。 | [登录服务](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/service/login/login_service.go)、[路由装配](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/router.go#L88) |
| 模组方案 | 已证实模组查询、配置与世界级设置，未在本次核查中建立独立可命名、复用整组模组方案的证据。 | [模组处理器](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/handler/mod_handler.go)、[模组服务](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/service/mod/mod_service.go) |

本项目已完成的准入名单、公告/保存/快进、玩家日志和容器内存配额不再重复推荐。地图预览及长期统计仍是可明确辨认的差距。

## 3. sukeme/DstServerHelper：自动维护脚本，而非完整面板

官方入口：[README](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/README.md)。README 明确所有现有维护功能位于 `foralive.py`；作用是维护已经开启的服务器，仍需另用开服工具，只支持 Linux，通过 screen 与游戏交互。

| 核查维度 | 外部已实现及边界 | 一手证据 |
| --- | --- | --- |
| 自动化策略 | 有闲置超时重置、达到指定游戏天数转无尽、游戏/模组更新检查、聊天记录备份、崩溃自启、网络错误重连、多层世界。README 默认值分别包含闲置 24 小时、40 天转无尽、更新 15 分钟。源码开关中闲置重置和转无尽默认关闭，不能把 README 示例理解为默认一定执行。 | [README 功能和限制](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/README.md#L7)、[源码开关](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L30)、[闲置与无尽流程](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L325) |
| 模组更新及失败定位 | 按各世界配置得到工坊 ID，查询远端更新时间，再读本地 ACF 核实更新；将更新成功/失败的模组分组，可记录具体 ID/名称。该证据支持下载更新结果归因，不足以称为任意启动崩溃的根因诊断。私有/未公开 mod 检测按 README 需要 Steam API key。 | [模组更新完整流程](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L642)、[失败 ID 处理](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L795) |
| 崩溃与连接 | `auto_restart` 区分 `crash` 和 `curl_error`，查看日志特征，包括 `LUA ERROR stack traceback`，连接错误路径尝试重连。重启前公告调用游戏命令。 | [崩溃/连接检查](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L904)、[公告与启停函数](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L1026) |
| 备份、权限、多节点、方案 | README 的备份项是聊天记录备份；不能写成完整世界异地备份。配置世界/mod 的解析是另一个网站准备工作，未据此认定维护脚本具备可复用模组方案。该脚本本身没有在本次核查中呈现面板用户/房间权限或远程节点控制架构。 | [README 功能范围与网站进度](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/README.md)、[聊天备份函数](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L541) |

可借鉴点是**维护失败结果细到具体模组**。闲置重置、自动转无尽会改变世界，属于独立运营策略，不作为本项目默认新增需求。

## 4. qinming99/dst-admin：传统单机面板基线

官方入口：[中文 README](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/README-zh.md)。其环境说明为 Ubuntu 与 JDK 1.8，提供 Docker 部署；用于补充传统 Java 面板的实现范围。

| 核查维度 | 外部已实现及边界 | 一手证据 |
| --- | --- | --- |
| 核心运维/玩家 | README 列出地面/洞穴启停、资源监控、房间/世界/mod 设置、运行日志、本地存档上传、管理员和黑名单，以及控制台踢人/回滚/重置。控制台操作是面板封装调用，实际世界行为由 DST 提供。 | [README 功能](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/README-zh.md#L4)、[主页操作控制器](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/src/main/java/com/tugos/dst/admin/controller/HomeController.java) |
| 自动化与版本更新 | README 声明自动备份、无人值守自动更新；配置模型有备份时间、更新时间与 `smartUpdate`，游戏更新命令调用 SteamCMD `app_update 343050 validate`。 | [调度配置](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/src/main/java/com/tugos/dst/admin/vo/ScheduleVO.java)、[更新命令](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/src/main/java/com/tugos/dst/admin/utils/DstConstant.java#L85) |
| 备份外部存储 | 备份服务在主机目录列出备份、计算占用、创建和恢复 ZIP，并支持上传/下载。本次核查未发现异地存储目的地。 | [备份服务](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/src/main/java/com/tugos/dst/admin/service/BackupService.java)、[备份控制器](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/src/main/java/com/tugos/dst/admin/controller/BackupController.java) |
| 用户/权限、多节点、模组预设 | 配置持有一份 `USER_INFO`，有个人信息和密码修改；本次未找到按房间多用户授权、远程 Agent 节点池或独立模组方案实现。README 的“远程控制台”指浏览器下发游戏命令，不能当作跨宿主机控制的证明。 | [单用户配置](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/src/main/java/com/tugos/dst/admin/utils/DstConfigData.java)、[用户控制器](https://github.com/qinming99/dst-admin/blob/0925bdccc28f3ce53b9878752861b58b393950a3/src/main/java/com/tugos/dst/admin/controller/UserController.java) |

这些已证实能力大多已被本项目覆盖，不作为新增优先项。

## 面向 DSTLauncher 的差距候选（产品建议）

以下是依据本项目基线形成的建议，**不是外部功能事实表**。优先级综合当前已有能力和实现范围；不要求照搬上游界面、许可证代码或商业云服务。

| 优先级 | 建议及最小可交付范围 | 外部证据与我方判断 |
| --- | --- | --- |
| 高 | **配置方案与复制房间配置**：把世界参数、各分片模组及配置保存为命名方案；应用后新建房间并重新分配标识/端口。首次只复制配置，是否携带存档单独选择；凭据不随共享方案默认导出。 | DMP 的 [RoomSnapshot](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/room/utils.go#L820) 与 [favoriteApplyPost](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/room/handler.go#L1361) 已证明整套配置可复用。对我方现有逐分片配置，是减少重复操作的直接扩展。 |
| 高 | **启动/更新失败诊断及工坊 ID 归因**：在现有日志上提取失败阶段、世界、原始错误片段与明确出现的工坊 ID，连到模组详情；先覆盖下载失败、Lua 栈明确指向 mod 的情况。 | DstServerHelper 的 [更新后逐 ID 核验](https://github.com/sukeme/DstServerHelper/blob/6bfcb0a5f412124dd021f41e56ccd4dedd676515/foralive.py#L795) 提供具体失败反馈的参照。完整启动诊断是我方建议，不能宣传成四个项目都已有。崩溃重启与退避已存在，不重复开发。 |
| 高 | **事件 Webhook**：先对连续崩溃达到上限、自动维护失败、备份失败发出可测试通知；按房间选择事件并提供签名。 | DMP [事件类型](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/webhook/types.go) 和 [POST/HMAC 实现](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/webhook/sender.go#L121) 是完整证据。价值是面板外获知状态，而非增加已有游戏内维护公告。 |
| 中高 | **异地备份及迁移包**：在本地 ZIP 成功后可复制到一个明确配置的外部目的地；迁移包包含房间/世界/模组配置及恢复清单，提供可核验恢复流程。先支持一个存储协议即可。 | DMP [云存档及配置元数据](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/cloud/archive.go) 证明外部存储与房间导入链路；当前仅本地 ZIP 是我方差距。通用 S3/WebDAV、定时异地副本和“整机迁移”均属于我方设计方向，并非上游已证实的同等功能。 |
| 中 | **地图预览**：先对最新保存点生成地表/洞穴静态图，显示其对应保存点；独立评估存档格式适配成本。 | dst-admin-go [地图 PNG 生成接口](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/handler/dst_map_handler.go#L49) 与 DMP [解析实现](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/dst/map.go) 有直接源码证据。无需把已有天数/季节再包装成新功能。 |
| 中 | **长期玩家统计**：复用已持久化的事件，先提供日期范围内活跃玩家、连接次数、死亡/角色分布；在线时长需明确跨重启、断线与未配对事件的口径。 | dst-admin-go [统计实现](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/handler/statistics_handler.go) 和 DMP [甘特/在线时长接口](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/player/router.go) 支撑。新增的是分析视图，不再建设另一套玩家日志。 |
| 中低 | **大厅可见性检查**：把“本机进程运行”和“大厅能查询到房间”分别展示，注明查询时间和数据来源。 | dst-admin-go [列表/详情代理](https://github.com/carrot-hu23/dst-admin-go/blob/d189b9e215f073de2973242f72aacc3bd8747016/internal/api/handler/dst_api_handler.go) 证明查询入口。自动验证自己房间、与端口连通性诊断组合是我方建议，需要另查当前 Klei/第三方接口契约。 |
| 低，协作需求出现后 | **按房间权限与多节点**：先有独立用户和房间授权，再评估多个已有面板实例的切换/聚合。统一远程 Agent 调度另立目标。 | DMP [用户字段](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/database/models/user.go) 与 [权限判定](https://github.com/miracleEverywhere/dst-management-platform-api/blob/13b414f4925f6edd640cae79ea90c7be2554df92/app/room/utils.go#L88) 有实际实现；[桌面客户端 README](https://github.com/miracleEverywhere/dst-management-platform-desktop/blob/9c86cd09cc6440868fb1b063ecc8b5f910b60fb2/README.md) 仅证明多实例管理。对当前偏单服主的部署方式，优先级低于配置复用、诊断和通知。 |

### 启动与大厅自检的官方规则补充

主会话通过 Exa 读取全文核实的 [Klei Dedicated Server Command Line Options Guide](https://support.klei.com/hc/en-us/articles/360029556192-Dedicated-Server-Command-Line-Options-Guide) 可作为产品规则的官方来源：离线模式不公开列入大厅，LAN 列表发现涉及游戏端口范围 `10998..11018`，多个分片的游戏端口及 Steam 端口需分别唯一。可据此在启动前检查配置冲突，针对离线/LAN 模式解释可见性。配置自检、大厅查询和公网连通性检测应分别展示各自结果。

## 归因与证据使用规则

- Steam 创意工坊的内容、远端版本信息以及 SteamCMD 下载能力属于 Steam；面板的贡献是查询、配置、缓存、安排维护、校验结果与反馈具体失败。
- DST 的 `c_save`、公告、回档、重置、模式切换等属于游戏原生操作；面板的贡献是安全触发、编排、确认、权限和结果展示。
- “多世界/多集群”不自动等于远程多节点，“管理员名单”不等于面板多用户，“上传备份”不自动等于上传外部存储，“mod 配置”不自动等于可复用模组预设。
- 表中的“未发现/未建立证据”均限定于本次 README 与注明的实现路径，不作为对仓库所有历史版本的绝对否定。所有优先项均可回溯到上述固定提交。

本次验证方式：读取官方仓库 API、浅克隆默认分支、检查 README、路由、模型和关键服务函数；DMP Desktop README 通过 GitHub contents API 获取。未部署外部面板、未运行其游戏服务器，也未将检索摘要、合作推广或第三方对比作为功能证据。
