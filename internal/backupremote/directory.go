package backupremote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type cancelReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func backupOrder(name string) string {
	parts := strings.SplitN(name, "_", 3)
	return strings.TrimPrefix(parts[2], "auto_")
}
func (e *Engine) syncDirectory(ctx context.Context, c Config, room int, name string, source *os.File) error {
	if err := os.MkdirAll(c.Directory, 0700); err != nil {
		return fmt.Errorf("创建异地备份目录失败: %w", err)
	}
	// Check resolved paths too: a selected junction must not point back into live saves.
	actual, err := filepath.EvalSymlinks(c.Directory)
	if err != nil {
		return err
	}
	check := c
	check.Directory = actual
	if err = e.validate(check); err != nil {
		return err
	}
	dir := filepath.Join(c.Directory, "dstlauncher-"+e.namespace, fmt.Sprintf("Cluster_%d", room))
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, cancelReader{ctx, source})
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && ownBackupName.MatchString(entry.Name()) && strings.HasPrefix(entry.Name(), fmt.Sprintf("Cluster_%d_", room)) {
			names = append(names, entry.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool { return backupOrder(names[i]) > backupOrder(names[j]) })
	retained := 1 // The copy that just completed must remain available, including a manual retry.
	for _, old := range names {
		if old == name {
			continue
		}
		if retained < c.Keep {
			retained++
			continue
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = os.Remove(filepath.Join(dir, old)); err != nil {
			return fmt.Errorf("同步已完成，清理旧异地备份失败: %w", err)
		}
	}
	return nil
}
