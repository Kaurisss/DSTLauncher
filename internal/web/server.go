// Package web 提供 HTTP API 与实时日志推送。
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dst-windows/internal/automation"
	"dst-windows/internal/backup"
	"dst-windows/internal/backupremote"
	"dst-windows/internal/chatlog"
	"dst-windows/internal/config"
	"dst-windows/internal/modmgr"
	"dst-windows/internal/notify"
	"dst-windows/internal/panelcfg"
	"dst-windows/internal/room"
	"dst-windows/internal/saveimport"
	"dst-windows/internal/saves"
	"dst-windows/internal/server"
	"dst-windows/internal/steamcmd"
)

// Options 汇集 Web 层的全部依赖。
// 用结构体而非一长串位置参数，避免以后新增依赖时改乱所有调用点。
type Options struct {
	Mgr      *server.Manager
	Rooms    *room.Store
	Steam    *steamcmd.Manager
	StaticFS fs.FS  // 前端静态资源（嵌入的 web/dist 产物）
	RootDir  string // -persistent_storage_root
	ConfDir  string // -conf_dir
	WorkDir  string // 程序目录，用于读写 panel.json
	UgcDir   string // 共享工坊模组目录（-ugc_directory）
	Version  string // 面板版本号，展示在侧边栏，便于确认「跑的是哪个构建」
	Password string // 启动参数或环境变量指定的面板访问密码（优先于 panel.json）
}

// Server 汇集所有模块的处理器，向前端暴露统一 API。
//
// 模组与备份管理器不预先构造：它们的目录随房间变化，
// 每次请求按 room 参数现算（构造本身只是拼路径，开销可忽略）。
type Server struct {
	mgr            *server.Manager
	rooms          *room.Store
	steam          *steamcmd.Manager
	staticFS       fs.FS
	rootDir        string
	confDir        string
	workDir        string
	version        string
	envPassword    string
	authSecret     []byte
	authMu         sync.Mutex
	authConfigSeen bool
	loginMu        sync.Mutex
	failCount      int
	lockUntil      time.Time
	loginDelay     time.Duration

	// taskMu 串行化「开服 / 安装更新服务端 / 模组下载」这四类会改动共享状态
	// （游戏安装目录、共享 ugc 目录）的操作，让「检查是否忙 → 启动」成为原子动作，
	// 关闭并发请求同时穿过检查的窗口。长任务本身是异步的，这里只锁决策与启动瞬间。
	taskMu          sync.Mutex
	maintenanceMu   sync.RWMutex
	automation      *automation.Engine
	automationErr   error
	playerLogs      *chatlog.History
	profileMu       sync.Mutex
	notifications   *notify.Engine
	notificationErr error
	migrations      *migrationJobs
	mapMu           sync.Mutex
	mapTexMu        sync.RWMutex
	mapTexCache     map[string][]byte
	mapIconMu       sync.Mutex
	mapIconCache    map[string][]byte
	mapIconIdxKey   string
	mapIconIdx      map[string]resolvedMapIcon
	backupMu        sync.Mutex
	backupRemote    *backupremote.Engine
	backupRemoteErr error
	backupChecks    *backupChecks
	publicIPMu      sync.Mutex
	publicIPCache   publicIPState
	publicIPFetcher func(context.Context) publicIPState
}

// NewServer 创建 HTTP 服务。
func NewServer(o Options) *Server {
	if o.Mgr != nil && o.UgcDir != "" {
		o.Mgr.SetUgcDir(o.UgcDir)
	}
	s := &Server{
		mgr:            o.Mgr,
		rooms:          o.Rooms,
		steam:          o.Steam,
		staticFS:       o.StaticFS,
		rootDir:        o.RootDir,
		confDir:        o.ConfDir,
		workDir:        o.WorkDir,
		version:        o.Version,
		envPassword:    strings.TrimSpace(o.Password),
		authSecret:     generateSecret(),
		authConfigSeen: o.WorkDir != "" && panelcfg.Exists(o.WorkDir),
		playerLogs:     chatlog.NewHistory(filepath.Join(o.RootDir, "playerlogs")),
	}
	s.automation, s.automationErr = automation.New(filepath.Join(o.RootDir, "automation.json"))
	s.notifications, s.notificationErr = notify.New(filepath.Join(o.RootDir, "notifications.json"))
	s.migrations = newMigrationJobs()
	s.backupChecks = newBackupChecks()
	s.backupRemote, s.backupRemoteErr = backupremote.New(filepath.Join(o.RootDir, "backup-remote.json"), o.RootDir, func(result backupremote.Result) {
		s.publishNotification("backup_failed", result.Room, "", "本地备份已保留，异地同步失败: "+result.Name+"；"+result.Error)
	})
	return s
}

