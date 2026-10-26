package chatlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryNewSessionImportsIdenticalAnnouncementsOnlyAfterChatIsRecreated(t *testing.T) {
	cluster, root := filepath.Join(t.TempDir(), "Cluster_1"), t.TempDir()
	oldStart := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	newStart := oldStart.Add(time.Hour)
	serverPath := writeGameLog(t, cluster, "Surface", "server_log.txt", fmt.Sprintf("[00:00:00]: Current time: %s\n[00:00:01]: Client authenticated: (KU_OLD) Same Name\n", oldStart.Format(time.ANSIC)))
	announcement := "[00:00:05]: [Join Announcement] Same Name\n"
	chatPath := writeGameLog(t, cluster, "Surface", "server_chat_log.txt", announcement)
	oldModified := oldStart.Add(5 * time.Second)
	if err := os.Chtimes(chatPath, oldModified, oldModified); err != nil {
		t.Fatal(err)
	}
	h := NewHistory(root)
	q := Query{Kind: "all", Limit: 10, Page: 1}
	first, err := h.Query(cluster, "Surface", q)
	if err != nil || first.Total != 1 || first.Entries[0].UserID != "KU_OLD" {
		t.Fatalf("%+v %v", first, err)
	}
	if err := os.WriteFile(serverPath, []byte(fmt.Sprintf("[00:00:00]: Current time: %s\n[00:00:01]: Client authenticated: (KU_NEW) Same Name\n", newStart.Format(time.ANSIC))), 0600); err != nil {
		t.Fatal(err)
	}
	waiting, err := h.Query(cluster, "Surface", q)
	if err != nil || waiting.Total != 1 {
		t.Fatalf("old chat file was imported as new session: %+v %v", waiting, err)
	}
	// The new game can emit exactly the same nickname and uptime as its previous run.
	if err := os.WriteFile(chatPath, []byte(announcement), 0600); err != nil {
		t.Fatal(err)
	}
	newModified := newStart.Add(5 * time.Second)
	if err := os.Chtimes(chatPath, newModified, newModified); err != nil {
		t.Fatal(err)
	}
	result, err := h.Query(cluster, "Surface", q)
	if err != nil || result.Total != 2 || result.Entries[0].UserID != "KU_NEW" || result.Entries[1].UserID != "KU_OLD" {
		t.Fatalf("new session lost an identical announcement: %+v %v", result, err)
	}
	result, err = NewHistory(root).Query(cluster, "Surface", q)
	if err != nil || result.Total != 2 {
		t.Fatalf("restart replayed a session: %+v %v", result, err)
	}
}

func TestHistoryReplacementWithUnchangedPrefixStillImportsNewTail(t *testing.T) {
	cluster := filepath.Join(t.TempDir(), "Cluster_1")
	padding := strings.Repeat("[00:00:00]: ignored\n", 400)
	path := writeGameLog(t, cluster, "Surface", "server_chat_log.txt", padding+"[00:00:01]: [Say] (KU_A) A: first\n")
	h := NewHistory(t.TempDir())
	q := Query{Kind: "all", Limit: 10, Page: 1}
	if _, err := h.Query(cluster, "Surface", q); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(padding+"[00:00:01]: [Say] (KU_A) A: different tail in replacement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := h.Query(cluster, "Surface", q)
	if err != nil || result.Total != 2 || result.Entries[0].Message != "different tail in replacement" {
		t.Fatalf("unchanged prefix hid a replacement: %+v %v", result, err)
	}
}

func TestHistorySpawnRolePrecedesLaterOwnershipRole(t *testing.T) {
	cluster := filepath.Join(t.TempDir(), "Cluster_1")
	serverPath := writeGameLog(t, cluster, "Surface", "server_log.txt", "[00:00:01]: Client authenticated: (KU_A) Alice\n[00:00:02]: Spawn request: willow from Alice\n")
	chatPath := writeGameLog(t, cluster, "Surface", "server_chat_log.txt", "[00:00:03]: [Say] (KU_A) Alice: early character\n")
	h := NewHistory(t.TempDir())
	q := Query{Kind: "all", Limit: 10, Page: 1}
	result, err := h.Query(cluster, "Surface", q)
	if err != nil || result.Entries[0].Prefab != "willow" {
		t.Fatalf("%+v %v", result, err)
	}
	appendGameLog(t, serverPath, "[00:00:10]: User ID KU_A assigned ownership to entity 123 - wendy\n")
	appendGameLog(t, chatPath, "[00:00:11]: [Say] (KU_A) Alice: new character\n")
	result, err = h.Query(cluster, "Surface", q)
	if err != nil || result.Entries[0].Prefab != "wendy" || result.Entries[1].Prefab != "willow" {
		t.Fatalf("later ownership rewrote a spawn-era event: %+v %v", result, err)
	}
}

func TestHistoryAnnouncementNamesCanContainDeathAndResurrectionPhrases(t *testing.T) {
	for _, scenario := range []struct{ name, kind, description string }{
		{"Alice 死于： Bob", "Death", "死于： 猎犬"},
		{"Alice was killed by Bob", "Death", "was killed by a hound"},
		{"Alice 复活自： Bob", "Resurrect", "复活自： 骷髅"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			cluster := filepath.Join(t.TempDir(), "Cluster_1")
			writeGameLog(t, cluster, "Surface", "server_log.txt", fmt.Sprintf("[00:00:01]: Client authenticated: (KU_ALICE) Alice\n[00:00:02]: Client authenticated: (KU_ACTUAL) %s\n", scenario.name))
			writeGameLog(t, cluster, "Surface", "server_chat_log.txt", fmt.Sprintf("[00:00:03]: [%s Announcement] %s %s\n", scenario.kind, scenario.name, scenario.description))
			result, err := NewHistory(t.TempDir()).Query(cluster, "Surface", Query{Kind: "all", Limit: 10, Page: 1})
			if err != nil || result.Total != 1 || result.Entries[0].UserID != "KU_ACTUAL" || result.Entries[0].Name != scenario.name || result.Entries[0].Message != scenario.description {
				t.Fatalf("announcement targeted another account: %+v %v", result, err)
			}
		})
	}
}
