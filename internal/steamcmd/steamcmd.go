// Package steamcmd locates and manages the Don't Starve Together dedicated server.
package steamcmd

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	ArchAuto = "auto"
	Arch64   = "64"
	Arch32   = "32"

	gameAppID       = "343050"
	gameDirName     = "Don't Starve Together Dedicated Server"
	steamCMDDirName = "steamcmd"
	maxOutputLines  = 400
)

// 各阶段百分比独立计算，必须同时显示阶段名，避免把校验进度当成下载回退。
var steamProgressRE = regexp.MustCompile(`(?i)Update state \(0x[0-9a-f]+\)\s+(preallocating|downloading|verifying install),\s*progress:\s*(\d+(?:\.\d+)?)\s*\(`)

// Detected is an installed dedicated-server directory discovered in a Steam library.
type Detected struct {
	Root   string `json:"root"`
	Source string `json:"source"`
}

// Layout describes the executable chosen from one dedicated-server installation.
type Layout struct {
	Root       string
	ServerDir  string
	GameDir    string
	Executable string
	Arch       string
	Source     string
}

// Manager owns either the panel-managed installation or an existing Steam installation.
type Manager struct {
	workDir     string
	steamCmdDir string
	archiveURL  string
	makeCmd     func(name string, args ...string) *exec.Cmd

	mu         sync.RWMutex
	layout     Layout
	archPref   string
	external   bool
	pinned     bool
	installing bool
	canceled   bool
	progress   string
	errText    string
	output     []string
	cancel     context.CancelFunc
	cmd        *exec.Cmd
}

// NormalizeArch normalizes an architecture preference. Invalid and empty values fall
// back to auto so a hand-edited panel.json cannot prevent the panel from starting.
func NormalizeArch(arch string) string {
	switch strings.TrimSpace(strings.ToLower(arch)) {
	case Arch32:
		return Arch32
	case Arch64:
		return Arch64
	default:
		return ArchAuto
	}
}

// NewManager creates a manager pointed at the panel-owned installation location.
// Configure may later replace it with a discovered or explicitly selected install.
func NewManager(workDir, archPref string) *Manager {
	archPref = NormalizeArch(archPref)
	root := ownRoot(workDir)
	return &Manager{
		workDir:     workDir,
		steamCmdDir: filepath.Join(workDir, steamCMDDirName),
		archPref:    archPref,
		layout:      plannedLayout(root, archPref, "面板自带安装"),
	}
}

// Configure selects a dedicated-server installation. An empty serverDir means use
// an installed Steam copy when one is discoverable, otherwise use the panel-managed
// location and wait for the user to install it.
func (m *Manager) Configure(serverDir, arch, source string) (string, error) {
	archPref := NormalizeArch(arch)
	serverDir = strings.TrimSpace(serverDir)

	var (
		layout   Layout
		external bool
		pinned   bool
		err      error
	)

	switch {
	case serverDir != "":
		if layout, err = PickLayout(serverDir, archPref, source); err != nil {
			return "", err
		}
		if layout.Source == "" {
			layout.Source = "手动指定"
		}
		external = !samePath(layout.Root, ownRoot(m.workDir))
		pinned = true

	default:
		if root, detectedSource, found := FindAuto(); found {
			layout, err = PickLayout(root, archPref, detectedSource)
			if err != nil {
				return "", err
			}
			external = true
		} else {
			layout = plannedLayout(ownRoot(m.workDir), archPref, "面板自带安装")
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.installing {
		return "", fmt.Errorf("正在安装服务器文件，请等待安装结束后再更改位置")
	}
	m.layout = layout
	m.archPref = archPref
	m.external = external
	m.pinned = pinned
	return layout.Root, nil
}

// ServerDir returns the bin or bin64 directory used as the DST process working directory.
func (m *Manager) ServerDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.layout.ServerDir
}

// Root returns the dedicated-server installation root.
func (m *Manager) Root() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.layout.Root
}

// WorkshopDir reuses the selected Steam library; standalone installs keep their own cache.
func (m *Manager) WorkshopDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	common := filepath.Dir(m.layout.Root)
	steamapps := filepath.Dir(common)
	if m.external && strings.EqualFold(filepath.Base(common), "common") &&
		strings.EqualFold(filepath.Base(steamapps), "steamapps") {
		return filepath.Join(steamapps, "workshop")
	}
	return filepath.Join(m.workDir, "ugc_mods")
}

