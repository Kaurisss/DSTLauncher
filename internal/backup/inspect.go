package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"dst-windows/internal/worldsettings"
)

const MaxArchiveBytes = int64(8 << 30)
const MaxRestoreBytes = int64(32 << 30)
const MaxArchiveFiles = 100000

var reservedPath = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)`)
var numericSnapshot = regexp.MustCompile(`^[0-9]+$`)

type ShardInspection struct {
	Name              string                    `json:"name"`
	Snapshots         int                       `json:"snapshots"`
	PlayerDirectories int                       `json:"playerDirectories"`
	PlayerFiles       int                       `json:"playerFiles"`
	WorldConfig       bool                      `json:"worldConfig"`
	ModsConfig        bool                      `json:"modsConfig"`
	State             *worldsettings.WorldState `json:"state,omitempty"`
	StateError        string                    `json:"stateError,omitempty"`
}
type Inspection struct {
	Name       string            `json:"name"`
	SHA256     string            `json:"sha256"`
	CheckedAt  time.Time         `json:"checkedAt"`
	CreatedAt  time.Time         `json:"createdAt"`
	Files      int               `json:"files"`
	Bytes      int64             `json:"bytes"`
	Shards     []ShardInspection `json:"shards"`
	Warnings   []string          `json:"warnings"`
	CanRestore bool              `json:"canRestore"`
}

func validName(name string) bool {
	return name != "" && name == filepath.Base(name) && !strings.ContainsAny(name, "/\\:\x00\r\n") && strings.HasSuffix(name, ".zip")
}
func (m *Manager) ArchivePath(name string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("非法的备份文件名")
	}
	return filepath.Join(m.backupDir, name), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func (m *Manager) Digest(ctx context.Context, name string) (string, error) {
	file, err := m.ArchivePath(name)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(m.backupDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	f, err := root.Open(filepath.Base(file))
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxArchiveBytes {
		return "", fmt.Errorf("备份不是普通 ZIP 或超过 8 GiB")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(contextReader{ctx, f}, MaxArchiveBytes+1))
	if err != nil {
		return "", err
	}
	if n > MaxArchiveBytes {
		return "", fmt.Errorf("备份超过 8 GiB")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func safeEntry(name string) error {
	if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00\r\n<>\"|?*") {
		return fmt.Errorf("备份包含非法路径: %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "." || strings.TrimRight(part, ". ") != part || reservedPath.MatchString(part) {
			return fmt.Errorf("备份包含非法路径: %q", name)
		}
	}
	return nil
}

// Stage verifies all ZIP payloads, including player files, in a private directory.
// It never modifies the live cluster directory.
func (m *Manager) Stage(ctx context.Context, name, destination string, expected []string, progress func(int, int)) (*Inspection, error) {
	file, err := m.ArchivePath(name)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxArchiveBytes {
		return nil, fmt.Errorf("备份不是普通 ZIP 或超过 8 GiB")
	}
	root, err := os.OpenRoot(m.backupDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxArchiveBytes {
		return nil, fmt.Errorf("备份不是普通 ZIP 或超过 8 GiB")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(contextReader{ctx, f}, MaxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if n > MaxArchiveBytes {
		return nil, fmt.Errorf("备份超过 8 GiB")
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return nil, fmt.Errorf("备份 ZIP 损坏: %w", err)
	}
	if len(zr.File) > MaxArchiveFiles {
		return nil, fmt.Errorf("备份条目超过 100000 个")
	}
	report := &Inspection{Name: name, SHA256: hex.EncodeToString(h.Sum(nil)), CheckedAt: time.Now(), CreatedAt: info.ModTime(), Shards: []ShardInspection{}, Warnings: []string{}}
	seen := map[string]bool{}
	canonical := map[string]string{}
	regular := map[string]bool{}
	for _, entry := range zr.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if err = safeEntry(name); err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("备份存在重复文件: %s", name)
		}
		seen[key] = true
		if entry.Mode()&os.ModeSymlink != 0 || !entry.FileInfo().IsDir() && !entry.Mode().IsRegular() {
			return nil, fmt.Errorf("备份不允许链接或特殊文件: %s", name)
		}
		for p := name; p != "."; p = path.Dir(p) {
			fold := strings.ToLower(p)
			if old, ok := canonical[fold]; ok && old != p {
				return nil, fmt.Errorf("备份存在大小写冲突: %s", p)
			}
			canonical[fold] = p
		}
		if !entry.FileInfo().IsDir() {
			regular[key] = true
			report.Files++
			if entry.UncompressedSize64 > uint64(MaxRestoreBytes) {
				return nil, fmt.Errorf("备份解压大小超过 32 GiB")
			}
			report.Bytes += int64(entry.UncompressedSize64)
			if report.Bytes > MaxRestoreBytes {
				return nil, fmt.Errorf("备份解压大小超过 32 GiB")
			}
		}
	}
	for name := range regular {
		for p := path.Dir(name); p != "."; p = path.Dir(p) {
			if regular[p] {
				return nil, fmt.Errorf("备份文件与目录冲突: %s", name)
			}
		}
	}
	if progress != nil {
		progress(0, report.Files)
	}
	if err = os.MkdirAll(destination, 0700); err != nil {
		return nil, err
	}
	completed := 0
	for _, entry := range zr.File {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		target := filepath.Join(destination, filepath.FromSlash(entry.Name))
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return nil, err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		src, err := entry.Open()
		if err != nil {
			return nil, err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			src.Close()
			return nil, err
		}
		n, copyErr := io.Copy(dst, io.LimitReader(contextReader{ctx, src}, int64(entry.UncompressedSize64)+1))
		src.Close()
		err = errors.Join(copyErr, dst.Close())
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", entry.Name, err)
		}
		if n != int64(entry.UncompressedSize64) {
			return nil, fmt.Errorf("备份文件长度不符: %s", entry.Name)
		}
		if !entry.Modified.IsZero() {
			if err = os.Chtimes(target, entry.Modified, entry.Modified); err != nil {
				return nil, err
			}
		}
		completed++
		if progress != nil {
			progress(completed, report.Files)
		}
	}
	return inspectStaged(ctx, destination, expected, report)
}
func inspectStaged(ctx context.Context, dir string, expected []string, report *Inspection) (*Inspection, error) {
	if info, err := os.Stat(filepath.Join(dir, "cluster.ini")); err != nil || !info.Mode().IsRegular() {
		report.Warnings = append(report.Warnings, "缺少 cluster.ini，不能恢复为完整房间")
	} else {
		report.CanRestore = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, name := range expected {
		names[name] = true
	}
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "save")); err == nil {
				names[e.Name()] = true
			}
		}
	}
	ordered := slices.Sorted(maps.Keys(names))
	totalWorlds := 0
	for _, name := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		shard := ShardInspection{Name: name}
		base := filepath.Join(dir, name)
		for _, file := range []string{"worldgenoverride.lua", "leveldataoverride.lua"} {
			if _, err := os.Stat(filepath.Join(base, file)); err == nil {
				shard.WorldConfig = true
			}
		}
		if _, err := os.Stat(filepath.Join(base, "modoverrides.lua")); err == nil {
			shard.ModsConfig = true
		}
		sessions, err := os.ReadDir(filepath.Join(base, "save", "session"))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for _, session := range sessions {
			if !session.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(base, "save", "session", session.Name()))
			if err != nil {
				return nil, err
			}
			worldFiles := map[string]bool{}
			for _, f := range files {
				if !f.IsDir() {
					worldFiles[f.Name()] = true
				}
			}
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".meta") && numericSnapshot.MatchString(strings.TrimSuffix(f.Name(), ".meta")) && !worldFiles[strings.TrimSuffix(f.Name(), ".meta")] {
					report.CanRestore = false
					report.Warnings = append(report.Warnings, fmt.Sprintf("%s 保存点 %s 缺少对应世界文件", name, strings.TrimSuffix(f.Name(), ".meta")))
				}
			}
			for _, f := range files {
				if !f.IsDir() {
					if numericSnapshot.MatchString(f.Name()) {
						shard.Snapshots++
					}
					continue
				}
				playerFiles, err := os.ReadDir(filepath.Join(base, "save", "session", session.Name(), f.Name()))
				if err != nil {
					return nil, err
				}
				count := 0
				for _, p := range playerFiles {
					if !p.IsDir() && numericSnapshot.MatchString(p.Name()) {
						count++
					}
				}
				if count > 0 {
					shard.PlayerDirectories++
					shard.PlayerFiles += count
				}
			}
		}
		totalWorlds += shard.Snapshots
		if shard.Snapshots == 0 {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s 没有世界保存点，可能尚未生成或未包含在备份中", name))
		}
		if shard.Snapshots > 0 {
			state, err := worldsettings.ReadWorldState(base)
			if err != nil {
				shard.StateError = err.Error()
			} else {
				shard.State = state
				if state != nil && state.SavedAt.Year() < 2000 {
					state.SavedAt = time.Time{}
				}
			}
			if shard.PlayerDirectories == 0 {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s 没有玩家保存目录", name))
			}
		}
		report.Shards = append(report.Shards, shard)
	}
	if totalWorlds == 0 {
		report.CanRestore = false
		report.Warnings = append(report.Warnings, "未找到世界保存点，不能恢复为空存档")
	}
	return report, nil
}
