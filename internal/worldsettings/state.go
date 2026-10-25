package worldsettings

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// WorldState describes the latest saved snapshot, not live simulation time.
type WorldState struct {
	Day           *int      `json:"day"`
	Season        string    `json:"season"`
	RemainingDays *int      `json:"remainingDays"`
	SavedAt       time.Time `json:"savedAt"`
}

var snapshotName = regexp.MustCompile(`^[0-9]+\.meta$`)

func ReadWorldState(shardDir string) (*WorldState, error) {
	paths, err := filepath.Glob(filepath.Join(shardDir, "save", "session", "*", "*.meta"))
	if err != nil {
		return nil, err
	}
	var newest string
	var modified time.Time
	for _, path := range paths {
		if !snapshotName.MatchString(filepath.Base(path)) {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if newest == "" || info.ModTime().After(modified) || info.ModTime().Equal(modified) && path > newest {
			newest, modified = path, info.ModTime()
		}
	}
	if newest == "" {
		return nil, nil
	}
	f, err := os.Open(newest)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 1024*1024 {
		return nil, fmt.Errorf("世界快照元数据超过 1 MB")
	}
	state, err := parseWorldState(string(b))
	if err != nil {
		return nil, err
	}
	state.SavedAt = modified
	return state, nil
}

func stateField(table *levelTable, names ...string) levelValue {
	var value levelValue
	for _, name := range names {
		value = levelValue{}
		if table == nil {
			return value
		}
		for _, field := range table.fields {
			if field.name == name {
				value = field.value
			}
		}
		table = value.table
	}
	return value
}

func parseWorldState(raw string) (*WorldState, error) {
	root, err := scanLevelRoot(raw)
	if err != nil {
		return nil, err
	}
	// Current .meta files expose clock/seasons directly; older world summaries nest them.
	data := root
	if nested := stateField(root, "world_network", "persistdata").table; nested != nil {
		data = nested
	}
	state := &WorldState{}
	if season := stateField(data, "seasons", "season"); season.kind == 's' {
		state.Season = season.text
	}
	parse := func(value levelValue) *int {
		if value.kind != 'n' {
			return nil
		}
		n, err := strconv.Atoi(value.text)
		if err != nil || n < 0 || n > 10000000 {
			return nil
		}
		return &n
	}
	state.Day = parse(stateField(data, "clock", "cycles"))
	if state.Day != nil {
		*state.Day = *state.Day + 1
	}
	state.RemainingDays = parse(stateField(data, "seasons", "remainingdaysinseason"))
	if mode := stateField(data, "seasons", "mode").text; mode == "always" || mode == "endless" {
		state.RemainingDays = nil
	}
	if state.Day == nil && state.Season == "" {
		return nil, fmt.Errorf("快照没有世界天数或季节")
	}
	return state, nil
}
