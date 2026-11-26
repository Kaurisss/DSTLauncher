package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// HardenPrivateFiles repairs permissions on existing sensitive files during startup.
// Windows does not use these Unix mode bits, so it leaves them unchanged there.
func HardenPrivateFiles(paths ...string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	for _, path := range paths {
		if err := hardenPrivatePath(path); err != nil {
			return err
		}
	}
	return nil
}

func hardenPrivatePath(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if info.Mode()&os.ModeSymlink == 0 {
			return os.Chmod(path, 0600)
		}
		return nil
	}
	return filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(current, 0700)
		}
		return os.Chmod(current, 0600)
	})
}
