package server

import (
	"testing"
	"time"
)

func TestStatusSubscriptionSurvivesLogFlood(t *testing.T) {
	m := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
	id, events := m.SubscribeEvents("crash_limit", "update_failed")
	defer m.Unsubscribe(id)
	for i := 0; i < 2000; i++ {
		m.emit("Cluster_1", "Master", "log", "loading")
	}
	m.emit("Cluster_1", "Master", "crash_limit", "达到重启上限")
	select {
	case event := <-events:
		if event.Type != "crash_limit" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("大量日志挤掉了关键状态通知")
	}
	m.setModErr("下载 workshop-123456 失败")
	select {
	case event := <-events:
		if event.Type != "update_failed" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("模组更新失败未发出事件")
	}
}
