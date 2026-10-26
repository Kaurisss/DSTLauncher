//go:build linux

package hoststats

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func readCgroupFixture(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		if v, ok := files[path]; ok {
			return []byte(v), nil
		}
		return nil, os.ErrNotExist
	}
}

func TestCgroupV2HonorsParentHeadroomAndNestedNamespace(t *testing.T) {
	const mounts = "42 1 0:3 / /sys/fs/cgroup rw - cgroup2 cgroup rw"
	files := map[string]string{
		"/sys/fs/cgroup/team/app/memory.max": "4096", "/sys/fs/cgroup/team/app/memory.current": "1024",
		"/sys/fs/cgroup/team/memory.max": "8192", "/sys/fs/cgroup/team/memory.current": "7680",
		"/sys/fs/cgroup/memory.max": "max",
	}
	got, err := containerMemory("0::/team/app", mounts, readCgroupFixture(files))
	if err != nil || got == nil || got.TotalBytes != 4096 || got.AvailableBytes != 512 {
		t.Fatalf("parent pressure: %+v %v", got, err)
	}
	files["/sys/fs/cgroup/team/app/memory.current"] = "5000"
	got, err = containerMemory("0::/team/app", mounts, readCgroupFixture(files))
	if err != nil || got.AvailableBytes != 0 {
		t.Fatalf("over quota must clamp to zero: %+v %v", got, err)
	}
	files = map[string]string{"/sys/fs/cgroup/memory.max": "2048", "/sys/fs/cgroup/memory.current": "512"}
	got, err = containerMemory("0::/", "42 1 0:3 /.. /sys/fs/cgroup rw - cgroup2 cgroup rw", readCgroupFixture(files))
	if err != nil || got == nil || got.TotalBytes != 2048 || got.AvailableBytes != 1536 {
		t.Fatalf("private namespace: %+v %v", got, err)
	}
}

func TestCgroupV1SubtreeMountAndUnlimitedSentinel(t *testing.T) {
	const mounts = "42 1 0:3 /docker /sys/fs/cgroup/memory rw - cgroup cgroup rw,memory"
	files := map[string]string{
		"/sys/fs/cgroup/memory/app/memory.limit_in_bytes": "1024", "/sys/fs/cgroup/memory/app/memory.usage_in_bytes": "256",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes": "9223372036854771712",
	}
	got, err := containerMemory("7:cpu,cpuacct:/other\n8:memory:/docker/app", mounts, readCgroupFixture(files))
	if err != nil || got == nil || got.TotalBytes != 1024 || got.AvailableBytes != 768 {
		t.Fatalf("v1: %+v %v", got, err)
	}
	files["/sys/fs/cgroup/memory/app/memory.limit_in_bytes"] = "9223372036854771712"
	delete(files, "/sys/fs/cgroup/memory/app/memory.usage_in_bytes")
	got, err = containerMemory("8:memory:/docker/app", mounts, readCgroupFixture(files))
	if err != nil || got != nil {
		t.Fatalf("unlimited: %+v %v", got, err)
	}
}

func TestCgroupMissingLimitsAndMalformedUsage(t *testing.T) {
	mounts := "42 1 0:3 / /sys/fs/cgroup rw - cgroup2 cgroup rw"
	for _, limit := range []string{"max", "", "0", "-1", "18446744073709551616", "bad"} {
		t.Run(fmt.Sprintf("limit-%s", limit), func(t *testing.T) {
			files := map[string]string{"/sys/fs/cgroup/memory.max": limit, "/sys/fs/cgroup/memory.current": "256"}
			got, err := containerMemory("0::/", mounts, readCgroupFixture(files))
			if limit == "max" {
				if err != nil || got != nil {
					t.Fatal(got, err)
				}
			} else if err == nil {
				t.Fatal("invalid limit silently accepted")
			}
		})
	}
	for _, usage := range []string{"nope", "-1", ""} {
		_, err := containerMemory("0::/", mounts, readCgroupFixture(map[string]string{"/sys/fs/cgroup/memory.max": "1024", "/sys/fs/cgroup/memory.current": usage}))
		if err == nil {
			t.Fatal("invalid usage silently accepted")
		}
	}
	got, err := containerMemory("0::/", mounts, readCgroupFixture(nil))
	if err != nil || got != nil {
		t.Fatal("missing controller is optional", got, err)
	}
	got, err = containerMemory("0::/outside", "42 1 0:3 /docker /sys/fs/cgroup rw - cgroup2 cgroup rw", func(path string) ([]byte, error) { t.Fatalf("escaped mount root: %s", path); return nil, nil })
	if err != nil || got != nil {
		t.Fatal(got, err)
	}
}

func TestCgroupMountEscapesAndUnavailableUsage(t *testing.T) {
	files := map[string]string{"/sys/has space/memory.max": "512"}
	_, err := containerMemory("0::/", `42 1 0:3 / /sys/has\040space rw - cgroup2 cgroup rw`, readCgroupFixture(files))
	if err == nil || !strings.Contains(err.Error(), "用量") {
		t.Fatalf("known limit with unknown usage must not look ready: %v", err)
	}
}

func TestCgroupV1ParentLimitsRequireHierarchicalAccounting(t *testing.T) {
	files := map[string]string{
		"/sys/memory/app/memory.limit_in_bytes": "4096", "/sys/memory/app/memory.usage_in_bytes": "1024",
		"/sys/memory/memory.limit_in_bytes": "8192", "/sys/memory/memory.usage_in_bytes": "8000",
		"/sys/memory/memory.use_hierarchy": "0",
	}
	const mounts = "42 1 0:3 / /sys/memory rw - cgroup cgroup rw,memory"
	got, err := containerMemory("8:memory:/app", mounts, readCgroupFixture(files))
	if err != nil || got.AvailableBytes != 3072 {
		t.Fatal("non-hierarchical parent should not limit child", got, err)
	}
	files["/sys/memory/memory.use_hierarchy"] = "1"
	got, err = containerMemory("8:memory:/app", mounts, readCgroupFixture(files))
	if err != nil || got.AvailableBytes != 192 {
		t.Fatal("hierarchical parent should constrain availability", got, err)
	}
}
