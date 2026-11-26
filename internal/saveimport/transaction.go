package saveimport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CloneTree copies a cluster directory into a staging directory.
// Files under save are hard-linked when possible so importing an archive does
// not copy unchanged world data twice.
func CloneTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("集群目录包含不支持的文件: %s", rel)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if isSavePath(rel) {
			if err := os.Link(path, target); err == nil {
				return nil
			}
		}
		return copyFile(path, target)
	})
}

// CommitTree replaces target with staged and keeps the old directory at backup
// until the caller removes its temporary work directory.
func CommitTree(staged, target, backup string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}

	hadOld := false
	if _, err := os.Stat(target); err == nil {
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("暂存旧集群目录失败: %w", err)
		}
		hadOld = true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(staged, target); err != nil {
		if hadOld {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return fmt.Errorf("替换集群目录失败: %w，恢复旧目录也失败: %v", err, restoreErr)
			}
		}
		return fmt.Errorf("替换集群目录失败: %w", err)
	}
	return nil
}

func isSavePath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "save" {
			return true
		}
	}
	return false
}