// respond 统一响应格式。
func respond(w http.ResponseWriter, code int, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 500, "message": err.Error(), "data": nil,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code": code, "message": "success", "data": data,
	})
}

// Handler 构建路由。
func (s *Server) Handler() http.Handler {
	apiMux := http.NewServeMux()

	// 面板登录与密码配置
	apiMux.HandleFunc("/api/auth/status", s.handleAuthStatus)
	apiMux.HandleFunc("/api/auth/login", s.handleAuthLogin)
	apiMux.HandleFunc("/api/auth/logout", s.handleAuthLogout)
	apiMux.HandleFunc("/api/auth/password", s.handleAuthPassword)

	// 房间（配置的唯一事实来源）
	apiMux.HandleFunc("/api/rooms", s.handleRooms)
	apiMux.HandleFunc("PUT /api/rooms/connect", s.handleRoomConnect)
	apiMux.HandleFunc("/api/rooms/meta", s.handleRoomMeta)
	apiMux.HandleFunc("POST /api/rooms/copy", s.handleProfileApply)
	apiMux.HandleFunc("GET /api/network/public-ip", s.handlePublicIP)
	apiMux.HandleFunc("GET /api/profiles", s.handleProfiles)
	apiMux.HandleFunc("POST /api/profiles", s.handleProfiles)
	apiMux.HandleFunc("DELETE /api/profiles", s.handleProfiles)
	apiMux.HandleFunc("POST /api/migration/export", s.handleMigrationExport)
	apiMux.HandleFunc("POST /api/migration/upload", s.handleMigrationUpload)
	apiMux.HandleFunc("POST /api/migration/apply", s.handleMigrationApply)
	apiMux.HandleFunc("GET /api/migration/job", s.handleMigrationJob)
	apiMux.HandleFunc("DELETE /api/migration/job", s.handleMigrationJob)
	apiMux.HandleFunc("GET /api/migration/download", s.handleMigrationDownload)
	apiMux.HandleFunc("GET /api/map", s.handleMap)
	apiMux.HandleFunc("GET /api/map/texture", s.handleMapTexture)
	apiMux.HandleFunc("GET /api/map/icon", s.handleMapIcon)
	apiMux.HandleFunc("POST /api/profiles/import", s.handleProfileImport)
	apiMux.HandleFunc("POST /api/profiles/apply", s.handleProfileApply)
	apiMux.HandleFunc("GET /api/diagnostics", s.handleDiagnostics)
	apiMux.HandleFunc("GET /api/notifications", s.handleNotifications)
	apiMux.HandleFunc("PUT /api/notifications", s.handleNotifications)
	apiMux.HandleFunc("POST /api/notifications/test", s.handleNotificationTest)

	// 房间管理员（adminlist.txt）
	apiMux.HandleFunc("/api/admins", s.handleAdmins)
	apiMux.HandleFunc("GET /api/player-lists", s.handlePlayerLists)
	apiMux.HandleFunc("POST /api/player-lists", s.handlePlayerLists)
	apiMux.HandleFunc("DELETE /api/player-lists", s.handlePlayerLists)

	// 在线玩家与结构化管理操作
	apiMux.HandleFunc("GET /api/players", s.handlePlayers)
	apiMux.HandleFunc("POST /api/players/action", s.handlePlayerAction)

	// 宿主机资源（独立于房间和分片进程）
	apiMux.HandleFunc("GET /api/host", s.handleHostStats)
	apiMux.HandleFunc("GET /api/automation", s.handleAutomation)
	apiMux.HandleFunc("PUT /api/automation", s.handleAutomation)
	apiMux.HandleFunc("GET /api/worldstate", s.handleWorldState)

	// 进程控制
	apiMux.HandleFunc("/api/status", s.handleStatus)
	apiMux.HandleFunc("/api/start", s.handleStart)
	apiMux.HandleFunc("/api/stop", s.handleStop)
	apiMux.HandleFunc("/api/restart", s.handleRestart)
	apiMux.HandleFunc("POST /api/rollback", s.handleRollback)
	apiMux.HandleFunc("/api/cmd", s.handleCmd)
	apiMux.HandleFunc("POST /api/quick-action", s.handleQuickAction)

	// 日志
	apiMux.HandleFunc("/api/logs", s.handleLogs)
	apiMux.HandleFunc("/api/logs/stream", s.handleLogStream)
	apiMux.HandleFunc("GET /api/chat-log", s.handleChatLog)
	apiMux.HandleFunc("DELETE /api/chat-log", s.handleChatLog)

	// 模组
	apiMux.HandleFunc("/api/mods", s.handleMods)
	apiMux.HandleFunc("GET /api/mods/icon", s.handleModIcon)
	apiMux.HandleFunc("/api/mods/toggle", s.handleModToggle)
	apiMux.HandleFunc("/api/mods/config", s.handleModConfig)
	apiMux.HandleFunc("/api/mods/schema", s.handleModSchema)
	apiMux.HandleFunc("/api/mods/delete", s.handleModDelete)
	apiMux.HandleFunc("/api/mods/update", s.handleModsUpdate)
	apiMux.HandleFunc("/api/mods/updates", s.handleModUpdates)

	// 创意工坊（搜索 / 详情 / API Key 设置）
	apiMux.HandleFunc("/api/workshop/details", s.handleWorkshopDetails)
	apiMux.HandleFunc("/api/workshop/search", s.handleWorkshopSearch)
	apiMux.HandleFunc("/api/workshop/key", s.handleWorkshopKey)

	// 世界配置（worldgenoverride.lua）
	apiMux.HandleFunc("/api/worldconfig", s.handleWorldConfig)

	// 备份
	apiMux.HandleFunc("/api/backups", s.handleBackups)
	apiMux.HandleFunc("/api/backups/create", s.handleBackupCreate)
	apiMux.HandleFunc("GET /api/backups/check", s.handleBackupCheck)
	apiMux.HandleFunc("POST /api/backups/check", s.handleBackupCheck)
	apiMux.HandleFunc("DELETE /api/backups/check", s.handleBackupCheck)
	apiMux.HandleFunc("GET /api/backups/check/map", s.handleBackupCheckMap)
	apiMux.HandleFunc("GET /api/backup-remote", s.handleBackupRemote)
	apiMux.HandleFunc("PUT /api/backup-remote", s.handleBackupRemote)
	apiMux.HandleFunc("POST /api/backup-remote/test", s.handleBackupRemoteTest)
	apiMux.HandleFunc("POST /api/backup-remote/sync", s.handleBackupRemoteSync)
	apiMux.HandleFunc("/api/backups/delete", s.handleBackupDelete)
	apiMux.HandleFunc("GET /api/backups/restore", s.handleBackupRestore)
	apiMux.HandleFunc("POST /api/backups/restore", s.handleCheckedRestore)
	apiMux.HandleFunc("/api/backups/download", s.handleBackupDownload)

	// 存档上传（外部存档整包 -> 房间）
	apiMux.HandleFunc("/api/saves/upload", s.handleSaveUpload)

	// 存档管理（磁盘上的集群目录盘点 / 清理）
	apiMux.HandleFunc("/api/saves", s.handleSaveArchives)

	// 生成结果预览
	apiMux.HandleFunc("/api/config", s.handleConfig)

	// SteamCMD 安装
	apiMux.HandleFunc("/api/install", s.handleInstall)
	apiMux.HandleFunc("/api/install/update", s.handleUpdate)

	// 服务器文件位置（可在面板里切换安装目录与架构）
	apiMux.HandleFunc("/api/server", s.handleServer)

	mux := http.NewServeMux()
	// 健康检查：供容器编排/反向代理探活。只要面板进程在跑、路由能响应就算健康，
	// 不以「游戏是否已安装/是否有分片在跑」为失败条件——那会让未安装时容器被误判为不健康。
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.Handle("/api/", apiMux)

	// 静态资源。index.html 必须禁缓存：前端产物带 hash 文件名，
	// 浏览器缓存了旧 index.html 就会继续加载旧 bundle，
	// 表现为「后端明明更新了，页面行为还是老的」
	mux.Handle("/", noCacheHTML(http.FileServer(http.FS(s.staticFS))))

	return s.withAuth(s.guardMaintenance(mux))
}