// GameDir returns the directory that contains the game data and mods directory.
func (m *Manager) GameDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.layout.GameDir
}

// Executable returns the selected dedicated-server executable path.
func (m *Manager) Executable() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.layout.Executable
}

// Arch returns the selected executable architecture.
func (m *Manager) Arch() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.layout.Arch
}

// ArchPref returns the user preference before layout selection.
func (m *Manager) ArchPref() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.archPref
}

// Source returns a human-readable description of the selected installation source.
func (m *Manager) Source() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.layout.Source
}

// Pinned reports whether the current path was explicitly selected by the user.
func (m *Manager) Pinned() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pinned
}

// IsExternal reports whether the selected install is managed outside this panel.
func (m *Manager) IsExternal() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.external
}

// IsInstalled reports whether the selected executable exists.
func (m *Manager) IsInstalled() bool {
	m.mu.RLock()
	executable := m.layout.Executable
	m.mu.RUnlock()
	info, err := os.Stat(executable)
	return err == nil && !info.IsDir()
}

// IsInstalling reports whether SteamCMD is currently running.
func (m *Manager) IsInstalling() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.installing
}

// IsCanceled reports whether the last installation or update was canceled by the user.
func (m *Manager) IsCanceled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.canceled
}

// Cancel stops an active installation or update. Already-downloaded SteamCMD chunks
// remain on disk so a subsequent install/update can resume from where it stopped.
func (m *Manager) Cancel() error {
	m.mu.Lock()
	if !m.installing {
		m.mu.Unlock()
		return fmt.Errorf("当前没有正在进行的服务器安装或更新")
	}
	m.canceled = true
	m.progress = "正在取消安装…"
	cancel := m.cancel
	cmd := m.cmd
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil {
		stopSteamCmdProcess(cmd)
	}
	return nil
}

// SteamCmdExists reports whether the panel's SteamCMD bootstrap has been installed.
func (m *Manager) SteamCmdExists() bool {
	info, err := os.Stat(filepath.Join(m.steamCmdDir, steamCmdExecutableName()))
	return err == nil && !info.IsDir()
}

