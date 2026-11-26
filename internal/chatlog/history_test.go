package chatlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const identityFixture = "[00:00:00]: Current time: Sun Oct 25 17:55:12 2026\n" +
	"[00:01:34]: Client connected from [LAN] 203.0.113.10|57005 <101>\n" +
	"[00:01:34]: Client authenticated: (KU_A-1) 玩家 有空格\n" +
	"[00:01:38]: [ClientObject] Initialized (authenticated) on server: guid=101 userid=KU_A-1 netid=76561198000000001 admin=0\n" +
	"[00:02:06]: User ID\tKU_A-1\tassigned ownership to entity\t119794 - wolfgang\n"

func writeGameLog(t *testing.T, cluster, shard, file, content string) string {
	t.Helper()
	dir := filepath.Join(cluster, shard)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendGameLog(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(content)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("append: %v %v", err, closeErr)
	}
}

func TestHistoryImportsIdentityEventsAndPersists(t *testing.T) {
	cluster, historyRoot := filepath.Join(t.TempDir(), "Cluster_1"), t.TempDir()
	writeGameLog(t, cluster, "Surface", "server_log.txt", identityFixture)
	chat := "[00:02:05]: [Join Announcement] 玩家 有空格\n" +
		"[00:02:10]: [Death Announcement] 玩家 有空格 死于： 猎犬。他变成了鬼魂！\n" +
		"[00:02:11]: [Resurrect Announcement] 玩家 有空格 复活自： 骷髅.\n" +
		"[00:02:12]: [Say] (KU_A-1) 玩家 有空格: hello <img src=x>\n" +
		"[00:04:00]: [Leave Announcement] 玩家 有空格\n"
	chatPath := writeGameLog(t, cluster, "Surface", "server_chat_log.txt", chat)
	h := NewHistory(historyRoot)
	r, err := h.Query(cluster, "Surface", Query{Kind: "all", Limit: 2, Page: 1})
	if err != nil || r.Total != 5 || len(r.Entries) != 2 || r.Truncated || r.PageSize != 2 {
		t.Fatalf("first page: %+v %v", r, err)
	}
	for _, e := range r.Entries {
		if e.UserID != "KU_A-1" || e.SteamID != "76561198000000001" || e.IP != "203.0.113.10" || e.Prefab != "wolfgang" || e.CollectedAt.IsZero() {
			t.Fatalf("missing identity: %+v", e)
		}
	}
	start, _ := time.ParseInLocation(time.ANSIC, "Sun Oct 25 17:55:12 2026", time.Local)
	if r.Entries[0].RecordedAt == nil || !r.Entries[0].RecordedAt.Equal(start.Add(4*time.Minute)) {
		t.Fatalf("event time must come from game startup: %+v", r.Entries[0])
	}
	r, err = h.Query(cluster, "Surface", Query{Kind: "all", Limit: 2, Page: 3})
	if err != nil || len(r.Entries) != 1 || r.Entries[0].Kind != "join" || r.Entries[0].ID != 1 {
		t.Fatalf("last page: %+v %v", r, err)
	}
	for _, kind := range []string{"death", "resurrect"} {
		r, err = h.Query(cluster, "Surface", Query{Kind: kind, Limit: 10, Page: 1, Search: "ku_a-1"})
		if err != nil || r.Total != 1 || r.Entries[0].UserID != "KU_A-1" || r.Entries[0].Name != "玩家 有空格" {
			t.Fatalf("%s: %+v %v", kind, r, err)
		}
	}
	reopened := NewHistory(historyRoot)
	r, err = reopened.Query(cluster, "Surface", Query{Kind: "all", Limit: 10, Page: 1, Search: "76561198000000001"})
	if err != nil || r.Total != 5 || r.Entries[0].ID != 5 || r.Entries[4].ID != 1 {
		t.Fatalf("restart must preserve history and IDs: %+v %v", r, err)
	}
	data, err := os.ReadFile(chatPath)
	if err != nil || string(data) != chat {
		t.Fatal("history collection modified the game log", err)
	}
}

