// Package chatlog collects player identities and archives game chat events.
package chatlog

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Query struct {
	Limit  int
	Kind   string
	Search string
	Page   int
}

func (q Query) Validate() error {
	if q.Limit < 1 || q.Limit > 500 {
		return fmt.Errorf("日志条数须为 1–500")
	}
	if q.Page < 0 {
		return fmt.Errorf("页码须为正整数")
	}
	switch q.Kind {
	case "all", "chat", "whisper", "join", "leave", "death", "resurrect", "announcement":
	default:
		return fmt.Errorf("不支持的聊天记录类型")
	}
	if !utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 200 {
		return fmt.Errorf("搜索内容最多 200 个字符")
	}
	return nil
}

type Entry struct {
	ID          int64      `json:"id,omitempty"`
	Offset      int64      `json:"offset"`
	Time        string     `json:"time"`
	Kind        string     `json:"kind"`
	UserID      string     `json:"userid,omitempty"`
	Name        string     `json:"name,omitempty"`
	Message     string     `json:"message"`
	Prefab      string     `json:"prefab,omitempty"`
	SteamID     string     `json:"steamid,omitempty"`
	IP          string     `json:"ip,omitempty"`
	RecordedAt  *time.Time `json:"recordedAt,omitempty"`
	CollectedAt time.Time  `json:"collectedAt,omitzero"`
}

type Result struct {
	Entries   []Entry   `json:"entries"`
	Exists    bool      `json:"exists"`
	Truncated bool      `json:"truncated"`
	Modified  time.Time `json:"modified"`
	Total     int       `json:"total"`
	Page      int       `json:"page"`
	PageSize  int       `json:"pageSize"`
}

var prefix = regexp.MustCompile(`^\[([0-9]+:[0-5][0-9]:[0-5][0-9])\]:\s*\[([^\]]+)\]\s*(.*)$`)

var announcementSeparators = []string{
	"死于：", "死于:", "复活自：", "复活自:", "was killed by", "died from", "starved", "committed suicide",
	"was resurrected", "resurrected from", "was revived by", "revived by",
}

func parse(line string, offset int64) (Entry, bool) {
	match := prefix.FindStringSubmatch(line)
	if match == nil {
		return Entry{}, false
	}
	e := Entry{Offset: offset, Time: match[1], Message: match[3]}
	switch match[2] {
	case "Say", "Whisper":
		e.Kind = "chat"
		if match[2] == "Whisper" {
			e.Kind = "whisper"
		}
		if !strings.HasPrefix(e.Message, "(") {
			return Entry{}, false
		}
		id, rest, found := strings.Cut(e.Message[1:], ") ")
		if !found || len(id) > 128 {
			return Entry{}, false
		}
		name, message, found := strings.Cut(rest, ": ")
		if !found {
			return Entry{}, false
		}
		e.UserID, e.Name, e.Message = id, name, message
	case "Join Announcement":
		e.Kind = "join"
		e.Name = e.Message
		e.Message = "进入游戏"
	case "Leave Announcement":
		e.Kind = "leave"
		e.Name = e.Message
		e.Message = "离开游戏"
	case "Death Announcement", "Resurrect Announcement":
		e.Kind = "death"
		if match[2] == "Resurrect Announcement" {
			e.Kind = "resurrect"
		}
		for _, separator := range announcementSeparators {
			if at := strings.Index(e.Message, " "+separator); at > 0 {
				e.Name = strings.TrimSpace(e.Message[:at])
				e.Message = strings.TrimSpace(e.Message[at:])
				break
			}
		}
	case "Announcement":
		e.Kind = "announcement"
	default:
		return Entry{}, false
	}
	return e, true
}
