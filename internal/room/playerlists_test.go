package room

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPlayerListsNormalizeAndKeepKindsSeparate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "whitelist.txt"), []byte("\uFEFF# comment\r\nKU_A-1\r\nku_a-1\nKU_B_2\nnot-a-user\n\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ids, err := ReadPlayerList(dir, WhiteList)
	if err != nil || strings.Join(ids, ",") != "KU_A-1,KU_B_2" {
		t.Fatalf("external list: %v, %v", ids, err)
	}
	for _, kind := range []string{"../whitelist", "admins", "whitelist.txt", ""} {
		if _, err := ChangePlayerList(dir, kind, "KU_test", false); err == nil {
			t.Fatalf("accepted invalid list kind %q", kind)
		}
	}
	if _, err := ChangePlayerList(dir, BlockList, "KU_bad\nKU_other", false); err == nil {
		t.Fatal("accepted line injection")
	}
	for _, id := range []string{"ku_blocked-1", "KU_blocked-1"} {
		ids, err = ChangePlayerList(dir, BlockList, id, false)
		if err != nil || len(ids) != 1 {
			t.Fatalf("idempotent add: %v, %v", ids, err)
		}
	}
	white, _ := ReadPlayerList(dir, WhiteList)
	if len(white) != 2 {
		t.Fatal("blocklist operation modified whitelist")
	}
	for range 2 {
		ids, err = ChangePlayerList(dir, BlockList, "KU_blocked-1", true)
		if err != nil || len(ids) != 0 {
			t.Fatalf("idempotent remove: %v, %v", ids, err)
		}
	}
}

func TestConcurrentPlayerListAndAdminAdditions(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("KU_player%d", i)
			if _, err := ChangePlayerList(dir, WhiteList, id, false); err != nil {
				t.Error(err)
			}
			if _, err := AddAdmin(dir, id); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	white, _ := ReadPlayerList(dir, WhiteList)
	admins, _ := ReadAdmins(dir)
	if len(white) != 24 || len(admins) != 24 {
		t.Fatalf("lost concurrent writes: white=%d admins=%d", len(white), len(admins))
	}
	r := Defaults()
	r.MaxPlayers = 6
	if r.ReservedSlots(dir) != 6 {
		t.Fatal("reservations may not exceed max_players")
	}
}