func TestHistoryBackfillsLateIdentityAndDoesNotGuessDuplicateNames(t *testing.T) {
	cluster := filepath.Join(t.TempDir(), "Cluster_1")
	writeGameLog(t, cluster, "Surface", "server_chat_log.txt", "[00:02:05]: [Join Announcement] 玩家 有空格\n")
	h := NewHistory(t.TempDir())
	q := Query{Kind: "all", Limit: 20, Page: 1}
	r, err := h.Query(cluster, "Surface", q)
	if err != nil || r.Entries[0].UserID != "" || r.Entries[0].RecordedAt != nil {
		t.Fatalf("must not fabricate identity or date: %+v %v", r, err)
	}
	path := writeGameLog(t, cluster, "Surface", "server_log.txt", identityFixture)
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Entries[0].UserID != "KU_A-1" || r.Entries[0].ID != 1 {
		t.Fatalf("late identity should fill the same event: %+v %v", r, err)
	}
	appendGameLog(t, path, "[00:03:00]: Client authenticated: (KU_B) 玩家 有空格\n")
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Entries[0].UserID != "" || r.Entries[0].SteamID != "" {
		t.Fatalf("duplicate names must not enable a ban against the wrong account: %+v %v", r, err)
	}
	appendGameLog(t, filepath.Join(cluster, "Surface", "server_chat_log.txt"), "[00:04:00]: [Whisper] (KU_A-1) 玩家 有空格: private\n")
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Entries[0].UserID != "KU_A-1" || r.Entries[0].SteamID == "" || r.Entries[1].UserID != "" {
		t.Fatalf("an explicit chat ID must survive duplicate names: %+v %v", r, err)
	}
}

func TestHistoryConnectionGUIDsRoleSourcesAndExactNames(t *testing.T) {
	cluster := filepath.Join(t.TempDir(), "Cluster_1")
	writeGameLog(t, cluster, "Surface", "server_log.txt", "[00:00:01]: Client connected from 203.0.113.1|1000 <101>\n"+
		"[00:00:01]: Client connected from [LAN] [2001:db8::2]|2000 <202>\n"+
		"[00:00:02]: Client authenticated: (KU_A) Alice A\n"+
		"[00:00:02]: Client authenticated: (KU_B) Alice B\n"+
		"[00:00:03]: [ClientObject] Initialized (authenticated) on server: guid=202 userid=KU_B netid=76561198000000002 admin=0\n"+
		"[00:00:03]: [ClientObject] Initialized (authenticated) on server: guid=101 userid=KU_A netid=76561198000000001 admin=0\n"+
		"[00:00:04]: Spawn request: willow from Alice A\n"+
		"[00:00:04]: DST_PANEL_PLAYERS_aabb:{\"player\":{\"userid\":\"KU_B\",\"name\":\"Alice B\",\"prefab\":\"wendy\"}}\n")
	writeGameLog(t, cluster, "Surface", "server_chat_log.txt", "[00:00:05]: [Join Announcement] Alice A\n[00:00:06]: [Join Announcement] Alice B\n[00:00:07]: [Join Announcement] Alice\n")
	r, err := NewHistory(t.TempDir()).Query(cluster, "Surface", Query{Kind: "all", Limit: 20, Page: 1})
	if err != nil || r.Total != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Entries[0].UserID != "" {
		t.Fatal("nickname substring must not match another player")
	}
	b, a := r.Entries[1], r.Entries[2]
	if b.UserID != "KU_B" || b.IP != "2001:db8::2" || b.Prefab != "wendy" || a.UserID != "KU_A" || a.IP != "203.0.113.1" || a.Prefab != "willow" {
		t.Fatalf("interleaved connections were mixed: A=%+v B=%+v", a, b)
	}
}

