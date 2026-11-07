package migration

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dst-windows/internal/automation"
	"dst-windows/internal/room"
	"dst-windows/internal/roomprofile"
)

const (
	Format        = "dstlauncher-migration"
	MaxUpload     = int64(8 << 30)
	MaxUnpacked   = int64(32 << 30)
	MaxFiles      = 100000
	manifestLimit = 32 << 20
)

var profileID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var reservedName = regexp.MustCompile(`^(COM|LPT)[1-9]$`)

type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Format       string             `json:"format"`
	Version      int                `json:"version"`
	CreatedAt    time.Time          `json:"createdAt"`
	PanelVersion string             `json:"panelVersion"`
	GameVersion  string             `json:"gameVersion"`
	Rooms        []*room.Room       `json:"rooms"`
	Plan         *automation.Config `json:"plan"`
	Profiles     []string           `json:"profiles"`
	Files        []File             `json:"files"`
}

type Options struct {
	Root         string
	ConfDir      string
	Rooms        []*room.Room
	Plan         automation.Config
	Profiles     []*roomprofile.Profile
	PanelVersion string
	GameVersion  string
}

type sourceFile struct {
	name     string
	file     string
	data     []byte
	size     int64
	modified time.Time
	root     *os.Root
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

type archiveWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *archiveWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("迁移压缩包不能超过 8 GiB")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Create writes only the explicitly selected room directories and portable profiles.
func Create(ctx context.Context, destination string, options Options, progress func(int, int)) (*Manifest, error) {
	m := &Manifest{Format: Format, Version: 1, CreatedAt: time.Now(), PanelVersion: options.PanelVersion, GameVersion: options.GameVersion, Rooms: options.Rooms, Plan: &options.Plan, Profiles: []string{}, Files: []File{}}
	var sources []sourceFile
	var total int64
	for _, rm := range options.Rooms {
		dir := rm.ClusterDir(options.Root, options.ConfDir)
		root, err := os.OpenRoot(dir)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		err = filepath.WalkDir(dir, func(file string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("迁移目录不支持符号链接: %s", file)
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("不是普通存档文件: %s", file)
			}
			rel, err := filepath.Rel(dir, file)
			if err != nil {
				return err
			}
			name := "rooms/" + strconv.Itoa(rm.ID) + "/" + filepath.ToSlash(rel)
			if err := safePath(name); err != nil {
				return err
			}
			total += info.Size()
			if total > MaxUnpacked || len(sources) >= MaxFiles {
				return fmt.Errorf("迁移包超过 32 GiB 或 100000 个文件")
			}
			sources = append(sources, sourceFile{name: name, file: rel, size: info.Size(), modified: info.ModTime(), root: root})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("读取房间「%s」失败: %w", rm.Name, err)
		}
	}
	for _, p := range options.Profiles {
		if err := p.Validate(); err != nil {
			return nil, err
		}
		if !profileID.MatchString(p.ID) {
			return nil, fmt.Errorf("配置方案 ID 不合法")
		}
		b, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		total += int64(len(b))
		if total > MaxUnpacked || len(sources) >= MaxFiles {
			return nil, fmt.Errorf("迁移包过大")
		}
		sources = append(sources, sourceFile{name: "profiles/" + p.ID + ".json", data: b, size: int64(len(b)), modified: p.CreatedAt})
		m.Profiles = append(m.Profiles, p.ID)
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer out.Close()
	complete := false
	defer func() {
		if !complete {
			_ = out.Close()
			_ = os.Remove(destination)
		}
	}()
	zw := zip.NewWriter(&archiveWriter{writer: out, remaining: MaxUpload})
	if progress != nil {
		progress(0, len(sources))
	}
	for i, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		header := &zip.FileHeader{Name: source.name, Method: zip.Deflate}
		header.SetMode(0600)
		header.SetModTime(source.modified)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		var reader io.Reader = bytes.NewReader(source.data)
		var f *os.File
		if source.file != "" {
			f, err = source.root.Open(source.file)
			if err != nil {
				return nil, err
			}
			reader = f
		}
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(writer, hash), io.LimitReader(contextReader{ctx, reader}, source.size+1))
		if f != nil {
			f.Close()
		}
		if copyErr != nil {
			return nil, copyErr
		}
		if n != source.size {
			return nil, fmt.Errorf("打包期间文件发生变化: %s", source.name)
		}
		m.Files = append(m.Files, File{source.name, n, hex.EncodeToString(hash.Sum(nil))})
		if progress != nil {
			progress(i+1, len(sources))
		}
	}
	if err := validateManifest(m); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b) > manifestLimit {
		return nil, fmt.Errorf("迁移清单过大")
	}
	w, err := zw.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if err := out.Close(); err != nil {
		return nil, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxUpload {
		return nil, fmt.Errorf("迁移压缩包不能超过 8 GiB")
	}
	complete = true
	return m, nil
}

