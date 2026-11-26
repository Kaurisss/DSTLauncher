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

## 待审查问题

以下问题已确认存在，但本次没有继续修改。

### 1. 登录失败计数是全局的

位置：`internal/web/server.go:72`、`internal/web/auth.go:223`

所有客户端共享同一个失败计数和冷却时间。面板通过反向代理对外开放时，任意攻击者触发 5 次错误登录就能让所有用户进入冷却期。

建议按客户端或代理层进行限流，并保留面板级保护作为第二层限制。

### 2. 旧敏感文件不会自动降权

本次修复只影响下次写入。已有的 `panel.json`、`rooms.json` 和旧备份可能仍然保留原来的 `0644` 权限。

Linux 部署需要增加启动时的权限迁移，或提供一次性安全检查。

### 3. 日志和玩家历史会无限增长

位置：`internal/server/logs.go:26`、`internal/chatlog/history.go:303`

游戏日志持续追加，玩家历史也持续保存。长期运行后，磁盘占用、启动同步时间和历史查询耗时都会增加。

建议增加按天数、文件大小或记录数的保留策略，并在删除前提供明确的设置项。

### 4. 地图资源扫描重复且缓存无上限

位置：`internal/web/map_textures.go:128`、`internal/web/map_textures.go:299`、`internal/web/map_icons.go:281`

地图资源请求会反复扫描游戏和模组目录，贴图和图标缓存持续增长。模组数量较多、反复打开地图时会增加磁盘扫描和内存占用。

建议缓存资源根目录，并为贴图和图标缓存设置大小上限或按安装目录变化清理。

### 5. 停用分片后旧配置文件仍留在磁盘

位置：`internal/room/generate.go:157`

生成配置时只处理启用分片，不清理已经停用或删除的 `server.ini`。旧文件可能在存档导入或手工检查时被误认为仍然有效。

需要先确定停用分片的历史配置是否必须保留，再决定清理策略。

### 6. 通知和异地备份队列会丢任务

位置：`internal/notify/notify.go:97`、`internal/backupremote/remote.go:72`

通知队列容量为 64，异地备份队列容量为 8。队列满时通知会丢弃，异地备份会返回失败并要求手动重试。

如果产品需要投递保证，应增加持久化队列或磁盘待处理记录。

### 7. 密码摘要使用普通迭代 SHA-256

位置：`internal/web/auth.go:24`

当前使用 10000 次 SHA-256。密码文件泄露后，弱密码更容易被离线猜测。

后续可以迁移到 Argon2id 或 bcrypt，并保留旧摘要的兼容读取和登录后升级。

## 已执行验证

- `go test ./...`
- `go vet ./...`
- `go test -race ./internal/saveimport ./internal/web ./internal/server`
- `pnpm --dir web test`
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

核心单机流程和现有自动化检查正常。下一步优先处理远程场景登录限流、旧敏感文件权限迁移和长期运行数据的保留策略。
