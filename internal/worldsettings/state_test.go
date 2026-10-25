package worldsettings

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func meta(cycles int, season string, remaining int) string {
	return fmt.Sprintf(`KLEI     1 return {clock={cycles=%d},seasons={season=%q,remainingdaysinseason=%d}}`, cycles, season, remaining)
}

func TestSnapshotStateFieldsAndSafeParsing(t *testing.T) {
	for _, season := range []string{"spring", "summer", "autumn", "winter", "mild", "wet", "dry", "green"} {
		s, err := parseWorldState(meta(0, season, 0))
		if err != nil || s.Day == nil || *s.Day != 1 || s.Season != season || s.RemainingDays == nil || *s.RemainingDays != 0 {
			t.Fatalf("%s: %+v %v", season, s, err)
		}
	}
	for _, raw := range []string{`return {}`, `return {world_network=os.execute("bad")}`, `return {world_network={persistdata={clock={cycles=-1}}}}`, `return {world_network={persistdata={clock={cycles=1.5}}}}`} {
		if _, err := parseWorldState(raw); err == nil {
			t.Fatalf("不可信元数据被接受: %s", raw)
		}
	}
	s, err := parseWorldState(`return {world_network={persistdata={seasons={season="winter",mode="always",remainingdaysinseason=999}}}}`)
	if err != nil || s.Day != nil || s.RemainingDays != nil {
		t.Fatal("未知天数和永久季节不应伪造数字", s, err)
	}
}

func TestReadNewestWorldSnapshotNotPlayerSnapshot(t *testing.T) {
	dir := t.TempDir()
	if s, err := ReadWorldState(dir); s != nil || err != nil {
		t.Fatal(s, err)
	}
	write := func(name, text string, when time.Time) {
		path := filepath.Join(dir, "save", "session", name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().Truncate(time.Second)
	write("AAA/0000000001.meta", meta(10, "autumn", 3), now.Add(-time.Hour))
	write("BBB/0000000002.meta", meta(20, "winter", 0), now)
	write("BBB/KU_user/0000000009.meta", `return {}`, now.Add(time.Hour))
	s, err := ReadWorldState(dir)
	if err != nil || *s.Day != 21 || s.Season != "winter" || !s.SavedAt.Equal(now) {
		t.Fatal(s, err)
	}
	write("BBB/0000000003.meta", `return {broken`, now.Add(time.Minute))
	if _, err := ReadWorldState(dir); err == nil {
		t.Fatal("最新快照损坏应报错，不能用旧快照伪装成功")
	}
}
