//go:build linux

package steamcmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const linuxSteamCMDURL = "https://client-update.steamstatic.com/installer/steamcmd_linux.tar.gz"

func steamCmdExecutableName() string { return "steamcmd.sh" }

func steamCmdArchiveURL() string { return linuxSteamCMDURL }

func configureSteamCmdProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopSteamCmdProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	// steamcmd.sh 会再启动 linux32/steamcmd，必须杀整组进程才能真正停下。
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = cmd.Process.Kill()
	}
}

func (m *Manager) extractPlatformArchive(ctx context.Context, archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开 SteamCMD 压缩包失败: %w", err)
	}
	defer file.Close()

	compressed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("读取 SteamCMD gzip 压缩包失败: %w", err)
	}
	defer compressed.Close()

	if err := os.MkdirAll(destination, 0755); err != nil {
		return fmt.Errorf("创建 SteamCMD 解压目录失败: %w", err)
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("打开 SteamCMD 解压目录失败: %w", err)
	}
	defer root.Close()

	archive := tar.NewReader(compressed)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("读取 SteamCMD tar 压缩包失败: %w", err)
		}
		clean, err := safeArchiveEntry(header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if clean == "." {
				continue
			}
			if err := root.MkdirAll(clean, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if clean == "." {
				return fmt.Errorf("SteamCMD 压缩包包含非法路径: %s", header.Name)
			}
			if dir := filepath.Dir(clean); dir != "." {
				if err := root.MkdirAll(dir, 0755); err != nil {
					return err
				}
			}
			out, err := root.OpenFile(clean, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, archive)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("SteamCMD 压缩包包含不支持的条目: %s", header.Name)
		}
	}
}

func safeArchiveEntry(name string) (string, error) {
	local := filepath.FromSlash(strings.ReplaceAll(name, "\\", "/"))
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("SteamCMD 压缩包包含非法路径: %s", name)
	}
	return filepath.Clean(local), nil
}

func serverPaths(root, arch string) (serverDir, executable string) {
	if arch == Arch32 {
		serverDir = filepath.Join(root, "bin")
		return serverDir, filepath.Join(serverDir, "dontstarve_dedicated_server_nullrenderer")
	}
	serverDir = filepath.Join(root, "bin64")
	return serverDir, filepath.Join(serverDir, "dontstarve_dedicated_server_nullrenderer_x64")
}

func isGameExecutableName(name string) bool {
	switch strings.ToLower(name) {
	case "dontstarve_dedicated_server_nullrenderer", "dontstarve_dedicated_server_nullrenderer_x64":
		return true
	default:
		return false
	}
}

func platformSteamRoots() []string {
	var roots []string
	if path := strings.TrimSpace(os.Getenv("STEAM_PATH")); path != "" {
		roots = append(roots, path)
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"),
		)
	}
	return roots
}