// noCacheHTML 长期缓存带内容哈希的构建资源，HTML 每次校验，固定路径图标缓存一天。
func noCacheHTML(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cache string
		switch {
		case strings.HasSuffix(r.URL.Path, "/"), strings.HasSuffix(r.URL.Path, ".html"):
			cache = "no-cache, must-revalidate"
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			cache = "public, max-age=31536000, immutable"
		default:
			cache = "public, max-age=86400"
		}
		w.Header().Set("Cache-Control", cache)
		next.ServeHTTP(w, r)
	})
}

// query 读取查询参数。
func query(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

// ---- 备份 ----

func (s *Server) bakFor(rm *room.Room) *backup.Manager {
	return backup.NewManager(s.rootDir, s.confDir, rm.ID)
}

func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	items, err := s.bakFor(rm).List()
	respond(w, 200, items, err)
}

// handleBackupCreate 备份整个集群目录。
// 备份前对所有运行中的分片发 c_save() 并等待落盘，否则会拷到半截存档。
func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	// 有分片在运行时会先请求落盘并等待，无运行分片则立即返回
	if err = s.saveRoom(r.Context(), rm); err != nil {
		s.publishNotification("backup_failed", rm.ID, "", "请求保存失败，未创建备份: "+err.Error())
		respond(w, 200, nil, err)
		return
	}
	item, err := s.bakFor(rm).Create()
	if err != nil {
		s.publishNotification("backup_failed", rm.ID, "", "备份失败: "+err.Error())
	}
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	queued, syncErr := s.queueRemoteBackup(rm.ID, item)
	message := ""
	if syncErr != nil {
		message = syncErr.Error()
		s.publishNotification("backup_failed", rm.ID, "", "本地备份已完成，异地同步未排队: "+message)
	}
	respond(w, 200, struct {
		*backup.BackupItem
		SyncQueued bool   `json:"syncQueued"`
		SyncError  string `json:"syncError,omitempty"`
	}{item, queued != nil, message}, nil)
}

