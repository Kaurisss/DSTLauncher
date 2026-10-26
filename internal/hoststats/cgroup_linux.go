//go:build linux

package hoststats

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readContainerMemory() (*Capacity, error) {
	membership, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return nil, nil
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, nil
	}
	return containerMemory(string(membership), string(mounts), os.ReadFile)
}

func hasController(controllers, name string) bool {
	for _, part := range strings.Split(controllers, ",") {
		if part == name {
			return true
		}
	}
	return false
}

func mountPath(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

// Both the process cgroup and its visible ancestors may limit allocations. The
// smallest remaining headroom also accounts for siblings using a parent quota.
func containerMemory(membership, mounts string, read func(string) ([]byte, error)) (*Capacity, error) {
	for _, line := range strings.Split(mounts, "\n") {
		left, right, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		before, after := strings.Fields(left), strings.Fields(right)
		if len(before) < 5 || len(after) < 3 {
			continue
		}
		v2 := after[0] == "cgroup2"
		if !v2 && (after[0] != "cgroup" || !hasController(after[2], "memory")) {
			continue
		}
		for _, entry := range strings.Split(membership, "\n") {
			parts := strings.SplitN(entry, ":", 3)
			if len(parts) != 3 {
				continue
			}
			if v2 && parts[1] != "" || !v2 && !hasController(parts[1], "memory") {
				continue
			}
			root, mount, cg := mountPath(before[3]), filepath.Clean(mountPath(before[4])), parts[2]
			relative, err := filepath.Rel(root, cg)
			// In a private cgroup namespace '/' names the namespace root, even when
			// mountinfo describes the corresponding host-side subtree (or '/..').
			if cg == "/" {
				relative = "."
				err = nil
			}
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || !filepath.IsAbs(mount) {
				continue
			}
			dir := filepath.Join(mount, relative)
			return memoryHierarchy(dir, mount, v2, read)
		}
	}
	return nil, nil
}

func memoryHierarchy(dir, root string, v2 bool, read func(string) ([]byte, error)) (*Capacity, error) {
	limitName, usedName := "memory.limit_in_bytes", "memory.usage_in_bytes"
	if v2 {
		limitName, usedName = "memory.max", "memory.current"
	}
	var effective *Capacity
	leaf := dir
	for {
		// Legacy v1 may disable accounting of descendant groups. Its own
		// limit then says nothing about this child's remaining allocations.
		if !v2 && dir != leaf {
			hierarchy, err := read(filepath.Join(dir, "memory.use_hierarchy"))
			if err != nil || strings.TrimSpace(string(hierarchy)) != "1" {
				break
			}
		}
		raw, err := read(filepath.Join(dir, limitName))
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("容器内存配额读取失败: %w", err)
		}
		if err == nil {
			text := strings.TrimSpace(string(raw))
			if text != "max" || !v2 {
				limit, parseErr := strconv.ParseUint(text, 10, 64)
				if parseErr != nil || limit == 0 {
					return nil, fmt.Errorf("容器内存配额无效")
				}
				// cgroup v1 represents no limit using a page-aligned LONG_MAX sentinel.
				if v2 || limit < 1<<60 {
					usage, err := read(filepath.Join(dir, usedName))
					if err != nil {
						return nil, fmt.Errorf("容器内存用量读取失败: %w", err)
					}
					used, err := strconv.ParseUint(strings.TrimSpace(string(usage)), 10, 64)
					if err != nil {
						return nil, fmt.Errorf("容器内存用量无效")
					}
					available := uint64(0)
					if used < limit {
						available = limit - used
					}
					if effective == nil {
						effective = &Capacity{TotalBytes: limit, AvailableBytes: available}
					} else {
						if limit < effective.TotalBytes {
							effective.TotalBytes = limit
						}
						if available < effective.AvailableBytes {
							effective.AvailableBytes = available
						}
					}
				}
			}
		}
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("容器内存层级路径无效")
		}
		dir = parent
	}
	if effective != nil && effective.AvailableBytes > effective.TotalBytes {
		effective.AvailableBytes = effective.TotalBytes
	}
	return effective, nil
}
