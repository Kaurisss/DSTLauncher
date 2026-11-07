package room

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateBatchAllocatesAgainstPendingAndRollsBack(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "rooms.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Create(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	templates := []*Room{Defaults(), Defaults()}
	created, err := s.CreateBatch(templates, func(rooms []*Room) error {
		if rooms[0].ID == rooms[1].ID || rooms[0].Shards[0].ServerPort == rooms[1].Shards[0].ServerPort || rooms[0].ClusterKey == rooms[1].ClusterKey {
			t.Fatal("批量房间分配冲突")
		}
		if rooms[0].Shards[0].ServerPort == first.Shards[0].ServerPort {
			t.Fatal("与现有房间冲突")
		}
		return nil
	}, func() error { t.Fatal("成功时回滚"); return nil })
	if err != nil || len(created) != 2 {
		t.Fatal(created, err)
	}
	_, err = s.CreateBatch(templates, func([]*Room) error { return fmt.Errorf("模拟准备失败") }, func() error { return nil })
	if err == nil || len(s.List()) != 3 {
		t.Fatal("部分房间被发布", err, s.List())
	}
	reloaded, err := NewStore(s.path)
	if err != nil || len(reloaded.List()) != 3 {
		t.Fatal(err)
	}
	s.path = filepath.Join(t.TempDir(), "occupied")
	if err := os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	rolledBack := false
	_, err = s.CreateBatch(templates, func([]*Room) error { return nil }, func() error { rolledBack = true; return nil })
	if err == nil || !rolledBack || len(s.List()) != 3 {
		t.Fatal("持久化失败未回滚", err)
	}
}