func (s *Server) handleBackupDelete(w http.ResponseWriter, r *http.Request) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	respond(w, 200, nil, s.bakFor(rm).Delete(query(r, "name")))
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	resume := s.mgr.PauseRecovery()
	defer resume()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	// 回滚前必须停服，否则运行中的服务器会用内存状态覆盖恢复的存档，
	// 且 Windows 上被占用的文件会导致覆盖失败。
	if s.mgr.AnyRunning(rm.Key()) {
		respond(w, 200, nil, fmt.Errorf("请先停止服务器再回滚存档"))
		return
	}
	respond(w, 200, nil, s.bakFor(rm).Restore(query(r, "name")))
}

// handleBackupDownload 把备份 zip 原样下发给浏览器。
// 用 http.ServeFile 而不是 respond()：这里要的是文件流，
// Content-Disposition 里的文件名取自备份名（Cluster_N_时间戳.zip，纯 ASCII，无需编码）。
func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	name := query(r, "name")
	if name != filepath.Base(name) || !strings.HasSuffix(name, ".zip") {
		respond(w, 200, nil, fmt.Errorf("非法的备份文件名"))
		return
	}
	path := filepath.Join(s.rootDir, "backups", fmt.Sprintf("Cluster_%d", rm.ID), name)
	if _, err := os.Stat(path); err != nil {
		respond(w, 200, nil, fmt.Errorf("备份不存在: %s", name))
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, path)
}

// ---- 存档上传 ----

// maxUploadBytes 是上传压缩包的大小上限。
// DST 存档长期运行后可能到 GB 级（地表 + 洞穴），给足余量；
// 上限只是为了挡住误传的超大文件把磁盘写满，不是功能限制。
const maxUploadBytes = 8 << 30

// saveImportSummary 是上传成功后回给前端的导入摘要，
// 用于在界面上说明「这次覆盖了哪些内容」，而不是只弹一句成功。
type saveImportSummary struct {
	NewRoom     bool     `json:"newRoom"`
	RoomID      int      `json:"roomId"`
	RoomName    string   `json:"roomName"`
	Shards      []string `json:"shards"`
	HasSave     bool     `json:"hasSave"`
	TokenSource string   `json:"tokenSource"`
	WorldFiles  []string `json:"worldFiles"`
	ListFiles   []string `json:"listFiles"`
	// ModsTotal 是存档里启用的模组数量；ModsMissing 是本地尚未下载的。
	// 缺失的模组需要用户到「模组」页点「更新模组」补齐后才能启动。
	ModsTotal   int      `json:"modsTotal"`
	ModsMissing []string `json:"modsMissing"`
	Warnings    []string `json:"warnings"`
	Notes       []string `json:"notes"`
}

// ---- 存档管理 ----

// runningClusters 汇总「有进程在跑」的集群标识。
func (s *Server) runningClusters() map[string]bool {
	out := make(map[string]bool)
	for _, st := range s.mgr.Status("") {
		switch st.State {
		case "running", "starting", "stopping":
			out[st.Cluster] = true
		}
	}
	return out
}

