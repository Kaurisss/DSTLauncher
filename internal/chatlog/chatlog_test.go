package chatlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chatFixture(t *testing.T, content string) (string, string) {
	t.Helper()
	cluster := t.TempDir()
	dir := filepath.Join(cluster, "Surface")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "server_chat_log.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return cluster, path
}

func TestChatLogTypesFilteringAndFileOrder(t *testing.T) {
	h := NewHistory(t.TempDir())
	cluster, _ := chatFixture(t, "[00:00:01]: startup\n[00:00:02]: [Join Announcement] 张三\r\n[00:00:03]: [Say] (KU_A-1) 张三: 你好: 世界 <img src=x>\n[00:00:04]: [Whisper] (KU_B_2) Alice: private\n[00:00:05]: [Leave Announcement] 张三\n[00:00:06]: [Announcement] 欢迎\n[00:00:01]: [Say] (KU_C) restart: new session\n")
	result, err := h.Query(cluster, "Surface", Query{Limit: 500, Kind: "all"})
	if err != nil || !result.Exists || len(result.Entries) != 6 {
		t.Fatalf("%+v %v", result, err)
	}
	if result.Entries[0].Name != "restart" || result.Entries[0].Time != "00:00:01" {
		t.Fatal("must use file order, not game timestamps")
	}
	e := result.Entries[4]
	if e.Name != "张三" || e.UserID != "KU_A-1" || e.Message != "你好: 世界 <img src=x>" {
		t.Fatalf("chat content not preserved: %+v", e)
	}
	filtered, err := h.Query(cluster, "Surface", Query{Limit: 1, Kind: "all", Search: "张三"})
	if err != nil || !filtered.Truncated || len(filtered.Entries) != 1 || filtered.Entries[0].Kind != "leave" {
		t.Fatalf("filters: %+v %v", filtered, err)
	}
	filtered, err = h.Query(cluster, "Surface", Query{Limit: 20, Kind: "chat", Search: "ku_a-1"})
	if err != nil || len(filtered.Entries) != 1 || filtered.Entries[0].Kind != "chat" {
		t.Fatalf("case-insensitive ID filter: %+v %v", filtered, err)
	}
}

func TestChatLogLargeSourcePartialLinesAndReplacement(t *testing.T) {
	h := NewHistory(t.TempDir())
	cluster, path := chatFixture(t, strings.Repeat("x", 1024*1024+17)+"\n[99:00:01]: [Say] (KU_A) A: complete\n[99:00:02]: [Say] (KU_A) A: incomplete")
	q := Query{Limit: 200, Kind: "all"}
	r, err := h.Query(cluster, "Surface", q)
	if err != nil || r.Total != 1 || len(r.Entries) != 1 || r.Entries[0].Message != "complete" {
		t.Fatalf("tail/partial: %+v %v", r, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || len(r.Entries) != 2 || r.Entries[0].Message != "incomplete" {
		t.Fatalf("append: %+v %v", r, err)
	}
	if err := os.WriteFile(path, []byte("[00:00:01]: [Announcement] replaced\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Total != 3 || r.Entries[0].Message != "replaced" {
		t.Fatalf("replacement: %+v %v", r, err)
	}
}

func TestChatLogMissingInvalidScopeAndLinks(t *testing.T) {
	h := NewHistory(t.TempDir())
	cluster := t.TempDir()
	q := Query{Limit: 200, Kind: "all"}
	r, err := h.Query(cluster, "Surface", q)
	if err != nil || r.Exists || r.Entries == nil {
		t.Fatalf("missing: %+v %v", r, err)
	}
	for _, shard := range []string{"..", "../elsewhere", "Surface/../outside", "C:\\logs"} {
		if _, err := h.Query(cluster, shard, q); err == nil {
			t.Fatalf("unsafe shard %q", shard)
		}
	}
	for _, bad := range []Query{{Limit: 0, Kind: "all"}, {Limit: 501, Kind: "all"}, {Limit: 2, Kind: "unknown"}, {Limit: 2, Kind: "all", Search: strings.Repeat("你", 201)}} {
		if _, err := h.Query(cluster, "Surface", bad); err == nil {
			t.Fatalf("invalid query %+v", bad)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(cluster, "Escaped")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := h.Query(cluster, "Escaped", q); err == nil {
		t.Fatal("followed external shard symlink")
	}
	if err := os.Mkdir(filepath.Join(cluster, "Surface"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "private.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "private.txt"), filepath.Join(cluster, "Surface", "server_chat_log.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Query(cluster, "Surface", q); err == nil {
		t.Fatal("followed log file symlink")
	}
}

func TestChatLogLimitRetainsLatestMatchingRows(t *testing.T) {
	h := NewHistory(t.TempDir())
	var b strings.Builder
	for i := range 600 {
		fmt.Fprintf(&b, "[00:00:01]: [Say] (KU_A) A: %d\n", i)
	}
	cluster, _ := chatFixture(t, b.String())
	r, err := h.Query(cluster, "Surface", Query{Limit: 500, Kind: "chat"})
	if err != nil || len(r.Entries) != 500 || r.Entries[0].Message != "599" || r.Entries[499].Message != "100" {
		t.Fatalf("limit: %d %v", len(r.Entries), err)
	}
}
