// Package backup 实现存档的备份、列表、清理与回滚。
//
// 备份粒度是「整个集群目录」(<root>/<confDir>/Cluster_N)，一次性覆盖
// cluster.ini、adminlist/whitelist/blocklist 与**全部分片**（不受分片数量影响），
// 因此不存在"只备份了地表、洞穴没备份"的问题；也不按分片分别打包——
// 多分片的存档有时序关联，回滚必须整体回。
//
// 关键设计：备份前必须先发 c_save() 并等待落盘完成。
// 直接拷贝 save 目录会拿到写了一半的存档，回滚后世界状态损坏。
// 该动作由调用方（Web 层）通过 server.Manager.SaveCluster 完成。
package backup

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"dst-windows/internal/config"
)

// Manager 管理备份的创建、列表与清理。
type Manager struct {
	rootDir     string
	confDir     string
	clusterName string // Cluster_N
	clusterDir  string // <root>/<confDir>/Cluster_N
	backupDir   string // <root>/backups/Cluster_N
	keepCount   int    // 保留最近 N 份，超出自动清理
	automatic   bool
}

// NewManager 创建备份管理器。
// rootDir 对应 -persistent_storage_root，confDir 对应 -conf_dir。
// 备份目录刻意放在 clusterDir 之外，避免被下次备份包进自己内部。
func NewManager(rootDir, confDir string, clusterID int) *Manager {
	clusterName := fmt.Sprintf("Cluster_%d", clusterID)
	return &Manager{
		rootDir:     rootDir,
		confDir:     confDir,
		clusterName: clusterName,
		clusterDir:  config.ClusterDir(rootDir, confDir, clusterID),
		backupDir:   filepath.Join(rootDir, "backups", clusterName),
		keepCount:   10,
	}
}

// BackupItem 描述一个备份文件。
type BackupItem struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	Path      string    `json:"path"`
}

// hasSave 判断集群下是否已经生成过世界存档（任一分片有实际存档内容）。
// 没有存档时拒绝生成空备份，避免用户以为备份成功、实际什么都没存。
func (m *Manager) hasSave() bool {
	entries, err := os.ReadDir(m.clusterDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if config.ShardHasSave(filepath.Join(m.clusterDir, e.Name())) {
			return true
		}
	}
	return false
}

// Create 创建一个新的备份：压缩整个集群目录为 zip。
func (m *Manager) Create() (*BackupItem, error) {
	src := m.clusterDir
	if _, err := os.Stat(src); err != nil {
		return nil, fmt.Errorf("集群目录不存在: %s（服务器可能尚未运行过）", src)
	}
	if !m.hasSave() {
		return nil, fmt.Errorf("尚未生成世界存档，暂无可备份内容")
	}

	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return nil, err
	}

	prefix := m.clusterName
	if m.automatic {
		prefix += "_auto"
	}
	name := fmt.Sprintf("%s_%s.zip", prefix, time.Now().Format("20060102_150405.000000000"))
	dst := filepath.Join(m.backupDir, name)

	tmp := dst + ".tmp"
	if err := m.zipDir(src, tmp); err != nil {
		// 备份失败时清理半成品文件，避免占用磁盘
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}

	fi, err := os.Stat(dst)
	if err != nil {
		return nil, err
	}

	item := &BackupItem{
		Name:      name,
		Size:      fi.Size(),
		CreatedAt: fi.ModTime(),
		Path:      dst,
	}

	// 备份成功后自动清理超出保留份数的旧备份
	if err := m.Clean(); err != nil {
		// 清理失败不影响本次备份结果
		fmt.Printf("清理旧备份失败: %v\n", err)
	}

	return item, nil
}

// CreateAutomatic rotates only automatic backups; manual backups remain untouched.
func (m *Manager) CreateAutomatic(keep int) (*BackupItem, error) {
	if keep < 1 || keep > 100 {
		return nil, fmt.Errorf("自动备份保留份数必须为 1–100")
	}
	auto := *m
	auto.automatic, auto.keepCount = true, keep
	return auto.Create()
}