// handleSaveArchives 盘点 / 清理磁盘上的集群存档目录。
//
// 删除房间只删定义，存档目录会留下——这里给这些目录一个出口。
// 删除是物理删除、不可恢复，因此只允许删「已经没有房间认领」的目录：
// 房间还在却想删存档属于误操作高发场景，必须先删房间再来。
func (s *Server) handleSaveArchives(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := saves.Scan(s.rootDir, s.confDir, s.rooms.List(), s.runningClusters())
		// 带上根目录绝对路径：孤儿存档删了就没了，界面要告诉用户
		// 想保留的话该去哪儿手动拷走。
		respond(w, 200, map[string]interface{}{
			"root":  config.ClusterRoot(s.rootDir, s.confDir),
			"items": list,
		}, err)

	case http.MethodDelete:
		cluster := strings.TrimSpace(query(r, "cluster"))
		if cluster == "" {
			respond(w, 200, nil, fmt.Errorf("缺少存档目录名"))
			return
		}
		id, ok := saves.ParseClusterID(cluster)
		if !ok {
			respond(w, 200, nil, fmt.Errorf("非法的存档目录名: %s", cluster))
			return
		}
		// 房间还在：先删房间。运行中的房间一定属于这种情况，也会被这条拦住。
		for _, rm := range s.rooms.List() {
			if rm.ID == id {
				respond(w, 200, nil, fmt.Errorf(
					"「%s」仍是面板里的房间，请先在房间页删除房间，存档才会变成可清理的孤儿存档", rm.Name))
				return
			}
		}
		if s.runningClusters()[cluster] {
			respond(w, 200, nil, fmt.Errorf("该存档仍被运行中的进程占用，请先停止服务器"))
			return
		}
		if err := saves.Delete(s.rootDir, s.confDir, cluster); err != nil {
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, map[string]interface{}{"cluster": cluster}, nil)

	default:
		respond(w, 200, nil, fmt.Errorf("不支持的方法: %s", r.Method))
	}
}

