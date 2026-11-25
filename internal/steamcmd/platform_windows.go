//go:build windows

package steamcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const windowsSteamCMDURL = "https://client-update.steamstatic.com/installer/steamcmd.zip"

var reRegValue = regexp.MustCompile(`(?m)^\s*[^\s]+\s+REG_\w+\s+(.+?)\s*$`)

func steamCmdExecutableName() string { return "steamcmd.exe" }

func steamCmdArchiveURL() string { return windowsSteamCMDURL }

func (m *Manager) extractPlatformArchive(ctx context.Context, archivePath, destination string) error {
	return m.extractZip(ctx, archivePath, destination)
}

func configureSteamCmdProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func stopSteamCmdProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	// SteamCMD 可能拉起自更新子进程，优先按进程树强制结束，失败再退回直接 Kill。
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}

func serverPaths(root, arch string) (serverDir, executable string) {
	if arch == Arch32 {
		serverDir = filepath.Join(root, "bin")
		return serverDir, filepath.Join(serverDir, "dontstarve_dedicated_server_nullrenderer.exe")
	}
	serverDir = filepath.Join(root, "bin64")
	return serverDir, filepath.Join(serverDir, "dontstarve_dedicated_server_nullrenderer_x64.exe")
}

func isGameExecutableName(name string) bool {
	switch strings.ToLower(name) {
	case "dontstarve_dedicated_server_nullrenderer.exe", "dontstarve_dedicated_server_nullrenderer_x64.exe":
		return true
	default:
		return false
	}
}

func platformSteamRoots() []string {
	roots := []string{
		regString(`HKCU\Software\Valve\Steam`, "SteamPath"),
		regString(`HKLM\SOFTWARE\Valve\Steam`, "InstallPath"),
		regString(`HKLM\SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"),
	}
	for _, variable := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if base := os.Getenv(variable); base != "" {
			roots = append(roots, filepath.Join(base, "Steam"))
		}
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		roots = append(roots, filepath.Join(home, "Steam"))
	}
	for _, variable := range []string{"SystemDrive", "HOMEDRIVE"} {
		drive := strings.TrimRight(strings.TrimSpace(os.Getenv(variable)), `\`)
		if drive == "" {
			continue
		}
		roots = append(roots,
			filepath.Join(drive+`\`, "Steam"),
			filepath.Join(drive+`\`, "SteamLibrary"),
			filepath.Join(drive+`\`, "Games", "Steam"),
			filepath.Join(drive+`\`, "Games", "SteamLibrary"),
		)
	}
	return roots
}

func regString(key, value string) string {
	out, err := exec.Command("reg.exe", "query", key, "/v", value).Output()
	if err != nil {
		return ""
	}
	match := reRegValue.FindStringSubmatch(string(out))
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
