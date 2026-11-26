# 代码库审计记录

审计日期：2026-11-26
审计分支：`master`
审计对象：`DSTLauncher` 全仓库

## 项目概况

项目是一个用 Go 管理《饥荒联机版》服务器的面板，包含房间、分片、模组、备份、迁移、玩家日志、自动维护、通知和 Vue 前端。

本次审计按单机少量房间、单管理员、长期运行面板的场景评估了后端、前端入口、测试、构建脚本、主要数据流和关键文件操作。

## 本次修复

### 配置输入

- 房间名、描述、密码、令牌、主分片地址和分片通信密钥拒绝换行与空字符。
- 增加对应的房间校验测试。
- 涉及 `internal/room/room.go` 和 `internal/room/room_test.go`。

### 存档导入

- 限制上传 ZIP 文件大小为 8 GiB。
- 限制解压内容大小为 32 GiB。
- 限制 ZIP 条目数量为 100000。
- 垃圾路径也计入 ZIP 条目数量限制。
- 拒绝重复路径、链接、特殊文件和声明大小异常的条目。
- 写入每个条目时按声明大小限流，避免压缩包目录中的大小字段失真导致额外写入。
- 增加重复路径测试。
- 涉及 `internal/saveimport/saveimport.go` 和 `internal/saveimport/parse_test.go`。

### 房间加载与存档上传

- 加载 `rooms.json` 时执行完整房间校验，拒绝绕过接口直接写入的非法内容。
- 存档上传先写入临时集群目录，完整成功后再替换正式目录。
- 上传失败时恢复原房间定义，避免配置和存档只更新一半。
- 临时目录复制存档时优先为未变化的 `save` 文件建立硬链接，减少重复复制大存档。
- 涉及 `internal/room/store.go`、`internal/saveimport/transaction.go` 和 `internal/web/server.go`。

### 日志接口

- 修复 `ReadLog` 返回过多行的问题。
- 请求 N 行现在严格返回最近 N 行。
- 增加回归测试。
- 涉及 `internal/server/logs.go` 和 `internal/server/manager_log_test.go`。

### 文件权限

- 原子写入文件使用 `0600`。
- 房间定义文件使用 `0600`。
- 新建备份 ZIP 使用 `0600`。
- 这会减少令牌、密码、通信密钥和备份内容被同机其他用户读取的风险。
- 涉及 `internal/config/ini.go`、`internal/room/store.go` 和 `internal/backup/backup.go`。

### 前端和文档清理

- 修正 README 中“API 无鉴权”的过时描述。
- 删除确认没有引用的 `web/src/components/ui/UiStat.vue`。
- 删除确认没有引用的 `web/src/components/ui/spinner/`。

### 后端文件拆分

- 将 `internal/web/server.go` 从 约 2400 行拆为 `server.go`、`rooms.go`、`process.go`、`mods.go` 和 `worldconfig.go`，主文件降到 915 行。
- 将 `internal/modmgr/modmgr.go` 从 1642 行拆出 `lua_overrides.go` 和 `modinfo.go`，主文件降到 985 行。
- 将 `internal/server/manager.go` 的日志路径、日志尾随器和日志读取移到 `logs.go`，主文件降到 973 行。
- 所有拆分保持原 package、函数名、路由和数据结构，只移动代码并清理对应导入。
- `go test ./internal/web`、`go test ./internal/modmgr` 和 `go test ./internal/server` 均通过。

## 第二轮修复

### 1. 登录失败计数按客户端隔离

登录失败计数和冷却时间现在按客户端地址保存，反向代理只在请求来自本机回环地址时读取转发的客户端地址。另保留更高阈值的全局保护，避免攻击者用大量地址绕过限制。

### 2. 启动时迁移旧敏感文件权限

启动时会收紧已有的 `panel.json`、`dst_save` 和 `logs` 内文件。Unix 文件使用 `0600`，目录使用 `0700`。Windows 由文件系统 ACL 管理。

### 3. 限制日志和玩家历史增长

单个面板日志文件超过 64 MiB 时安全保留尾部内容。单个分片的玩家历史最多保留 50000 条。游戏源日志由游戏进程持有，采集器只读，不在采集过程中原地截断。

### 4. 缓存地图资源根目录并限制缓存

地图资源根目录缓存 10 秒。贴图缓存最多 256 项，图标缓存最多 512 项。检测到资源根目录变化时清空相关缓存。

### 5. 清理停用或删除分片的旧配置

生成房间配置时会删除非启用分片目录下的 `server.ini`，保留该分片定义、存档和世界配置文件。

### 6. 使用 PBKDF2-SHA256 保存新密码

新密码使用 600000 次 PBKDF2-SHA256。旧版 `salt:hash` 摘要仍可登录，成功登录后自动升级为新格式。

## 剩余问题

### 7. 队列尚未持久化

通知最多保留 320 个待发送事件，异地备份最多保留 40 个待同步任务。超过上限会记录失败或返回队列已满。待处理任务仍只保存在进程内，面板进程重启时会丢失。

如果需要跨重启的投递保证，需要把待处理任务写入磁盘并在启动时恢复。

## 已执行验证

- `go test ./...`
- `go vet ./...`
- `go test -race ./internal/saveimport ./internal/web ./internal/server`
- `pnpm --dir web test`（失败：`check-world-icons.mjs` 扫描到工作区未跟踪的世界图标目录）
- `pnpm --dir web run lint`
- `pnpm --dir web run build`
- `go build -o dst-windows.exe .`
- `git diff --check`

## 未核验范围

- 未启动真实 DST 游戏进程。
- 未执行真实 SteamCMD 下载和更新。
- 未连接真实 WebDAV 目标。
- 未启动 Docker Compose。
- 未使用浏览器完成完整视觉检查。
- 未修改运行时目录中的 `dst_save`、`logs` 和 `ugc_mods` 数据。

## 结论

Go 测试、竞态检查、静态检查、前端 lint 和构建通过。前端纯 Node 检查仍因工作区未跟踪的世界图标素材失败。当前剩余功能问题是通知和异地备份队列没有跨重启持久化。