// Version returns version.txt from the current game installation when available.
func (m *Manager) Version() string {
	m.mu.RLock()
	path := filepath.Join(m.layout.Root, "version.txt")
	m.mu.RUnlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Progress returns a stable snapshot suitable for API responses.
func (m *Manager) Progress() (progress, errText string, output []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.progress, m.errText, append([]string(nil), m.output...)
}

// Install starts the panel-managed installation asynchronously. Errors are exposed
// through Progress because the existing HTTP API starts this task asynchronously.
func (m *Manager) Install() {
	m.startInstall(false)
}

// Update starts an asynchronous SteamCMD update for the panel-managed install.
func (m *Manager) Update() error {
	m.mu.RLock()
	external := m.external
	installing := m.installing
	m.mu.RUnlock()
	if external {
		return fmt.Errorf("当前使用的是外部服务器安装，不能由面板更新")
	}
	if installing {
		return fmt.Errorf("服务器文件安装或更新正在进行中")
	}
	if !m.IsInstalled() {
		return fmt.Errorf("服务器尚未安装")
	}
	m.startInstall(true)
	return nil
}

func (m *Manager) startInstall(update bool) {
	m.mu.Lock()
	if m.installing {
		m.mu.Unlock()
		return
	}
	if m.external {
		m.errText = "当前使用的是外部服务器安装，不能由面板安装或更新"
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.installing = true
	m.canceled = false
	m.cancel = cancel
	m.cmd = nil
	m.errText = ""
	m.output = nil
	if update {
		m.progress = "正在检查并更新 DST 专用服务器…"
	} else {
		m.progress = "正在准备安装 DST 专用服务器…"
	}
	m.mu.Unlock()

	go m.doInstall(ctx, cancel)
}

func (m *Manager) doInstall(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()

	var err error
	if !m.SteamCmdExists() {
		m.setProgress("正在下载 SteamCMD…")
		err = m.downloadSteamCmd(ctx)
	}
	if err == nil {
		m.setProgress("正在通过 SteamCMD 安装 DST 专用服务器…")
		err = m.runSteamCmd(ctx)
	}
	if err == nil {
		err = m.refreshLayout()
	}
	if err == nil && !m.IsInstalled() {
		err = fmt.Errorf("SteamCMD 已结束，但未找到服务器可执行文件")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	canceled := m.canceled || errors.Is(err, context.Canceled) || ctx.Err() != nil
	m.installing = false
	m.cancel = nil
	m.cmd = nil
	if canceled {
		m.canceled = true
		m.errText = "已取消安装（已下载的文件会保留，可随时继续）"
		m.progress = "已取消安装"
		m.appendOutputLocked("[!] 已手动取消安装，已下载的文件已保留")
		return
	}
	if err != nil {
		m.errText = err.Error()
		m.progress = "安装或更新失败"
		m.appendOutputLocked("[!] " + err.Error())
		return
	}
	m.progress = "服务器文件已就绪"
}

func (m *Manager) refreshLayout() error {
	m.mu.RLock()
	root, pref, source := m.layout.Root, m.archPref, m.layout.Source
	m.mu.RUnlock()

	layout, err := PickLayout(root, pref, source)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.layout = layout
	m.mu.Unlock()
	return nil
}

func (m *Manager) runSteamCmd(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.RLock()
	steamCmdDir := m.steamCmdDir
	installRoot := ownRoot(m.workDir)
	m.mu.RUnlock()

	steamCmd := filepath.Join(steamCmdDir, steamCmdExecutableName())
	if _, err := os.Stat(steamCmd); err != nil {
		return fmt.Errorf("未找到 SteamCMD: %w", err)
	}
	if err := os.MkdirAll(installRoot, 0755); err != nil {
		return fmt.Errorf("创建服务器目录失败: %w", err)
	}

	makeCmd := m.makeCmd
	if makeCmd == nil {
		makeCmd = exec.Command
	}
	cmd := makeCmd(steamCmd,
		"+force_install_dir", installRoot,
		"+login", "anonymous",
		"+app_update", gameAppID, "validate",
		"+quit",
	)
	cmd.Dir = steamCmdDir
	configureSteamCmdProcess(cmd)
	writer := &outputWriter{manager: m}
	cmd.Stdout = writer
	cmd.Stderr = writer

	m.mu.Lock()
	if m.canceled || ctx.Err() != nil {
		m.mu.Unlock()
		return context.Canceled
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("启动 SteamCMD 失败: %w", err)
	}
	m.cmd = cmd
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
		}
		m.mu.Unlock()
	}()

	err := cmd.Wait()
	writer.flush()
	if m.IsCanceled() || ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		return steamCmdError(err)
	}
	return nil
}

// steamCmdError 把 SteamCMD 的退出码翻译成可操作的提示。
// 实测大陆网络下最常见的是下载中途断流，SteamCMD 报
// 「Error! App '343050' state is 0x402 after update job」并返回 8；
// 直接抛「exit status 8」对用户没有任何指导意义。
func steamCmdError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 8 {
		return fmt.Errorf("SteamCMD 下载中断（exit 8，App state 0x402）："+
			"通常是到 Steam 内容服务器的连接中断或超时。"+
			"已下载的部分会保留，重新点击安装/更新即可续传；"+
			"若持续失败请检查网络（部分网络环境需要代理）或磁盘空间: %w", err)
	}
	return fmt.Errorf("SteamCMD 执行失败: %w", err)
}

