package hoststats

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHostAndSaveDisk(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "未创建的存档", "DST")
	got := Read(missing)
	if got.MemoryError != "" || got.DiskError != "" {
		t.Fatalf("采样失败: %+v", got)
	}
	for name, capacity := range map[string]*Capacity{"memory": got.Memory, "disk": got.Disk} {
		if capacity == nil || capacity.TotalBytes == 0 || capacity.AvailableBytes > capacity.TotalBytes {
			t.Fatalf("%s 容量无效: %+v", name, capacity)
		}
	}
	if got.DiskPath != missing {
		t.Fatalf("应保留实际配置路径: %s", got.DiskPath)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("采样不应创建存档目录: %v", err)
	}
}

func TestDiskFailureDoesNotHideMemory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	got := Read(file)
	if got.Disk != nil || got.DiskError == "" {
		t.Fatalf("非目录应报告磁盘采样错误: %+v", got)
	}
	if got.Memory == nil || got.MemoryError != "" {
		t.Fatalf("磁盘失败不应影响内存: %+v", got)
	}
}

func TestReadDiskRejectsMissingDirectory(t *testing.T) {
	if _, err := readDisk(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("系统调用应返回路径错误")
	}
}
