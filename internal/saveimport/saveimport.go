// Package saveimport 把一个外部存档（Klei 下载的整包、别的面板导出的整包，
// 或从游戏目录里捞出来的 Cluster_* 目录）导入到面板的房间里。
//
// 导入分两步，顺序不可颠倒，这是本包的核心约束：
//
//  1. 解析：从压缩包里读出 cluster.ini / server.ini / 世界设置，得到一个「房间草稿」；
//  2. 落盘：把存档的数据文件（各分片的 save 世界数据、世界设置、玩家名单）拷进目标房间目录。
//
// 之后必须再由 room.Generate 用面板的规则重写 cluster.ini / server.ini / cluster_token.txt。
// 刻意不照抄存档里的 ini：那里的端口、cluster_key、master_ip 都是**原机器**的值，
// 照抄会与本机的端口分配撞号，甚至让两个房间抢同一个端口。所以只取「玩法配置」，
// 机器本地字段一律以目标房间为准（详见 MergeInto）。
package saveimport

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dst-windows/internal/config"
	"dst-windows/internal/modmgr"
	"dst-windows/internal/room"
	"dst-windows/internal/worldsettings"
)

// clusterIniName 是存档根目录的判定文件，同时用于定位与解析。
const clusterIniName = "cluster.ini"

const (
	maxArchiveBytes = int64(8 << 30)
	maxExtractBytes = int64(32 << 30)
	maxExtractItems = 100000
)

// levelDataOverrideName 是游戏实际使用的世界配置文件。
//
// Klei 在服务器源码里写明：leveldataoverride.lua 供游戏使用、会完全覆盖已保存的世界数据，
// worldgenoverride.lua 才是给用户编辑的。导入时优先读取并原样保留前者，
// 避免转换成 worldgenoverride.lua 时丢失完整世界定义。
const levelDataOverrideName = "leveldataoverride.lua"

// ShardSave 是一个分片（世界）导入所需的全部信息。
type ShardSave struct {
	// Dir 是存档里该分片的实际目录名，导入时按它拷数据。
	Dir string
	// Name 是面板里的分片名，取自存档 server.ini 的 name，缺失时用目录名。
	// 面板不再把它归一化成 Master/Caves：多层世界里分片叫什么都可以，
	// 但名字必须与目录名一致（面板按 name 建目录、拼 -shard 参数）。
	Name string
	// WorldType 是世界位置标识，决定世界生成预设与可用设置项。
	// 优先取 leveldataoverride 的 location，缺失时再按预设或名字推断。
	WorldType string
	// GameID 是存档里的世界 id（server.ini [SHARD] id）。
	// 玩家的传送门记的是 id，导入时尽量沿用，避免导进来却传不过去。
	GameID   int
	IsMaster bool
	// HasSave 表示该分片带着真实世界数据（save 目录非空）。
	HasSave bool

	// Override 是该分片的世界设置；HasOverride 为 false 时表示存档没带世界配置。
	Override    *worldsettings.Override
	HasOverride bool
	// OverrideSrc 记录世界配置来自哪个文件，用于在界面上说明来源。
	OverrideSrc string
	// Mods 为 nil 表示来源未带配置；空 Overrides 表示明确清空此分片。
	Mods *modmgr.Overrides
}

// Parsed 是一份上传存档解析后的结果。
type Parsed struct {
	// ClusterDir 是解压后包含 cluster.ini 的那个目录。
	ClusterDir string
	// Room 是依据存档解析出的房间草稿。
	// 端口 / 集群密钥 / master_ip / 分片端口一律留空，由调用方按「新建 or 覆盖」决定。
	Room *room.Room
	// Shards 至少一项，Master 排在最前。
	Shards []ShardSave

	Token    string
	HasToken bool

	HasMods bool

	// 三份玩家名单的原文。Has* 表示存档里确实有该文件——
	// 只有确实存在时才覆盖目标房间的同名文件，否则会把面板里维护好的名单清空。
	AdminList string
	HasAdmin  bool
	BlockList string
	HasBlock  bool
	WhiteList string
	HasWhite  bool

	// HasSave 表示至少有一个分片带了真实世界数据。
	HasSave bool

	// Warnings 收集解析过程中的降级说明，最终展示给用户。
	Warnings []string
}

// warnf 记录一条降级说明。
func (p *Parsed) warnf(format string, args ...interface{}) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
}

// NewWorkDir 在存档根目录下开一个临时工作目录，用于放上传的压缩包与解压结果。
//
// 放在 rootDir 而不是系统临时目录：存档动辄几百 MB，
// 若系统盘与存档盘不是同一个盘，解压 + 拷贝会白白翻倍磁盘 IO。
// 调用方必须负责在结束时删除该目录。
func NewWorkDir(rootDir string) (string, error) {
	base := filepath.Join(rootDir, "tmp")
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	dir, err := os.MkdirTemp(base, "upload-")
	if err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	return dir, nil
}

