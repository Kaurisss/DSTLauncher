# DSTWin · 饥荒联机版开服面板（Windows / Linux）

《饥荒联机版》(Don't Starve Together) 专用服务器管理系统。用 Go 原生进程管理（`exec.Command` + stdin 管道）替代 Linux 下的 `screen`/`bash`/`ps`/`tail`：在 Windows 上直接运行，无需 WSL；也提供 Linux 原生进程层与 Docker Compose 部署。

> 本项目的实现思路参考了 [dst-management-platform-api (DMP)](https://github.com/miracleEverywhere/dst-management-platform-api)，感谢 Miracle 的优秀开源作品（MIT License）。DSTWin 并非 DMP 的分支，而是面向 Windows / Linux 平台的独立实现。

## 功能

- **Web 管理面板**：房间管理、启停/重启、控制台命令、实时日志（SSE 流式输出）
- **多层世界**：一个房间可开任意多个世界（地表 + 洞穴 + 自定义世界），每个世界是独立进程、独立端口，可单独启停
- **模组管理**：mod 启用/禁用/配置/删除/更新，通过 `-only_update_server_mods` 多轮预下载，所有房间共享一份 mod 目录
- **模组更新检测**：对比本地记录与创意工坊最新版，直接指出是哪个 mod 有了新版本
- **存档备份与回滚**：手动备份、一键恢复
- **游戏存档回档**：概览/备份页可选择回退存档次数，主分片发起后全部世界同步重载
- **存档管理**：盘点磁盘上的全部存档目录，标出对应的房间与孤儿存档，可清理
- **进程守护 + 崩溃重启**：`cmd.Wait()` 监听退出自动拉起；停止前自动发送 `c_save()` + `c_shutdown()`，避免丢档
- **房间创建**：世界设置（worldgenoverride 可视化编辑）、管理员名单（adminlist.txt）
- **存档上传**：导入外部存档整包，自动解析新旧两版 ini 结构、转换 leveldataoverride
- **服务器文件安装/更新**：自动探测 Steam 库；首次点击「安装并启动」会通过 SteamCMD 安装 DST 专用服务器（appid 343050，约 4.5GB），成功后继续开服；已有安装不重复下载

## 环境

**Windows**：Windows 10/11，Go 1.25+，Steam / SteamCMD（服务器文件安装用，已有安装则自动探测）。

**Linux / Docker**：任意支持 Docker 的 Linux x86_64 主机（推荐直接用下文的 Docker Compose）；裸机运行需 Go 1.25+ 与 i386 运行库（`libc6-i386`、`lib32gcc-s1`、`lib32stdc++6`，SteamCMD 是 32 位程序）。详见下文「Linux / Docker 部署」。

## 构建与运行

前端产物通过 `go:embed` 打进二进制，**构建产物只有一个 exe**。改了前端代码后，必须先重新构建前端再编译 Go：

```bash
# 前端（修改 web/ 下代码后需要重新构建）
cd web
npm install
npm run build   # 产物输出到 web/dist，编译时嵌入 exe

# 后端（在仓库根目录，要求 web/dist 已存在）
go build -o dst-windows.exe .

# 运行
./dst-windows.exe              # 默认端口 127.0.0.1:16800
./dst-windows.exe -port 8080   # 指定端口
./dst-windows.exe -install     # 仅安装服务器文件后退出
./dst-windows.exe -v           # 显示版本
```

发布 release 只需带上 `dst-windows.exe` 一个文件；`dst_save/`、`ugc_mods/`、`logs/`、`panel.json` 首次运行自动生成，SteamCMD 缺失时面板会自动下载。

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
dst_save/               运行时数据：存档根目录（-persistent_storage_root）、rooms.json
ugc_mods/               共享 mod 目录（-ugc_directory）
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