// handleSaveUpload 处理外部存档整包的上传。
//
// 语义：存档「占用」目标房间——存档里的可覆盖玩法配置写回房间，
// 世界数据拷进房间目录，随后由面板重新生成 ini/token。
// mode=new 时用存档新建房间，mode=overwrite 时覆盖 room 参数指定的房间。
//
// 端口、集群密钥、master_ip 一律保留目标房间的（新建则自动分配），
// 理由见 saveimport.MergeInto。
func (s *Server) handleSaveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, 200, nil, fmt.Errorf("上传存档请使用 POST 请求"))
		return
	}

	// 先卡住请求体大小：超大文件不该先落满磁盘再报错
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respond(w, 200, nil, fmt.Errorf("解析上传内容失败（文件可能过大，或未选择文件）: %w", err))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		respond(w, 200, nil, fmt.Errorf("没有收到存档文件: %w", err))
		return
	}
	defer file.Close()

	// 定位目标房间
	var target *room.Room
	if r.FormValue("mode") == "overwrite" {
		id, convErr := strconv.Atoi(r.FormValue("room"))
		if convErr != nil || id <= 0 {
			respond(w, 200, nil, fmt.Errorf("请选择要覆盖的房间"))
			return
		}
		got, ok := s.rooms.Get(id)
		if !ok {
			respond(w, 200, nil, fmt.Errorf("房间不存在: %d", id))
			return
		}
		// 运行中的服务器会占住存档文件（Windows 下覆盖直接失败），
		// 而且退服时会用内存状态把新存档盖回去，所以必须先停服
		if s.mgr.AnyRunning(got.Key()) {
			respond(w, 200, nil, fmt.Errorf("房间「%s」正在运行，请先停止再上传存档", got.Name))
			return
		}
		target = got
	}

	work, err := saveimport.NewWorkDir(s.rootDir)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	defer func() { _ = os.RemoveAll(work) }()

	// 上传的文件名不可信，统一存成固定名字
	zipPath := filepath.Join(work, "upload.zip")
	if err := saveUploadedFile(file, zipPath); err != nil {
		respond(w, 200, nil, err)
		return
	}

	extractDir := filepath.Join(work, "extract")
	if err := saveimport.Extract(zipPath, extractDir); err != nil {
		respond(w, 200, nil, err)
		return
	}
	clusterDir, err := saveimport.FindClusterDir(extractDir)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	parsed, err := saveimport.Parse(clusterDir)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}

	// 先落房间定义取得 ID，再把文件写到临时集群目录。
	merged := saveimport.MergeInto(parsed, target)
	var saved *room.Room
	if target == nil {
		saved, err = s.rooms.Create(merged)
	} else {
		saved, err = s.rooms.Update(merged)
	}
	if err != nil {
		respond(w, 200, nil, err)
		return
	}

	rollbackRoom := func(cause error) error {
		var rollbackErr error
		if target == nil {
			rollbackErr = s.rooms.Delete(saved.ID)
		} else {
			_, rollbackErr = s.rooms.Update(target)
		}
		if rollbackErr != nil {
			return fmt.Errorf("%w；回滚房间定义失败: %v", cause, rollbackErr)
		}
		return cause
	}

	stageRoot := filepath.Join(work, "staging")
	stageCluster := config.ClusterDir(stageRoot, s.confDir, saved.ID)
	targetCluster := saved.ClusterDir(s.rootDir, s.confDir)
	if _, statErr := os.Stat(targetCluster); statErr == nil {
		if err := saveimport.CloneTree(targetCluster, stageCluster); err != nil {
			respond(w, 200, nil, rollbackRoom(fmt.Errorf("准备存档临时目录失败: %w", err)))
			return
		}
	} else if !os.IsNotExist(statErr) {
		respond(w, 200, nil, rollbackRoom(fmt.Errorf("检查目标存档目录失败: %w", statErr)))
		return
	}

	notes, err := parsed.WriteInto(stageRoot, s.confDir, saved)
	if err != nil {
		respond(w, 200, nil, rollbackRoom(fmt.Errorf("写入存档临时目录失败: %w", err)))
		return
	}
	// ini / token 统一由面板按房间配置生成，不照抄存档里的那份
	if err := room.Generate(stageRoot, s.confDir, saved); err != nil {
		respond(w, 200, nil, rollbackRoom(fmt.Errorf("生成临时配置文件失败: %w", err)))
		return
	}

	var modsMissing []string
	if parsed.HasMods {
		modsMissing, err = modmgr.NewManager(
			stageCluster,
			s.steam.GameDir(),
			s.mgr.UgcDir(),
			saved.ShardNames(),
		).MissingMods()
		if err != nil {
			respond(w, 200, nil, rollbackRoom(fmt.Errorf("读取导入模组失败: %w", err)))
			return
		}
	}

	backupCluster := filepath.Join(work, "old-cluster")
	if err := saveimport.CommitTree(stageCluster, targetCluster, backupCluster); err != nil {
		respond(w, 200, nil, rollbackRoom(err))
		return
	}

	// 下载清单是安装级的，模组或房间变化后必须重算并集
	s.syncModsSetup()

	s.mgr.Register(server.NewCluster(saved))

	// 汇总这次导入带了哪些模组、缺哪些，界面上直接告诉用户下一步该干嘛
	respond(w, 200, map[string]interface{}{
		"room":    saved,
		"summary": buildImportSummary(parsed, saved, notes, target == nil, modsMissing),
	}, nil)
}

// saveUploadedFile 把上传的文件写到 dst。
func saveUploadedFile(src io.Reader, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("保存上传文件失败: %w", err)
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return fmt.Errorf("保存上传文件失败: %w", err)
	}
	return out.Close()
}

// buildImportSummary 汇总这次导入实际覆盖了什么。
func buildImportSummary(p *saveimport.Parsed, saved *room.Room, notes []string, isNew bool, modsMissing []string) saveImportSummary {
	sum := saveImportSummary{
		NewRoom:     isNew,
		RoomID:      saved.ID,
		RoomName:    saved.Name,
		HasSave:     p.HasSave,
		Warnings:    p.Warnings,
		Notes:       notes,
		ModsMissing: modsMissing,
	}
	modIDs := map[string]bool{}
	for _, sh := range p.Shards {
		if target, ok := saved.Shard(sh.Name); ok && target.Enabled() && sh.Mods != nil {
			for _, id := range sh.Mods.EnabledWorkshopIDs() {
				modIDs[id] = true
			}
		}
		sum.Shards = append(sum.Shards, sh.Name)
		if sh.HasOverride {
			sum.WorldFiles = append(sum.WorldFiles, fmt.Sprintf(
				"%s ← %s（%d 项）", sh.Name, sh.OverrideSrc, len(sh.Override.Overrides)))
		}
	}
	sum.ModsTotal = len(modIDs)
	switch {
	case strings.TrimSpace(p.Token) != "":
		sum.TokenSource = "存档自带"
	case saved.HasToken():
		sum.TokenSource = "保留房间原有令牌"
	default:
		sum.TokenSource = "无（在线模式需自行填写）"
	}
	if p.HasAdmin {
		sum.ListFiles = append(sum.ListFiles, "adminlist.txt")
	}
	if p.HasBlock {
		sum.ListFiles = append(sum.ListFiles, "blocklist.txt")
	}
	if p.HasWhite {
		sum.ListFiles = append(sum.ListFiles, "whitelist.txt")
	}
	return sum
}