// FindClusterDir 在解压目录里递归查找包含 cluster.ini 的目录。
//
// 存档的来源不同，压缩包里的层级也不一样，所以不能写死路径：
//   - Klei 网站 / 面板导出的整包：Cluster_1/cluster.ini
//   - 用户自己重新打包：可能又套了一层目录
//   - macOS 打包：会多出 __MACOSX、._* 之类的垃圾条目
//
// 用广度优先，保证浅层目录优先命中，避免被深层同名目录抢先。
func FindClusterDir(root string) (string, error) {
	queue := []string{root}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]

		if config.FileExists(filepath.Join(dir, clusterIniName)) {
			return dir, nil
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // 读不了的目录直接跳过，不影响继续找
		}
		for _, e := range entries {
			if !e.IsDir() || isJunkName(e.Name()) {
				continue
			}
			queue = append(queue, filepath.Join(dir, e.Name()))
		}
	}
	return "", fmt.Errorf("压缩包里没有找到 cluster.ini，请确认上传的是 DST 存档（含 Cluster_* 目录的整包）")
}

// isJunkName 判断是否是压缩包里应当忽略的目录/文件。
func isJunkName(name string) bool {
	switch name {
	case ".DS_Store", "Thumbs.db", "desktop.ini":
		return true
	}
	// __MACOSX 与 ._xxx 是 macOS 打包产物
	return name == "__MACOSX" || strings.HasPrefix(name, "._") || strings.HasPrefix(name, "__")
}

// Extract 把 zip 解压到 dstDir。
//
// 条目名一律当作不可信输入：压缩包可以被伪造，必须防路径穿越（Zip Slip）。
func Extract(zipPath, dstDir string) error {
	info, err := os.Stat(zipPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxArchiveBytes {
		return fmt.Errorf("存档压缩包不是普通文件或超过 8 GiB")
	}
	rc, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("不是有效的 zip 压缩包: %w", err)
	}
	defer rc.Close()

	absDst, err := filepath.Abs(dstDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absDst, 0755); err != nil {
		return fmt.Errorf("创建解压目录失败: %w", err)
	}
	root, err := os.OpenRoot(absDst)
	if err != nil {
		return fmt.Errorf("打开解压目录失败: %w", err)
	}
	defer root.Close()

	seen := map[string]bool{}
	var total int64
	for _, f := range rc.File {
		name := filepath.FromSlash(strings.ReplaceAll(f.Name, "\\", "/"))
		if name == "" {
			continue
		}
		if !filepath.IsLocal(name) {
			return fmt.Errorf("压缩包中存在非法路径: %s", f.Name)
		}
		clean := filepath.Clean(name)
		if len(seen) >= maxExtractItems {
			return fmt.Errorf("压缩包条目超过 %d 个", maxExtractItems)
		}
		key := strings.ToLower(filepath.ToSlash(clean))
		if seen[key] {
			return fmt.Errorf("压缩包存在重复路径: %s", f.Name)
		}
		seen[key] = true
		if isJunkPath(f.Name) {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return fmt.Errorf("压缩包不允许链接或特殊文件: %s", f.Name)
		}
		if f.UncompressedSize64 > uint64(maxExtractBytes) ||
			total > maxExtractBytes-int64(f.UncompressedSize64) {
			return fmt.Errorf("压缩包解压内容超过 32 GiB")
		}
		if !f.FileInfo().IsDir() {
			total += int64(f.UncompressedSize64)
		}

		if f.FileInfo().IsDir() {
			if clean == "." {
				continue
			}
			if err := root.MkdirAll(clean, 0755); err != nil {
				return err
			}
			continue
		}
		if clean == "." {
			return fmt.Errorf("压缩包中存在非法路径: %s", f.Name)
		}
		if dir := filepath.Dir(clean); dir != "." {
			if err := root.MkdirAll(dir, 0755); err != nil {
				return err
			}
		}
		if err := writeZipEntry(root, f, clean); err != nil {
			return fmt.Errorf("解压 %s 失败: %w", f.Name, err)
		}
	}
	return nil
}

// writeZipEntry 把一个压缩包条目写到 root 内的 target。
func writeZipEntry(root *os.Root, f *zip.File, target string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := root.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(src, int64(f.UncompressedSize64)+1))
	if err != nil {
		_ = dst.Close()
		return err
	}
	if n != int64(f.UncompressedSize64) {
		_ = dst.Close()
		return fmt.Errorf("压缩包条目大小不符")
	}
	return dst.Close()
}

// isJunkPath 判断条目路径里是否含应当忽略的段。
func isJunkPath(name string) bool {
	for seg := range strings.SplitSeq(filepath.ToSlash(name), "/") {
		if seg == "" || seg == "." {
			continue
		}
		if isJunkName(seg) {
			return true
		}
	}
	return false
}