func safePath(name string) error {
	if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00\r\n") {
		return fmt.Errorf("迁移包包含非法路径: %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "." || strings.TrimRight(part, ". ") != part {
			return fmt.Errorf("迁移包包含非法路径: %q", name)
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if strings.ContainsAny(part, `<>"|?*`) || base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || reservedName.MatchString(base) {
			return fmt.Errorf("迁移包路径包含保留名称: %s", name)
		}
	}
	return nil
}

func validateManifest(m *Manifest) error {
	if m.Format != Format || m.Version != 1 || m.Plan == nil || len(m.Rooms) == 0 {
		return fmt.Errorf("不是受支持的完整迁移包")
	}
	if err := m.Plan.Validate(); err != nil {
		return fmt.Errorf("维护计划不合法: %w", err)
	}
	ids := map[int]bool{}
	for _, rm := range m.Rooms {
		if rm == nil || rm.ID < 1 || ids[rm.ID] {
			return fmt.Errorf("迁移包房间 ID 缺失或重复")
		}
		ids[rm.ID] = true
		if err := room.ValidateTemplate(rm); err != nil {
			return err
		}
		for _, value := range []string{rm.Name, rm.Description, rm.Token, rm.Password, rm.ClusterKey, rm.MasterIP} {
			if strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("房间字段包含非法换行或空字符")
			}
		}
	}
	profiles := map[string]bool{}
	for _, id := range m.Profiles {
		if !profileID.MatchString(id) || profiles[id] {
			return fmt.Errorf("配置方案 ID 不合法或重复")
		}
		profiles[id] = true
	}
	paths := map[string]bool{}
	var total int64
	if len(m.Files) > MaxFiles {
		return fmt.Errorf("迁移包文件数量过多")
	}
	for _, file := range m.Files {
		if err := safePath(file.Path); err != nil {
			return err
		}
		folded := strings.ToLower(file.Path)
		if paths[folded] || file.Size < 0 || file.Size > MaxUnpacked || !digestPattern.MatchString(file.SHA256) {
			return fmt.Errorf("迁移清单包含重复文件、非法大小或摘要: %s", file.Path)
		}
		paths[folded] = true
		parts := strings.Split(file.Path, "/")
		allowed := false
		if len(parts) >= 3 && parts[0] == "rooms" {
			id, err := strconv.Atoi(parts[1])
			allowed = err == nil && ids[id] && parts[1] == strconv.Itoa(id)
		} else if len(parts) == 2 && parts[0] == "profiles" && strings.HasSuffix(parts[1], ".json") {
			allowed = profiles[strings.TrimSuffix(parts[1], ".json")]
		}
		if !allowed {
			return fmt.Errorf("迁移包包含清单范围之外的文件: %s", file.Path)
		}
		total += file.Size
		if total > MaxUnpacked {
			return fmt.Errorf("解压总大小超过 32 GiB")
		}
	}
	for name := range paths {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if paths[parent] {
				return fmt.Errorf("文件和目录路径冲突: %s", name)
			}
		}
	}
	for _, rm := range m.Rooms {
		if !paths[fmt.Sprintf("rooms/%d/cluster.ini", rm.ID)] {
			return fmt.Errorf("房间「%s」缺少 cluster.ini", rm.Name)
		}
	}
	for id := range profiles {
		if !paths["profiles/"+id+".json"] {
			return fmt.Errorf("配置方案文件缺失: %s", id)
		}
	}
	return nil
}