func TestHistoryPreservesEarlierRolesAndAddresses(t *testing.T) {
	cluster := filepath.Join(t.TempDir(), "Cluster_1")
	writeGameLog(t, cluster, "Surface", "server_log.txt", identityFixture+
		"[00:04:00]: Client connected from 203.0.113.11|57006 <102>\n"+
		"[00:04:01]: [ClientObject] Initialized (authenticated) on server: guid=102 userid=KU_A-1 netid=76561198000000001 admin=0\n"+
		"[00:05:00]: User ID KU_A-1 assigned ownership to entity 120001 - willow\n")
	writeGameLog(t, cluster, "Surface", "server_chat_log.txt", "[00:02:05]: [Join Announcement] 玩家 有空格\n"+
		"[00:03:00]: [Say] (KU_A-1) 玩家 有空格: old character\n"+
		"[00:06:00]: [Leave Announcement] 玩家 有空格\n")
	r, err := NewHistory(t.TempDir()).Query(cluster, "Surface", Query{Kind: "all", Limit: 10, Page: 1})
	if err != nil || r.Total != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Entries[0].Prefab != "willow" || r.Entries[0].IP != "203.0.113.11" {
		t.Fatalf("new observation missing: %+v", r.Entries[0])
	}
	for _, entry := range r.Entries[1:] {
		if entry.Prefab != "wolfgang" || entry.IP != "203.0.113.10" {
			t.Fatalf("later character or reconnect rewrote an earlier event: %+v", entry)
		}
	}
}