// zipDir 将目录递归压缩为 zip。
func (m *Manager) zipDir(srcDir, dstPath string) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	root, err := os.OpenRoot(srcDir)
	if err != nil {
		return err
	}
	defer root.Close()

	// srcDir 必须以分隔符结尾，否则 filepath.Walk 会把父目录也遍历进来
	err = filepath.Walk(srcDir+string(os.PathSeparator), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("备份不支持链接或特殊文件: %s", path)
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// zip 条目统一用正斜杠，保证压缩包跨平台可解
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name, header.Method = filepath.ToSlash(rel), zip.Deflate
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := root.Open(rel)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		closeErr := src.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		_ = zw.Close()
		return fmt.Errorf("压缩失败: %w", err)
	}

	if err := zw.Close(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

// List 返回所有备份，按时间倒序。
func (m *Manager) List() ([]BackupItem, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupItem{}, nil
		}
		return nil, err
	}

	var out []BackupItem
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupItem{
			Name:      e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
			Path:      filepath.Join(m.backupDir, e.Name()),
		})
	}

	slices.SortFunc(out, func(a, b BackupItem) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return out, nil
}

// Clean 删除超出保留份数的旧备份。
func (m *Manager) Clean() error {
	items, err := m.List()
	if err != nil {
		return err
	}
	filtered := items[:0]
	for _, item := range items {
		if strings.HasPrefix(item.Name, m.clusterName+"_auto_") == m.automatic {
			filtered = append(filtered, item)
		}
	}
	items = filtered
	if len(items) <= m.keepCount {
		return nil
	}
	// 保留最新的 keepCount 份，删除更早的
	for _, item := range items[m.keepCount:] {
		if err := os.Remove(item.Path); err != nil {
			return fmt.Errorf("删除旧备份失败: %w", err)
		}
	}
	return nil
}

// Delete 删除指定备份。
func (m *Manager) Delete(name string) error {
	// 校验文件名，防止路径穿越
	if name != filepath.Base(name) || !strings.HasSuffix(name, ".zip") {
		return fmt.Errorf("非法的备份文件名")
	}
	return os.Remove(filepath.Join(m.backupDir, name))
}

// Restore 从备份回滚整个集群目录。
// 注意：调用方必须确保服务器已停止，否则文件占用会导致解压失败，
// 且运行中的服务器会在退出时用内存状态覆盖刚恢复的存档。
func (m *Manager) Restore(name string) error {
	if err := os.MkdirAll(filepath.Dir(m.clusterDir), 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(m.clusterDir), ".restore-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	stage := filepath.Join(dir, "payload")
	report, err := m.Stage(context.Background(), name, stage, nil, nil)
	if err != nil {
		return err
	}
	if !report.CanRestore {
		return fmt.Errorf("备份检查未通过: %s", strings.Join(report.Warnings, "；"))
	}
	_, err = m.RestoreStaged(context.Background(), name, stage, report.SHA256)
	return err
}

// RestoreStaged consumes a private, fully verified Stage directory. Live data is
// renamed only after the source archive still matches the checked digest.
func (m *Manager) RestoreStaged(ctx context.Context, name, stage, digest string) (string, error) {
	current, err := m.Digest(ctx, name)
	if err != nil {
		return "", err
	}
	if current != digest {
		return "", fmt.Errorf("备份在检查后发生变化，请重新检查")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	previous := ""
	if _, err = os.Stat(m.clusterDir); err == nil {
		previous = m.clusterDir + ".before-restore-" + time.Now().Format("20060102_150405") + "-" + rand.Text()
		if err = os.Rename(m.clusterDir, previous); err != nil {
			return "", fmt.Errorf("保留当前存档失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err = os.Rename(stage, m.clusterDir); err != nil {
		if previous != "" {
			err = errors.Join(err, os.Rename(previous, m.clusterDir))
		}
		return "", fmt.Errorf("替换存档失败: %w", err)
	}
	return previous, nil
}