// Extract verifies every manifest entry and CRC before exposing a preview for import.
func Extract(ctx context.Context, archive, destination string, progress func(int, int)) (*Manifest, error) {
	info, err := os.Stat(archive)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxUpload {
		return nil, fmt.Errorf("迁移压缩包不能超过 8 GiB")
	}
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return nil, fmt.Errorf("无法读取迁移 ZIP: %w", err)
	}
	defer zr.Close()
	entries := map[string]*zip.File{}
	seen := map[string]bool{}
	for _, f := range zr.File {
		name := strings.TrimSuffix(f.Name, "/")
		if err := safePath(name); err != nil {
			return nil, err
		}
		folded := strings.ToLower(name)
		if seen[folded] {
			return nil, fmt.Errorf("ZIP 存在重复路径: %s", name)
		}
		seen[folded] = true
		if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return nil, fmt.Errorf("ZIP 不允许链接或特殊文件: %s", name)
		}
		if !f.FileInfo().IsDir() {
			entries[name] = f
		}
		if len(seen) > MaxFiles+10000 {
			return nil, fmt.Errorf("ZIP 条目过多")
		}
	}
	meta, ok := entries["manifest.json"]
	if !ok || meta.UncompressedSize64 > manifestLimit {
		return nil, fmt.Errorf("缺少有效迁移清单 manifest.json")
	}
	reader, err := meta.Open()
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(contextReader{ctx, reader}, manifestLimit+1))
	reader.Close()
	if err != nil || len(b) > manifestLimit {
		return nil, fmt.Errorf("读取迁移清单失败")
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("迁移清单损坏: %w", err)
	}
	if err := validateManifest(&m); err != nil {
		return nil, err
	}
	if len(entries) != len(m.Files)+1 {
		return nil, fmt.Errorf("ZIP 文件与迁移清单不一致")
	}
	if progress != nil {
		progress(0, len(m.Files))
	}
	for i, file := range m.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, ok := entries[file.Path]
		if !ok || entry.UncompressedSize64 != uint64(file.Size) {
			return nil, fmt.Errorf("文件缺失或大小不符: %s", file.Path)
		}
		src, err := entry.Open()
		if err != nil {
			return nil, err
		}
		target := filepath.Join(destination, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			src.Close()
			return nil, err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			src.Close()
			return nil, err
		}
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(dst, hash), io.LimitReader(contextReader{ctx, src}, file.Size+1))
		src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("读取 %s: %w", file.Path, copyErr)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if n != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
			return nil, fmt.Errorf("文件 SHA-256 校验失败: %s", file.Path)
		}
		if !entry.Modified.IsZero() {
			if err := os.Chtimes(target, entry.Modified, entry.Modified); err != nil {
				return nil, err
			}
		}
		if progress != nil {
			progress(i+1, len(m.Files))
		}
	}
	for _, id := range m.Profiles {
		info, err := os.Stat(filepath.Join(destination, "profiles", id+".json"))
		if err != nil {
			return nil, err
		}
		if info.Size() > roomprofile.MaxSize {
			return nil, fmt.Errorf("配置方案超过 8 MiB")
		}
		b, err := os.ReadFile(filepath.Join(destination, "profiles", id+".json"))
		if err != nil {
			return nil, err
		}
		var p roomprofile.Profile
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, err
		}
		if err := p.Validate(); err != nil {
			return nil, err
		}
		if p.ID != id {
			return nil, fmt.Errorf("配置方案 ID 与清单不符")
		}
	}
	return &m, nil
}
