package room

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dst-windows/internal/config"
)

// Atomic replacement alone does not protect concurrent read-modify-write calls.
var playerListsMu sync.Mutex

const (
	BlockList = "blocklist"
	WhiteList = "whitelist"
)

func PlayerListPath(clusterDir, kind string) (string, error) {
	if kind != BlockList && kind != WhiteList {
		return "", fmt.Errorf("名单类型须为 blocklist 或 whitelist")
	}
	return filepath.Join(clusterDir, kind+".txt"), nil
}

func ReadPlayerList(clusterDir, kind string) ([]string, error) {
	path, err := PlayerListPath(clusterDir, kind)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", kind+".txt", err)
	}
	return normalizePlayerList(string(data)), nil
}

// Preserve valid IDs' casing and order when reading external files.
func normalizePlayerList(content string) []string {
	result := []string{}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimPrefix(content, "\uFEFF"), "\n") {
		id := NormalizeKleiID(line)
		key := strings.ToLower(id)
		if ValidKleiID(id) && !seen[key] {
			result = append(result, id)
			seen[key] = true
		}
	}
	return result
}

// ChangePlayerList does not kick players or send game commands. The game reads
// these files on startup. Both duplicate adds and repeated removes are safe.
func ChangePlayerList(clusterDir, kind, id string, remove bool) ([]string, error) {
	path, err := PlayerListPath(clusterDir, kind)
	if err != nil {
		return nil, err
	}
	id = NormalizeKleiID(id)
	if !ValidKleiID(id) {
		return nil, fmt.Errorf("Klei ID 不合法：应为 KU_ 开头、后接字母数字")
	}
	playerListsMu.Lock()
	defer playerListsMu.Unlock()
	list, err := ReadPlayerList(clusterDir, kind)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list)+1)
	found := false
	for _, existing := range list {
		if strings.EqualFold(existing, id) {
			found = true
			if remove {
				continue
			}
		}
		out = append(out, existing)
	}
	if !found && !remove {
		out = append(out, id)
	}
	if found == !remove {
		return out, nil
	}
	content := strings.Join(out, "\n")
	if len(out) > 0 {
		content += "\n"
	}
	if err := config.WriteFileAtomic(path, []byte(content)); err != nil {
		return nil, fmt.Errorf("保存 %s 失败: %w", kind+".txt", err)
	}
	return out, nil
}
