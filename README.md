# DSTWin - 饥荒联机版开服面板（Windows / Linux）

《饥荒联机版》(Don't Starve Together) 专用服务器管理系统。用 Go 原生进程管理（`exec.Command` + stdin 管道）替代 Linux 下的 `screen`/`bash`/`ps`/`tail`：在 Windows 上直接运行，无需 WSL；也提供 Linux 原生进程层与 Docker Compose 部署。

> 本项目的实现思路参考了 [dst-management-platform-api (DMP)](https://github.com/miracleEverywhere/dst-management-platform-api)，感谢 Miracle 的优秀开源作品（MIT License）。DSTWin 并非 DMP 的分支，而是面向 Windows / Linux 平台的独立实现。

## 功能

- **Web 管理面板**：房间管理、启停/重启、控制台命令、实时日志（SSE 流式输出）
- **登录限流**：同一面板累计输错 5 次后锁定 30 秒；冷却结束后继续输错，等待时间逐次翻倍，最高 15 分钟。冷却中返回 HTTP 429 和 `Retry-After`，前端禁用提交并倒计时，刷新不会解除锁定。成功登录后清零；计数保存在进程内，重启面板后清零。
- **多层世界**：一个房间可开任意多个世界（地表 + 洞穴 + 自定义世界），每个世界是独立进程、独立端口，可单独启停
- **模组管理**：在模组页选择分片，分别启用、禁用和配置模组；各分片使用独立的 `modoverrides.lua`，所有房间共享模组文件与版本。通过 `-only_update_server_mods` 多轮预下载，下载清单汇总全部启用分片的模组。
- **模组更新检测**：对比本地记录与创意工坊最新版，直接指出是哪个 mod 有了新版本
- **存档备份与回滚**：手动备份、一键恢复
- **玩家准入名单**：房间编辑页管理黑名单与白名单，白名单联动预留位，支持真实 Klei ID 中的短横线与下划线；修改在重启后生效。
- **快捷保存与公告**：等待运行分片的存档落盘回调，分别展示成功与失败；广播全服公告，并提供需确认的主世界快进。
- **玩家日志**：后台采集各分片的连接与聊天日志，关联 Klei ID、Steam ID、IP 与角色，持久化聊天、进出、死亡、复活及公告历史；支持分页搜索、复制 ID、加入黑名单及删除面板历史。
- **容器内存配额**：Linux 额外检测 cgroup v1/v2 与可见父级内存限额，剩余配额不足时提醒。
- **游戏存档回档**：概览/备份页可选择回退存档次数，主分片发起后全部世界同步重载
- **存档管理**：盘点磁盘上的全部存档目录，标出对应的房间与孤儿存档，可清理
- **进程守护 + 崩溃重启**：`cmd.Wait()` 监听退出自动拉起；停止前自动发送 `c_save()` + `c_shutdown()`，避免丢档
- **房间创建**：世界设置（worldgenoverride 可视化编辑）、管理员名单（adminlist.txt）
- **存档上传**：导入外部存档整包，自动解析新旧两版 ini 结构、转换 leveldataoverride
- **服务器文件安装/更新**：自动探测 Steam 库；首次点击「安装并启动」会通过 SteamCMD 安装 DST 专用服务器（appid 343050，约 4.5GB），成功后继续开服；已有安装不重复下载
- **配置方案**：保存、导入导出房间及各分片世界/模组配置，复制为使用新端口和密钥的新房间。
- **故障诊断**：结合启动失败记录、分片日志和共享模组下载记录，展示错误阶段、日志依据、相关模组及处理建议。
- **事件通知**：按房间和事件发送带可选 HMAC 签名的 Webhook，支持测试发送及最近投递结果。
- **完整迁移**：导出全部房间的存档、配置、名单和配置方案；校验后导入为新房间，重新分配端口，可选择应用维护计划。
- **存档地图预览**：查看最近保存点的真实地形、海岸和地标，支持缩放拖动、地标搜索定位及虫洞配对。
- **异地备份**：新备份后台同步到目录、NAS 或 WebDAV，支持保留份数、连接测试、失败通知和手动重试。
- **恢复检查**：恢复前核验 ZIP、分片世界和玩家存档，预览天数/季节与地图，确认摘要后恢复并保留原存档。

这三项功能的入口、存储位置和接入协议见 [配置方案、故障诊断与事件通知](docs/operations-features.md)。

迁移步骤、地图预览范围及接口见 [完整迁移与存档地图预览](docs/migration-map.md)。

异地目标配置和备份恢复流程见 [异地备份与恢复检查](docs/backup-recovery.md)。

## 环境

**Windows**：Windows 10/11，Go 1.25+，Steam / SteamCMD（服务器文件安装用，已有安装则自动探测）。

**Linux / Docker**：任意支持 Docker 的 Linux x86_64 主机（推荐直接用下文的 Docker Compose）；裸机运行需 Go 1.25+ 与 i386 运行库（`libc6-i386`、`lib32gcc-s1`、`lib32stdc++6`，SteamCMD 是 32 位程序）。详见下文「Linux / Docker 部署」。

## 构建与运行

前端产物通过 `go:embed` 打进二进制，**构建产物只有一个 exe**。改了前端代码后，必须先重新构建前端再编译 Go：

Windows 可在仓库根目录运行一键构建脚本（需要 Node.js/pnpm 和 Go；首次自动安装前端依赖）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\build.ps1
```

生成根目录下的 `dst-windows.exe`。任一步失败会停止构建；脚本不会自动重启面板。

Windows 桌面版使用独立窗口显示同一套管理面板，不显示控制台窗口：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\build.ps1 -Desktop
```

生成根目录下的 `dst-desktop.exe`，双击即可打开。运行需要 Microsoft Edge WebView2 Runtime（Windows 11 通常已自带；缺失时可从 [Microsoft 官网](https://developer.microsoft.com/microsoft-edge/webview2/) 安装 Evergreen Runtime）。无需 C/C++ 编译器。关闭窗口会停止后台任务，并等待游戏服务器保存后退出。桌面版沿用上方命令行参数与数据目录规则；浏览器缓存保存在数据目录的 `webview2/` 下。Web 版与桌面版默认使用相同端口，请先退出正在运行的面板再启动另一个版本。

仅编译桌面后端（要求已构建 `web/dist`）：

```powershell
go generate -tags desktop ./internal/desktopresources
go build -tags desktop -ldflags "-H=windowsgui" -o dst-desktop.exe .
```

桌面构建会将项目图标和 Per-Monitor V2 DPI 清单嵌入 EXE，按 Windows 显示缩放比例渲染，并在跨屏移动时调整窗口。资源生成工具使用固定版本，首次构建时由 Go 下载；生成的 `.syso` 只参与桌面版构建。运行 `powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\check-desktop.ps1` 可检查 EXE 图标、DPI 感知与窗口关闭流程。

手动构建：

```bash
# 前端（修改 web/ 下代码后需要重新构建）
cd web
pnpm install
pnpm run build   # 产物输出到 web/dist，编译时嵌入 exe

# 后端（在仓库根目录，要求 web/dist 已存在）
cd ..
go build -o dst-windows.exe .

# 运行
./dst-windows.exe              # 默认端口 127.0.0.1:16800
./dst-windows.exe -port 8080   # 指定端口
./dst-windows.exe -install     # 仅安装服务器文件后退出
./dst-windows.exe -v           # 显示版本
```

发布 release 只需带上 `dst-windows.exe` 一个文件；`dst_save/`、`logs/`、`panel.json` 首次运行自动生成，SteamCMD 缺失时面板会自动下载。模组目录见下方说明。

### 命令行参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-port` | `16800` | 面板端口 |
| `-bind` | `127.0.0.1` | 监听地址 |
| `-install` | `false` | 仅安装服务器文件后退出 |
| `-serverdir` | 自动探测 | 专用服务器根目录（含 `bin/` 的 "Don't Starve Together Dedicated Server"） |
| `-arch` | `auto` | 服务器架构：auto / 64 / 32 |
| `-data-dir` | 可执行文件同级 | 运行数据根目录（存档/日志/模组/panel.json/SteamCMD）。也可用环境变量 `DST_DATA_DIR`，容器里指向挂载卷 |

服务器目录也可以通过环境变量 `DST_SERVER_DIR` 或 Web 面板里的配置页指定（写入 `panel.json`）。

### 模组目录

- 使用 Steam 库中的服务器时，直接读取并下载到同一库的 `steamapps/workshop/content/322330/<模组 ID>`，开服和模组更新共用这份文件。实际路径显示在「设置 → 服务器文件与架构 → 模组目录」。
- 独立安装或容器安装继续使用数据目录下的 `ugc_mods/`；切换服务器安装位置时自动重新定位。原有 `ugc_mods/` 保留，不自动搬迁或删除。
- 更新或删除共享模组前，请退出使用同一份文件的游戏客户端，并暂停 Steam 的工坊更新，避免同时写入。重新下载失败会恢复旧副本；删除会影响客户端及其它房间。
- 模组页默认选择主分片，也可编辑已停用分片；新分片从空配置开始。房间运行或启停期间配置只读。需要多分片共同安装的模组，请在相关分片分别启用。
- 导入存档保留每个分片各自的模组配置；来源未带配置时保留目标原文件。删除共享模组会移除当前房间所有分片中的该模组记录。

## 玩家管理与快捷运维

- 「房间 → 编辑」可维护管理员、黑名单和白名单。名单始终属于正在编辑的房间；白名单预留位包含在最大人数内，黑名单修改不会立即踢人。
- 「概览 → 快捷操作」可保存所有运行分片、发送公告、确认后让主世界快进一天。保存以游戏写入回调为准，回执超时请先检查日志，避免重复提交。
- 侧边栏「玩家日志」查看历史事件与玩家身份，支持分片/类型筛选和服务端分页搜索。面板后台每 5 秒采集，首次启动补采现有游戏日志；数据保存于 `dst_save/playerlogs/`，面板重启后继续保留。删除仅影响面板历史，原始游戏日志不变；同名账号无法唯一关联时不猜测 ID。

与 `dst-admin-go` 的功能对照、接口和行为边界见 [功能对照与本批改进](docs/dst-admin-go-comparison.md)。

## Linux / Docker 部署

面板已支持 Linux：进程管理层按平台拆分（`internal/server/process_{windows,linux}.go`、`portstat_linux.go`、`procstat_linux.go`），SteamCMD 同样分平台（`internal/steamcmd/platform_{windows,linux}.go`）。Linux 下每个 DST 分片是独立进程组，超时强停会整组回收，避免残留子进程。

用 Docker Compose 单容器部署（一个面板进程管理全部房间/分片）：

```bash
cp .env.example .env            # 按需修改 PANEL_UID/GID、DST_SERVER_DIR
mkdir -p ./data && sudo chown -R 1000:1000 ./data   # 与 PANEL_UID/GID 一致
docker compose up -d --build
docker compose logs -f
```

- **网络**：用 bridge + 显式端口映射（**不用** `network_mode: host`）。
  - Docker Desktop（Windows/macOS）的 host 网络指向 WSL2/虚拟机自己的 loopback，宿主机浏览器访问不到；
  - 面板端口只映射到宿主 `127.0.0.1:16800`，默认不对外暴露；
  - 游戏/协调/Steam 四类端口按范围映射（各 100 个），房间端口由面板动态分配；
  - 超出映射范围的分片玩家会连不上，多开时按同样步长追加范围。
- **访问面板**：浏览器打开 `http://127.0.0.1:16800`。远程服务器用 SSH 隧道 `ssh -L 16800:127.0.0.1:16800 user@host`。当前 API 无鉴权，勿改成对外绑定；需要公网时前置带 TLS+认证的反向代理。
- **首次开服**：在线房间先在房间设置中填写 Klei 令牌，再回概览点击「安装并启动」。面板自动下载 SteamCMD 和服务端，显示安装进度，成功后继续启动所选房间/分片；请保持页面打开。失败会显示原因，可重新点击续传；只下载不开服可用「仅安装服务器」。
- **持久化**：全部可变数据在 `./data`（容器内 `/data`），含存档、模组、日志、`panel.json`、SteamCMD 与面板自带安装的游戏服务端。备份/迁移直接归档 `data/`。
- **优雅关服**：`init: true` 转发 SIGTERM，`stop_grace_period: 2m` 给各分片执行 `c_save()` → `c_shutdown()` 的时间，超时才强制结束进程组。
- **健康检查**：`GET /healthz` 只反映面板进程是否存活，不以「游戏未安装/未开服」为失败。

### 资源要求（实测数据）

DST 专用服务器是内存密集型进程，内存不够会先卡在加载阶段、最终被内核 OOM killer 杀掉。
实测（游戏版本 `756039`，地表 `forest` 世界）：

| 场景 | 实测内存 |
|---|---|
| 单个分片（Master） | 约 0.9～1GB |
| 双分片（Master + Caves） | 需 ≥ 2.5GB |
| 安装后磁盘占用 | 约 4.3GB（首次下载 4.5GB） |

因此：

- **每开一个分片至少预留 1～1.5GB 内存**，另加面板与系统开销。
- 只开地表（关闭洞穴）时 2GB 可跑；地表 + 洞穴建议 **≥ 3GB**；多房间再按分片数递增。
- Docker Desktop / WSL 默认内存上限偏低时，必须先调大（Windows: `%UserProfile%\\.wslconfig` 的 `memory=`），
  否则会在世界加载阶段卡住，最终整个容器被 OOM killer 杀掉（`docker inspect` 可见 `ExitCode 137` / `OOMKilled=true`）。
- 首次生成世界（`WorldSim` 生成地图）比加载已有存档耗时更久、内存峰值更高，宜留出余量。

服务端文件首次由面板用 SteamCMD 匿名安装（`appid 343050`，约 4.5GB）到 `/data/steamapps/common/...`；也可在 `.env` 里用 `DST_SERVER_DIR` 指向已挂载的现有安装。

> 安装中途失败（如 `exit 8` / `App state 0x402`）通常是到 Steam 内容服务器的连接中断。已下载的分块会保留，重新点击安装/更新即可续传。

## 目录结构

```
main.go                 入口：参数解析、目录探测、面板启动
internal/
  server/               进程管理层（Go 原生进程管理）：启停、守护、崩溃重启
  room/                 房间：创建、配置生成、cluster.ini/server.ini/token、管理员名单
  config/               ini 读写、路径计算（-persistent_storage_root 等）
  steamcmd/             SteamCMD 调用、Steam 库探测、服务器安装与更新
  workshop/             创意工坊 mod 下载
  modmgr/               mod 管理：modoverrides.lua、dedicated_server_mods_setup.lua 同步
  saveimport/           外部存档整包导入
  saves/                存档目录盘点与清理（孤儿存档）
  backup/               存档备份与回滚
  worldsettings/        世界设置（worldgenoverride.lua 生成）
  panelcfg/             panel.json 面板配置
  web/                  HTTP API + 静态资源服务
web/                    Vue 3 + Vite 前端
dst_save/               运行时数据：存档根目录（-persistent_storage_root）、rooms.json、playerlogs/ 历史记录
ugc_mods/               独立安装的共享 mod 目录（Steam 安装直接复用库内 workshop）
logs/                   运行日志、mod 更新日志
```

## 游戏存档回档

在「概览 → 房间状态」或「备份」页点击「回档」，选择回退存档次数并确认。
回退 1 次表示恢复到**最新存档之前的一个存档点**；存档通常每天产生，手动保存也会产生存档点，因此次数不一定等于天数。

- 先启动房间全部启用的世界，并等待分片连接完成。未运行、加载中、历史存档不足时会明确提示。
- 操作会丢弃未保存进度及目标之后的存档点，建议先保留整房间备份。
- 回档前不会额外保存。主分片使用游戏原生快照接口选择目标并重新加载，次分片由游戏同步；不直接删除磁盘存档。
- 游戏确认接受后显示「请求已发送」，可在实时日志中确认重载结果；同一房间一分钟内禁止重复提交。
- 此功能使用游戏自动/手动保存的快照，与备份列表中的 ZIP「回滚」相互独立。

接口为 `POST /api/rollback?room=<房间ID>`，JSON 请求体 `{"count":1}`。必须明确指定房间，回退次数为 1–99 的整数；返回 `requested` 仅代表游戏接受请求，实际重载进度见日志。

## 关键实现说明

- **进程管理**：`exec.Command` 启动分片进程（地表/洞穴各一个），按平台隔离进程组（Windows `CREATE_NEW_PROCESS_GROUP`；Linux `Setpgid`）脱离面板生命周期；长期持有 stdin 管道写入控制台命令；`cmd.Wait()` 监听退出实现崩溃检测与自动重启；超时强停时 Windows 杀进程、Linux 杀整个进程组
- **存档路径**：用 `-persistent_storage_root` 指向数据目录下的 `dst_save/`，规避 Windows 默认 `文档\Klei` 路径受 OneDrive 重定向的影响
- **mod 预下载**：启动分片前以 `-only_update_server_mods` 跑一次预更新（引擎约 30 秒无回调即退出、一轮只下 1 个，面板自动多轮循环直至下载完整），分片启动时带 `-skip_update_server_mods` 避免多分片抢写共享目录
- **mods 并集同步**：`dedicated_server_mods_setup.lua` 是安装级文件，面板在 mod/房间增删改时自动重算为所有房间启用 mod 的并集

## 已知注意事项

- 每个世界（分片）都是**独立进程**，端口必须错开；只开地表时洞穴目录留着也不会启动
- 启动参数顺序敏感：`-persistent_storage_root <绝对路径> -conf_dir <单段名> -cluster <单段名> -shard <单段名>`，可执行文件工作目录必须是 `bin/`
- 配置文件（ini/lua）必须 **UTF-8 无 BOM**，否则中文乱码
- 停止服务器务必走面板的"停止"，停止前会自动保存存档

## 交流与反馈

文档不可能覆盖所有情况。第一次在 Windows 上开服，遇到解决不了的问题，可以加 QQ 群 **1131102486** 询问。

## 致谢

- [dst-management-platform-api (DMP)](https://github.com/miracleEverywhere/dst-management-platform-api) —— 本项目的功能设计与实现思路参考来源，作者 [Miracle](https://github.com/miracleEverywhere)，MIT License

## 许可

本项目基于 [MIT License](LICENSE) 开源。