// ---- SteamCMD 安装 ----

// handleInstall 查询安装状态，或触发首次安装。
//
// 注意：复用 Steam 里已安装的专用服务器是**正常状态**，不是错误。
// 因此这里照常返回状态，另外把来源信息一并带上，
// 让前端能如实说明「服务器来自哪里」以及「更新该走 Steam」。
func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	// 取消/暂停正在进行的安装或更新（保留已下载的分块，后续可续传）
	if r.Method == http.MethodDelete || action == "cancel" || action == "stop" {
		if err := s.steam.Cancel(); err != nil {
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, map[string]interface{}{"canceled": true}, nil)
		return
	}
	// 显式触发安装
	if action == "start" {
		s.taskMu.Lock()
		defer s.taskMu.Unlock()
		if s.steam.IsExternal() {
			respond(w, 200, nil, fmt.Errorf(
				"当前已复用本机已安装的服务器（%s），无需在此安装；如需更新请在 Steam 中更新对应的游戏或专用服务器工具",
				s.steam.Root()))
			return
		}
		if s.steam.IsInstalling() {
			respond(w, 200, nil, fmt.Errorf("安装正在进行中"))
			return
		}
		if err := s.busyTaskError("install"); err != nil {
			respond(w, 200, nil, err)
			return
		}
		if err := s.requireNoRunningShards(); err != nil {
			respond(w, 200, nil, err)
			return
		}
		s.steam.Install()
		respond(w, 200, map[string]interface{}{"started": true}, nil)
		return
	}

	progress, errStr, output := s.steam.Progress()
	// errStr 为 "<nil>" 时视为无错误，避免前端拿到无意义字符串
	if errStr == "<nil>" {
		errStr = ""
	}
	respond(w, 200, map[string]interface{}{
		"installed":  s.steam.IsInstalled(),
		"installing": s.steam.IsInstalling(),
		"canceled":   s.steam.IsCanceled(),
		"progress":   progress,
		"error":      errStr,
		"output":     output,
		"version":    s.steam.Version(),
		// 服务器来源
		"external":   s.steam.IsExternal(),
		"root":       s.steam.Root(),
		"executable": s.steam.Executable(),
		"source":     s.steam.Source(),
		"arch":       s.steam.Arch(),
	}, nil)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.steam.IsExternal() {
		respond(w, 200, nil, fmt.Errorf(
			"服务器复用自 %s，请在 Steam 中更新对应的「Don't Starve Together」游戏本体或专用服务器工具",
			s.steam.Root()))
		return
	}
	if !s.steam.IsInstalled() {
		respond(w, 200, nil, fmt.Errorf("服务器尚未安装"))
		return
	}
	if err := s.busyTaskError("install"); err != nil {
		respond(w, 200, nil, err)
		return
	}
	if err := s.requireNoRunningShards(); err != nil {
		respond(w, 200, nil, err)
		return
	}
	err := s.steam.Update()
	respond(w, 200, map[string]interface{}{"started": err == nil}, err)
}

// busyTaskError 实现「开服 / 安装更新服务端 / 模组下载」之间的全局互斥。
// 这三类操作都会改动游戏安装目录或共享 ugc 目录，必须互斥。
// skip 指明调用方自己属于哪类任务（"install" / "mods"），避免自己挡自己。
func (s *Server) busyTaskError(skip string) error {
	if skip != "install" && s.steam.IsInstalling() {
		return fmt.Errorf("服务器文件正在安装或更新，请等待完成后再操作")
	}
	if skip != "mods" && s.mgr.ModsUpdating() {
		return fmt.Errorf("模组正在下载，请等待完成后再操作")
	}
	return nil
}

// requireNoRunningShards 在安装 / 更新服务器文件前确认没有分片在运行。
//
// 正在运行的服务器进程会持有游戏文件，SteamCMD 替换这些文件可能失败，
// 最坏情况是留下一个「更新了一半」的安装——那种状态很容易在下次启动时崩溃。
func (s *Server) requireNoRunningShards() error {
	busy := s.mgr.RunningShards()
	if len(busy) == 0 {
		return nil
	}
	return fmt.Errorf(
		"有分片正在运行（%s），请先全部停止再安装或更新："+
			"运行中的服务器会占用游戏文件，更新可能失败或只更新一半",
		strings.Join(busy, "、"))
}

