package server

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkshopDirectoryUsedForLoadingAndDownloading(t *testing.T) {
	manager := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
	workshop := filepath.Join(t.TempDir(), "Steam 库", "steamapps", "workshop")
	manager.SetUgcDir(workshop)
	cluster := &Cluster{ID: 2, ConfDir: "DST"}
	args := manager.baseArgs(cluster, "Caves")
	got := ""
	for i, arg := range args {
		if arg == "-ugc_directory" {
			got = args[i+1]
		}
	}
	if got != workshop || manager.UgcDir() != workshop {
		t.Fatalf("分片和下载进程应使用指定工坊: %v", args)
	}
	if got := manager.targetModDir("123"); got != filepath.Join(workshop, "content", "322330", "123") {
		t.Fatalf("下载目标错误: %s", got)
	}
}

func TestRedownloadPreservesOldModOnFailure(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			manager := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
			manager.SetUgcDir(t.TempDir())
			target := manager.targetModDir("123")
			if err := os.MkdirAll(target, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "modinfo.lua"), []byte("old"), 0644); err != nil {
				t.Fatal(err)
			}
			manager.SetModsSetupSwap(func(string) (func(), error) {
				if !success {
					return nil, fmt.Errorf("模拟下载清单失败")
				}
				// Simulate a completed replacement without launching a real game.
				if err := os.MkdirAll(target, 0755); err != nil {
					return nil, err
				}
				if err := os.WriteFile(filepath.Join(target, "modinfo.lua"), []byte("new"), 0644); err != nil {
					return nil, err
				}
				return func() {}, nil
			})
			log := func(string, ...interface{}) {}
			got := manager.runSingleTarget(nil, "", "", "", "123", log, func(string) {})
			if got != success {
				t.Fatalf("下载结果错误: %v", got)
			}
			data, err := os.ReadFile(filepath.Join(target, "modinfo.lua"))
			want := "old"
			if success {
				want = "new"
			}
			if err != nil || string(data) != want {
				t.Fatalf("成功应替换，失败应保留旧文件: %q, %v", data, err)
			}
			entries, err := os.ReadDir(filepath.Dir(target))
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if !reflect.DeepEqual(names, []string{"123"}) {
				t.Fatalf("正常恢复/替换后不应残留备份: %v", names)
			}
		})
	}
}
