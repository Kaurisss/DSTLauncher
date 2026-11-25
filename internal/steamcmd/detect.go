package steamcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reVdfPath  = regexp.MustCompile(`(?i)"path"\s*"([^"]+)"`)
	reVdfIndex = regexp.MustCompile(`(?m)^\s*"\d+"\s*"([^"]+)"`)
)

// DetectInstalled finds valid dedicated-server installations in known Steam
// libraries. Both the standalone Dedicated Server tool and the main game client
// (which bundles the same dedicated-server executables) are scanned, with the
// standalone tool ranked first. Invalid or incomplete directories are omitted
// so callers can show every candidate without later failing on a missing binary.
func DetectInstalled() []Detected {
	return detectInstalledInLibraries(libraryRoots(steamRoots()))
}

func detectInstalledInLibraries(libraries []string) []Detected {
	targets := []struct {
		dirName string
		label   string
	}{
		{dirName: gameDirName, label: "Steam 库（专用服务器）："},
		{dirName: clientDirName, label: "Steam 库（游戏本体）："},
	}

	var found []Detected
	seen := make(map[string]bool)
	for _, target := range targets {
		for _, library := range libraries {
			root := filepath.Join(library, "steamapps", "common", target.dirName)
			layout, err := PickLayout(root, ArchAuto, "")
			if err != nil || seen[normalizePath(layout.Root)] {
				continue
			}
			seen[normalizePath(layout.Root)] = true
			found = append(found, Detected{
				Root:   layout.Root,
				Source: target.label + library,
			})
		}
	}
	return found
}

// FindAuto returns the first valid Steam-managed installation, if one exists.
func FindAuto() (root, source string, found bool) {
	installs := DetectInstalled()
	if len(installs) == 0 {
		return "", "", false
	}
	return installs[0].Root, installs[0].Source, true
}

func steamRoots() []string {
	return dedupe(platformSteamRoots())
}

func libraryRoots(steamRoots []string) []string {
	libraries := make([]string, 0, len(steamRoots)*2)
	for _, root := range steamRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		libraries = append(libraries, root)
		data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		libraries = append(libraries, vdfLibraryPaths(string(data))...)
	}
	return dedupe(libraries)
}

func vdfLibraryPaths(content string) []string {
	var paths []string
	for _, match := range reVdfPath.FindAllStringSubmatch(content, -1) {
		paths = append(paths, unescapeVDF(match[1]))
	}
	for _, match := range reVdfIndex.FindAllStringSubmatch(content, -1) {
		paths = append(paths, unescapeVDF(match[1]))
	}
	return dedupe(paths)
}

func unescapeVDF(value string) string {
	value = strings.ReplaceAll(value, `\\`, `\`)
	value = strings.ReplaceAll(value, `\"`, `"`)
	return strings.TrimSpace(value)
}

func dedupe(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		key := normalizePath(path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, filepath.Clean(path))
	}
	return out
}

func normalizePath(path string) string {
	return strings.ToLower(filepath.Clean(path))
}