func TestHistoryPartialLinesDeletionRestartAndLogRotation(t *testing.T) {
	cluster, root := filepath.Join(t.TempDir(), "Cluster_1"), t.TempDir()
	serverPath := writeGameLog(t, cluster, "Surface", "server_log.txt", identityFixture)
	chatPath := writeGameLog(t, cluster, "Surface", "server_chat_log.txt", "[00:02:05]: [Join Announcement] 玩家 有空格")
	h := NewHistory(root)
	q := Query{Kind: "all", Limit: 20, Page: 1}
	r, err := h.Query(cluster, "Surface", q)
	if err != nil || !r.Exists || r.Total != 0 {
		t.Fatalf("partial line: %+v %v", r, err)
	}
	appendGameLog(t, chatPath, "\n")
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Total != 1 {
		t.Fatalf("completed line: %+v %v", r, err)
	}
	if count, err := h.Delete(cluster, "Surface", []int64{r.Entries[0].ID}); err != nil || count != 1 {
		t.Fatalf("delete: %d %v", count, err)
	}
	h = NewHistory(root)
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Total != 0 {
		t.Fatalf("deleted history was re-imported: %+v %v", r, err)
	}
	appendGameLog(t, chatPath, "[00:03:00]: [Leave Announcement] 玩家 有空格\n")
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Total != 1 || r.Entries[0].ID != 2 || r.Entries[0].UserID != "KU_A-1" {
		t.Fatalf("append after delete: %+v %v", r, err)
	}
	if err := os.WriteFile(serverPath, []byte("[00:00:00]: Current time: Mon Oct 26 18:00:00 2026\n[00:00:01]: Client authenticated: (KU_NEW) 玩家 有空格\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chatPath, []byte("[00:00:05]: [Join Announcement] 玩家 有空格\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changedAt := time.Date(2026, 10, 26, 18, 0, 5, 0, time.Local)
	if err := os.Chtimes(chatPath, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	r, err = h.Query(cluster, "Surface", q)
	if err != nil || r.Total != 2 || r.Entries[0].ID != 3 || r.Entries[0].UserID != "KU_NEW" || r.Entries[0].SteamID != "" || r.Entries[1].UserID != "KU_A-1" {
		t.Fatalf("rotation must archive old events without reusing old identity: %+v %v", r, err)
	}
	r, err = NewHistory(root).Query(cluster, "Surface", q)
	if err != nil || r.Total != 2 {
		t.Fatalf("rotation replayed after panel restart: %+v %v", r, err)
	}
}

func TestHistoryScopesPaginationValidationAndConcurrentReads(t *testing.T) {
	base := t.TempDir()
	h := NewHistory(t.TempDir())
	q := Query{Kind: "all", Limit: 10, Page: 1}
	for _, room := range []string{"Cluster_1", "Cluster_2"} {
		for _, shard := range []string{"Surface", "Below"} {
			writeGameLog(t, filepath.Join(base, room), shard, "server_chat_log.txt", "[00:00:01]: [Announcement] "+room+" "+shard+"\n")
		}
	}
	cluster := filepath.Join(base, "Cluster_1")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := h.Query(cluster, "Surface", q)
			if err != nil || r.Total != 1 || r.Entries[0].ID != 1 {
				t.Errorf("concurrent query: %+v %v", r, err)
			}
		}()
	}
	wg.Wait()
	if _, err := h.Delete(cluster, "Surface", []int64{1}); err != nil {
		t.Fatal(err)
	}
	for _, scope := range [][2]string{{"Cluster_1", "Below"}, {"Cluster_2", "Surface"}} {
		r, err := h.Query(filepath.Join(base, scope[0]), scope[1], q)
		if err != nil || r.Total != 1 || !strings.Contains(r.Entries[0].Message, scope[0]) {
			t.Fatalf("cross-scope deletion: %+v %v", r, err)
		}
	}
	for _, invalid := range []Query{{Kind: "all", Limit: 0}, {Kind: "all", Limit: 501}, {Kind: "bad", Limit: 10}, {Kind: "all", Limit: 10, Page: -1}} {
		if _, err := h.Query(cluster, "Surface", invalid); err == nil {
			t.Fatalf("accepted query: %+v", invalid)
		}
	}
	for _, ids := range [][]int64{nil, {0}, {-1}, make([]int64, 501)} {
		if _, err := h.Delete(cluster, "Surface", ids); err == nil {
			t.Fatalf("accepted IDs: %+v", ids)
		}
	}
	for _, shard := range []string{"", "..", "../Surface", "Surface/../Below"} {
		if err := h.Sync(cluster, shard); err == nil {
			t.Fatalf("accepted path: %s", shard)
		}
	}
	r, err := h.Query(cluster, "Below", Query{Kind: "all", Limit: 10, Page: int(^uint(0) >> 1)})
	if err != nil || len(r.Entries) != 0 || r.Total != 1 {
		t.Fatalf("out of range page: %+v %v", r, err)
	}
}

func TestHistoryNeverOverwritesUnreadableArchivesOrReadsLogLinks(t *testing.T) {
	cluster, root := filepath.Join(t.TempDir(), "Cluster_1"), t.TempDir()
	writeGameLog(t, cluster, "Surface", "server_chat_log.txt", "[00:00:01]: [Announcement] safe\n")
	h := NewHistory(root)
	path := h.statePath(cluster, "Surface")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.Sync(cluster, "Surface"); err == nil {
		t.Fatal("corrupt archive was silently replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken archive" {
		t.Fatal("corrupt archive changed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	external := writeGameLog(t, t.TempDir(), "Private", "secret.log", "[00:00:01]: Client authenticated: (KU_secret) private\n")
	link := filepath.Join(cluster, "Surface", "server_log.txt")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := h.Sync(cluster, "Surface"); err == nil {
		t.Fatal("followed external server log symlink")
	}
}

func TestHistoryKeepsMoreThanLegacyTailAndSearchesAllArchivedRows(t *testing.T) {
	cluster := filepath.Join(t.TempDir(), "Cluster_1")
	var content strings.Builder
	for i := range 260 {
		fmt.Fprintf(&content, "[00:00:01]: [Say] (KU_A) A: message-%03d\n", i)
	}
	path := writeGameLog(t, cluster, "Surface", "server_chat_log.txt", content.String())
	h := NewHistory(t.TempDir())
	q := Query{Kind: "all", Limit: 10, Page: 26}
	r, err := h.Query(cluster, "Surface", q)
	if err != nil || r.Total != 260 || len(r.Entries) != 10 || r.Entries[9].Message != "message-000" {
		t.Fatalf("history pagination: %+v %v", r, err)
	}
	if err := os.WriteFile(path, []byte("[00:00:01]: [Announcement] new session\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err = h.Query(cluster, "Surface", Query{Kind: "chat", Limit: 10, Page: 1, Search: "message-000"})
	if err != nil || r.Total != 1 || r.Entries[0].ID != 1 {
		t.Fatalf("rotated log erased history search: %+v %v", r, err)
	}
}

func TestFacesKeepsLastKnownNameAfterHistoryTrim(t *testing.T) {
	state := &historyState{
		StartedAt: time.Date(2026, time.November, 26, 12, 0, 0, 0, time.UTC),
		Players: map[string]identity{
			"KU_PLAYER": {
				Names:          map[string]bool{"旧名字": true, "新名字": true},
				LastName:       "新名字",
				LastNameAt:     time.Hour,
				LastNameDirect: true,
			},
		},
		Entries: []storedEntry{},
	}

	faces := state.faces()
	if faces["ku_player"].Name != "新名字" {
		t.Fatalf("历史裁剪后应保留最后名称: %+v", faces)
	}
}

func TestFacesKeepsLatestNameAndCharacterAcrossShards(t *testing.T) {
	cluster := filepath.Join(t.TempDir(), "Cluster_1")
	start := time.Now().Add(-2 * time.Hour)
	surface := fmt.Sprintf("[00:00:00]: Current time: %s\n", start.Format(time.ANSIC)) +
		"[00:01:00]: Client authenticated: (KU_A-1) 旧名字\n" +
		"[00:01:10]: Client authenticated: (KU_B) 甲\n" +
		"[00:01:20]: Client authenticated: (KU_B) 乙\n" +
		"[00:01:30]: User ID KU_B assigned ownership to entity 3 - wx78\n" +
		"[00:01:40]: Client authenticated: (KU_C) 丙\n" +
		"[00:02:00]: User ID KU_A-1 assigned ownership to entity 1 - willow\n"
	cavesAt := start.Add(-time.Hour)
	caves := fmt.Sprintf("[00:00:00]: Current time: %s\n", cavesAt.Format(time.ANSIC)) +
		"[00:01:00]: Client authenticated: (KU_A-1) 洞穴名\n" +
		"[00:02:00]: User ID KU_A-1 assigned ownership to entity 2 - wendy\n"
	writeGameLog(t, cluster, "Surface", "server_log.txt", surface)
	writeGameLog(t, cluster, "Surface", "server_chat_log.txt", "[00:03:00]: [Say] (KU_A-1) 旧名字: hi\n[00:04:00]: [Say] (KU_A-1) 新名字: hi\n")
	writeGameLog(t, cluster, "Caves", "server_log.txt", caves)
	faces, err := NewHistory(t.TempDir()).Faces(cluster, []string{"Surface", "Caves"})
	if err != nil {
		t.Fatal(err)
	}
	alice := faces["ku_a-1"]
	if alice.ID != "KU_A-1" || alice.Name != "新名字" || alice.Prefab != "willow" {
		t.Fatalf("latest face: %+v", alice)
	}
	if faces["ku_b"].Name != "" || faces["ku_b"].Prefab != "wx78" {
		t.Fatalf("ambiguous name: %+v", faces["ku_b"])
	}
	if faces["ku_c"].Name != "丙" || faces["ku_c"].Prefab != "" {
		t.Fatalf("single name: %+v", faces["ku_c"])
	}
}
