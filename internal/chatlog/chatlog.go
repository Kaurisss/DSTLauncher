// Package chatlog reads a bounded, read-only tail of a shard's game chat log.
package chatlog

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxBytes = 1024 * 1024

type Query struct {
	Limit  int
	Kind   string
	Search string
}

func (q Query) Validate() error {
	if q.Limit < 1 || q.Limit > 500 {
		return fmt.Errorf("日志条数须为 1–500")
	}
	switch q.Kind {
	case "all", "chat", "whisper", "join", "leave", "announcement":
	default:
		return fmt.Errorf("不支持的聊天记录类型")
	}
	if !utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 200 {
		return fmt.Errorf("搜索内容最多 200 个字符")
	}
	return nil
}

type Entry struct {
	Offset  int64  `json:"offset"`
	Time    string `json:"time"`
	Kind    string `json:"kind"`
	UserID  string `json:"userid,omitempty"`
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
}

type Result struct {
	Entries   []Entry   `json:"entries"`
	Exists    bool      `json:"exists"`
	Truncated bool      `json:"truncated"`
	Modified  time.Time `json:"modified"`
}

var prefix = regexp.MustCompile(`^\[([0-9]+:[0-5][0-9]:[0-5][0-9])\]:\s*\[([^\]]+)\]\s*(.*)$`)

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
	case "Announcement":
		e.Kind = "announcement"
	default:
		return Entry{}, false
	}
	return e, true
}

func Read(clusterDir, shard string, q Query) (Result, error) {
	result := Result{Entries: []Entry{}}
	if err := q.Validate(); err != nil {
		return result, err
	}
	if shard == "" || shard == "." || shard == ".." || strings.ContainsAny(shard, "/\\") {
		return result, fmt.Errorf("分片名无效")
	}
	// Resolve the actual directory and reject links leading outside this cluster.
	base, err := filepath.EvalSymlinks(clusterDir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	dir, err := filepath.EvalSymlinks(filepath.Join(clusterDir, shard))
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	relative, err := filepath.Rel(base, dir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return result, fmt.Errorf("分片日志路径超出当前房间")
	}
	path := filepath.Join(dir, "server_chat_log.txt")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("聊天日志不是普通文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return result, err
	}
	start := info.Size() - MaxBytes
	if start < 0 {
		start = 0
	}
	data := make([]byte, info.Size()-start)
	n, err := f.ReadAt(data, start)
	if err != nil && err != io.EOF {
		return result, err
	}
	data = data[:n]
	result.Exists = true
	result.Modified = info.ModTime()
	result.Truncated = start > 0
	if start > 0 {
		if end := bytes.IndexByte(data, '\n'); end >= 0 {
			start += int64(end + 1)
			data = data[end+1:]
		} else {
			return result, nil
		}
	}
	// Ignore a line still being written; it will appear after its newline arrives.
	if end := bytes.LastIndexByte(data, '\n'); end >= 0 {
		data = data[:end+1]
	} else {
		return result, nil
	}
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
		text := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
		entry, ok := parse(text, start)
		start += int64(len(line))
		if !ok || (q.Kind != "all" && entry.Kind != q.Kind) {
			continue
		}
		haystack := entry.Name + " " + entry.UserID + " " + entry.Message + " " + entry.Time
		if needle != "" && !strings.Contains(strings.ToLower(haystack), needle) {
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	if len(result.Entries) > q.Limit {
		result.Entries = result.Entries[len(result.Entries)-q.Limit:]
		result.Truncated = true
	}
	// Latest first while keeping each shard's file order; the clock is game uptime,
	// not wall time and must not be used to sort across game restarts.
	for i, j := 0, len(result.Entries)-1; i < j; i, j = i+1, j-1 {
		result.Entries[i], result.Entries[j] = result.Entries[j], result.Entries[i]
	}
	return result, nil
}