// ---- 服务器文件位置 ----

// serverStatus 是 /api/server 的返回结构。
type serverStatus struct {
	Root        string              `json:"root"`
	UgcDir      string              `json:"ugcDir"`
	Source      string              `json:"source"`
	Arch        string              `json:"arch"`     // 实际使用中的架构
	ArchPref    string              `json:"archPref"` // 用户设置的偏好
	External    bool                `json:"external"`
	Pinned      bool                `json:"pinned"` // 路径是显式指定的，不是自动探测
	Installed   bool                `json:"installed"`
	Installing  bool                `json:"installing"`
	Version     string              `json:"version"`
	Executable  string              `json:"executable"`
	ConfigPath  string              `json:"configPath"`
	Locked      bool                `json:"locked"` // 有分片在运行，此时不允许改
	ArchOptions []string            `json:"archOptions"`
	Candidates  []steamcmd.Detected `json:"candidates"`
}

func (s *Server) serverStatus() serverStatus {
	return serverStatus{
		Root:        s.steam.Root(),
		UgcDir:      s.mgr.UgcDir(),
		Source:      s.steam.Source(),
		Arch:        s.steam.Arch(),
		ArchPref:    s.steam.ArchPref(),
		External:    s.steam.IsExternal(),
		Pinned:      s.steam.Pinned(),
		Installed:   s.steam.IsInstalled(),
		Installing:  s.steam.IsInstalling(),
		Version:     s.steam.Version(),
		Executable:  s.steam.Executable(),
		ConfigPath:  panelcfg.Path(s.workDir),
		Locked:      s.anyRunning() || s.steam.IsInstalling() || s.mgr.ModsUpdating(),
		ArchOptions: []string{steamcmd.ArchAuto, steamcmd.Arch64, steamcmd.Arch32},
		// 每次查询都重新探测，用户装完 Steam 版工具后刷新页面就能看到
		Candidates: steamcmd.DetectInstalled(),
	}
}

// anyRunning 判断是否还有分片在跑。
// 直接过一遍进程层的全部记录，不依赖集群是否已注册。
func (s *Server) anyRunning() bool {
	for _, st := range s.mgr.Status("") {
		switch st.State {
		case "running", "starting", "stopping":
			return true
		}
	}
	return false
}

// handleServer 查询 / 修改「用哪一份专用服务器安装」。
//
// 运行中不允许改：正在跑的分片用的是旧目录下的可执行文件，
// 中途换掉会让「运行中的进程」与「面板认为的安装位置」对不上。
func (s *Server) handleServer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, s.serverStatus(), nil)

	case http.MethodPut:
		s.taskMu.Lock()
		defer s.taskMu.Unlock()
		var req struct {
			ServerDir string `json:"serverDir"`
			Arch      string `json:"arch"`
		}
		if err := decodeJSON(w, r, &req, 4096); err != nil {
			respond(w, 200, nil, err)
			return
		}
		if s.anyRunning() {
			respond(w, 200, nil, fmt.Errorf("还有分片在运行，请先全部停止再更改服务器文件位置"))
			return
		}
		if err := s.busyTaskError(""); err != nil {
			respond(w, 200, nil, err)
			return
		}

		cfg, err := s.loadPanelConfig()
		if err != nil {
			respond(w, 200, nil, err)
			return
		}

		// ServerDir 为空表示「保持/恢复自动探测」，此时只改架构
		if _, err := s.steam.Configure(req.ServerDir, req.Arch, ""); err != nil {
			respond(w, 200, nil, err)
			return
		}

		s.mgr.SetUgcDir(s.steam.WorkshopDir())
		s.syncModsSetup()

		// 落盘，重启面板后仍然生效。
		// 以磁盘上的现有配置为底，只覆盖本次请求涉及的字段——
		// 否则设置页保存一次就会把别的接口写入的字段（如 steamApiKey）抹掉
		cfg.ServerDir = strings.TrimSpace(req.ServerDir)
		cfg.Arch = s.steam.ArchPref()
		if err := panelcfg.Save(s.workDir, cfg); err != nil {
			respond(w, 200, s.serverStatus(),
				fmt.Errorf("设置已生效，但写入 %s 失败: %w", panelcfg.Path(s.workDir), err))
			return
		}
		respond(w, 200, s.serverStatus(), nil)

	default:
		respond(w, 200, nil, fmt.Errorf("不支持的方法: %s", r.Method))
	}
}