func (m *Manager) downloadSteamCmd(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.steamCmdDir, 0755); err != nil {
		return fmt.Errorf("创建 SteamCMD 目录失败: %w", err)
	}

	url := m.archiveURL
	if url == "" {
		url = steamCmdArchiveURL()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建 SteamCMD 下载请求失败: %w", err)
	}
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("下载 SteamCMD 失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 SteamCMD 失败: HTTP %s", response.Status)
	}

	tmp, err := os.CreateTemp(m.steamCmdDir, "steamcmd-*.zip")
	if err != nil {
		return fmt.Errorf("创建 SteamCMD 临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	reader := &downloadReader{reader: response.Body, total: response.ContentLength, manager: m}
	if _, err := io.Copy(tmp, reader); err != nil {
		_ = tmp.Close()
		if ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("保存 SteamCMD 下载文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭 SteamCMD 下载文件失败: %w", err)
	}
	return m.extractPlatformArchive(ctx, tmpPath, m.steamCmdDir)
}

func (m *Manager) extractZip(ctx context.Context, zipPath, destination string) error {
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 SteamCMD 压缩包失败: %w", err)
	}
	defer archive.Close()

	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(destination, entry.Name)
		rel, err := filepath.Rel(destination, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("SteamCMD 压缩包包含非法路径: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		src, err := entry.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entry.Mode())
		if err == nil {
			_, err = io.Copy(dst, src)
			closeErr := dst.Close()
			if err == nil {
				err = closeErr
			}
		}
		_ = src.Close()
		if err != nil {
			return fmt.Errorf("解压 SteamCMD 文件 %s 失败: %w", entry.Name, err)
		}
	}
	return nil
}

func (m *Manager) setProgress(progress string) {
	m.mu.Lock()
	m.progress = progress
	m.mu.Unlock()
}

func (m *Manager) appendOutput(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendOutputLocked(line)
}

func (m *Manager) appendOutputLocked(line string) {
	m.output = append(m.output, line)
	if len(m.output) > maxOutputLines {
		m.output = append([]string(nil), m.output[len(m.output)-maxOutputLines:]...)
	}
	if progress := extractProgress(line); progress != "" {
		m.progress = progress
	}
}

func extractProgress(line string) string {
	match := steamProgressRE.FindStringSubmatch(line)
	if len(match) < 3 {
		return ""
	}
	var stage string
	switch strings.ToLower(match[1]) {
	case "preallocating":
		stage = "预分配磁盘空间"
	case "downloading":
		stage = "下载服务器文件"
	case "verifying install":
		stage = "校验服务器文件"
	}
	return stage + " " + match[2] + "%"
}

type outputWriter struct {
	manager *Manager
	mu      sync.Mutex
	pending string
}

func (w *outputWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	text := w.pending + string(data)
	lines := strings.Split(text, "\n")
	w.pending = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		w.manager.appendOutput(line)
	}
	return len(data), nil
}

func (w *outputWriter) flush() {
	w.mu.Lock()
	line := w.pending
	w.pending = ""
	w.mu.Unlock()
	w.manager.appendOutput(line)
}

type downloadReader struct {
	reader  io.Reader
	total   int64
	read    int64
	manager *Manager
	lastPct int
}

func (r *downloadReader) Read(data []byte) (int, error) {
	n, err := r.reader.Read(data)
	r.read += int64(n)
	if r.total > 0 && n > 0 {
		pct := int(r.read * 100 / r.total)
		if pct > r.lastPct {
			r.lastPct = pct
			r.manager.setProgress(fmt.Sprintf("正在下载 SteamCMD %d%%", pct))
		}
	}
	return n, err
}

func ownRoot(workDir string) string {
	return filepath.Join(workDir, "steamapps", "common", gameDirName)
}

func plannedLayout(root, archPref, source string) Layout {
	arch := Arch64
	if NormalizeArch(archPref) == Arch32 {
		arch = Arch32
	}
	return layoutFor(root, arch, source)
}

// PickLayout validates a dedicated-server directory and selects an executable.
// root may point to the installation root, bin/bin64, or an executable itself.
func PickLayout(root, arch, source string) (Layout, error) {
	root = normalizeRoot(root)
	if root == "" || root == "." {
		return Layout{}, fmt.Errorf("服务器目录不能为空")
	}

	want := NormalizeArch(arch)
	try := func(candidate string) (Layout, bool) {
		layout := layoutFor(root, candidate, source)
		info, err := os.Stat(layout.Executable)
		return layout, err == nil && !info.IsDir()
	}

	switch want {
	case Arch64:
		if layout, ok := try(Arch64); ok {
			return layout, nil
		}
	case Arch32:
		if layout, ok := try(Arch32); ok {
			return layout, nil
		}
	default:
		if layout, ok := try(Arch64); ok {
			return layout, nil
		}
		if layout, ok := try(Arch32); ok {
			return layout, nil
		}
	}

	return Layout{}, fmt.Errorf(
		"未在 %s 找到 DST 专用服务器可执行文件（需要 %s 或 %s）",
		root,
		layoutFor(root, Arch64, "").Executable,
		layoutFor(root, Arch32, "").Executable,
	)
}

func layoutFor(root, arch, source string) Layout {
	serverDir, executable := serverPaths(root, arch)
	return Layout{
		Root:       root,
		ServerDir:  serverDir,
		GameDir:    root,
		Executable: executable,
		Arch:       arch,
		Source:     source,
	}
}

func normalizeRoot(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "bin", "bin64":
		return filepath.Dir(path)
	default:
		if isGameExecutableName(base) {
			return filepath.Dir(filepath.Dir(path))
		}
		return path
	}
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
