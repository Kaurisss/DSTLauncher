package chatlog

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// History archives both log streams independently of whether the page is open.
// The source cursors and records are committed together so restarts do not re-import deleted rows.
type History struct {
	root   string
	mu     sync.Mutex
	states map[string]*historyState
}

type logCursor struct {
	Offset     int64     `json:"offset"`
	PrefixSize int       `json:"prefixSize"`
	PrefixHash string    `json:"prefixHash"`
	Session    int       `json:"session"`
	Modified   time.Time `json:"modified"`
}

type storedEntry struct {
	Entry
	Session      int    `json:"session"`
	DirectID     bool   `json:"directId"`
	Announcement string `json:"announcement,omitempty"`
}

type historyState struct {
	Version           int                         `json:"version"`
	NextID            int64                       `json:"nextId"`
	Server            logCursor                   `json:"server"`
	Chat              logCursor                   `json:"chat"`
	StartedAt         time.Time                   `json:"startedAt"`
	ChatServerSession int                         `json:"chatServerSession"`
	Players           map[string]identity         `json:"players"`
	Connections       map[string]string           `json:"connections"`
	Spawns            map[string][]identityChange `json:"spawns"`
	Entries           []storedEntry               `json:"entries"`
	Modified          time.Time                   `json:"modified"`
	dirty             bool
}

func NewHistory(root string) *History {
	return &History{root: root, states: map[string]*historyState{}}
}

func (h *History) Sync(clusterDir, shard string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.sync(clusterDir, shard)
	return err
}

func (h *History) Query(clusterDir, shard string, q Query) (Result, error) {
	result := Result{Entries: []Entry{}}
	if err := q.Validate(); err != nil {
		return result, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s, err := h.sync(clusterDir, shard)
	if err != nil {
		return result, err
	}
	matched := make([]Entry, 0)
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	for i := len(s.Entries) - 1; i >= 0; i-- {
		e := s.Entries[i].Entry
		if q.Kind != "all" && e.Kind != q.Kind {
			continue
		}
		text := strings.Join([]string{e.Name, e.UserID, e.SteamID, e.IP, e.Prefab, e.Message, e.Time}, " ")
		if needle == "" || strings.Contains(strings.ToLower(text), needle) {
			matched = append(matched, e)
		}
	}
	page := q.Page
	if page == 0 {
		page = 1
	}
	from := len(matched)
	if page-1 <= len(matched)/q.Limit {
		from = (page - 1) * q.Limit
	}
	to := min(from+q.Limit, len(matched))
	result.Entries = append(result.Entries, matched[from:to]...)
	result.Total, result.Page, result.PageSize = len(matched), page, q.Limit
	result.Exists = s.Chat.Offset > 0 || len(s.Entries) > 0
	// A new empty chat file is different from a missing one.
	if !result.Exists {
		_, statErr := os.Stat(filepath.Join(clusterDir, shard, "server_chat_log.txt"))
		result.Exists = statErr == nil
	}
	result.Modified = s.Modified
	result.Truncated = q.Page == 0 && len(matched) > q.Limit
	return result, nil
}

// Face is who a Klei ID was the last time this room's logs recorded them.
type Face struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Prefab string `json:"prefab,omitempty"`
}

type faceStamp struct {
	Face
	nameAt, prefabAt time.Time
}

// Faces reads each shard's player history. Keys are lowercase Klei IDs.
// A name shared by several log lines is kept only when a later chat or join says which one is current.
func (h *History) Faces(clusterDir string, shards []string) (map[string]Face, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	merged := map[string]*faceStamp{}
	for _, shard := range shards {
		s, err := h.sync(clusterDir, shard)
		if err != nil {
			return nil, err
		}
		for key, face := range s.faces() {
			mergeFace(merged, key, face)
		}
	}
	out := make(map[string]Face, len(merged))
	for key, face := range merged {
		out[key] = face.Face
	}
	return out, nil
}

func (s *historyState) faces() map[string]faceStamp {
	latest := map[string]faceStamp{}
	for _, entry := range s.Entries {
		if entry.UserID == "" || entry.Name == "" {
			continue
		}
		at := entry.CollectedAt
		if entry.RecordedAt != nil && !entry.RecordedAt.IsZero() {
			at = *entry.RecordedAt
		}
		key := strings.ToLower(entry.UserID)
		prev, ok := latest[key]
		if ok && at.Before(prev.nameAt) {
			continue
		}
		latest[key] = faceStamp{Face: Face{Name: entry.Name}, nameAt: at}
	}
	out := map[string]faceStamp{}
	for id, player := range s.Players {
		key := strings.ToLower(id)
		face := faceStamp{Face: Face{ID: id, Prefab: player.Prefab}, prefabAt: s.prefabTime(id)}
		if named, ok := latest[key]; ok {
			face.Name, face.nameAt = named.Name, named.nameAt
		} else if len(player.Names) == 1 {
			for name := range player.Names {
				face.Name = name
			}
			face.nameAt = s.StartedAt
		}
		if face.Name == "" && face.Prefab == "" {
			continue
		}
		out[key] = face
	}
	return out
}

func (s *historyState) prefabTime(id string) time.Time {
	player := s.Players[id]
	if s.StartedAt.IsZero() || player.Prefab == "" {
		return time.Time{}
	}
	at := time.Duration(-1)
	for _, change := range player.Changes {
		if change.Prefab != "" && change.At >= at {
			at = change.At
		}
	}
	if at < 0 {
		return time.Time{}
	}
	return s.StartedAt.Add(at)
}

func mergeFace(dst map[string]*faceStamp, key string, next faceStamp) {
	cur := dst[key]
	if cur == nil {
		dst[key] = &next
		return
	}
	if next.Name != "" && (cur.Name == "" || later(next.nameAt, cur.nameAt)) {
		cur.Name, cur.nameAt = next.Name, next.nameAt
	}
	if next.Prefab != "" && (cur.Prefab == "" || later(next.prefabAt, cur.prefabAt)) {
		cur.Prefab, cur.prefabAt = next.Prefab, next.prefabAt
	}
}

func later(a, b time.Time) bool {
	return !a.IsZero() && (b.IsZero() || a.After(b))
}

func (h *History) Delete(clusterDir, shard string, ids []int64) (int, error) {
	if len(ids) == 0 || len(ids) > 500 {
		return 0, fmt.Errorf("请选择 1–500 条日志")
	}
	selected := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return 0, fmt.Errorf("日志 ID 须为正整数")
		}
		selected[id] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s, err := h.sync(clusterDir, shard)
	if err != nil {
		return 0, err
	}
	remaining := make([]storedEntry, 0, len(s.Entries))
	for _, e := range s.Entries {
		if !selected[e.ID] {
			remaining = append(remaining, e)
		}
	}
	deleted := len(s.Entries) - len(remaining)
	if deleted == 0 {
		return 0, nil
	}
	previous := s.Entries
	s.Entries, s.dirty = remaining, true
	if err := h.save(clusterDir, shard, s); err != nil {
		s.Entries = previous
		return 0, err
	}
	return deleted, nil
}

func (h *History) sync(clusterDir, shard string) (*historyState, error) {
	dir, err := resolveShardDir(clusterDir, shard)
	if err != nil {
		return nil, err
	}
	path := h.statePath(clusterDir, shard)
	s := h.states[path]
	if s == nil {
		s, err = loadHistory(path)
		if err != nil {
			return nil, err
		}
		h.states[path] = s
	}
	if dir != "" {
		serverCursor, chatCursor := s.Server, s.Chat
		if err := readLog(filepath.Join(dir, "server_log.txt"), &s.Server, func() {
			s.StartedAt = time.Time{}
			s.Players = map[string]identity{}
			s.Connections = map[string]string{}
			s.Spawns = map[string][]identityChange{}
		}, func(line string, _ int64) { s.observeServer(line); s.dirty = true }); err != nil {
			return nil, err
		}
		chatPath := filepath.Join(dir, "server_chat_log.txt")
		ready, err := s.prepareChat(chatPath)
		if err != nil {
			return nil, err
		}
		if ready {
			if err := readLog(chatPath, &s.Chat, func() {}, func(line string, offset int64) {
				s.dirty = true
				if e, ok := parse(line, offset); ok {
					if e.UserID != "" {
						p := s.player(e.UserID)
						p.Names[e.Name] = true
						s.Players[e.UserID] = p
					}
					e.ID, e.CollectedAt = s.NextID, time.Now()
					s.NextID++
					record := storedEntry{Entry: e, Session: s.Server.Session, DirectID: e.UserID != ""}
					if e.Kind == "death" || e.Kind == "resurrect" {
						record.Announcement = prefix.FindStringSubmatch(line)[3]
					}
					s.Entries = append(s.Entries, record)
				}
			}); err != nil {
				return nil, err
			}
		}
		if serverCursor != s.Server || chatCursor != s.Chat {
			s.dirty = true
		}
	}
	s.enrich()
	if s.dirty {
		if err := h.save(clusterDir, shard, s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (h *History) statePath(clusterDir, shard string) string {
	return filepath.Join(h.root, filepath.Base(filepath.Clean(clusterDir)), shard+".json")
}

func (h *History) save(clusterDir, shard string, s *historyState) error {
	path := h.statePath(clusterDir, shard)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("创建玩家历史目录: %w", err)
	}
	s.Modified = time.Now()
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("写入玩家历史: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("保存玩家历史: %w", err)
	}
	s.dirty = false
	return nil
}

func loadHistory(path string) (*historyState, error) {
	s := &historyState{Version: 1, NextID: 1, Server: logCursor{Session: 1}, Chat: logCursor{Session: 1}, ChatServerSession: 1,
		Players: map[string]identity{}, Connections: map[string]string{}, Spawns: map[string][]identityChange{}, Entries: []storedEntry{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取玩家历史: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("解析玩家历史: %w", err)
	}
	if s.Version != 1 || s.NextID < 1 || s.Players == nil || s.Connections == nil || s.Spawns == nil || s.ChatServerSession < 1 || s.ChatServerSession > s.Server.Session {
		return nil, fmt.Errorf("玩家历史文件格式无效: %s", path)
	}
	for _, cursor := range []logCursor{s.Server, s.Chat} {
		if cursor.Offset < 0 || cursor.PrefixSize < 0 || cursor.PrefixSize > 4096 || int64(cursor.PrefixSize) > cursor.Offset || cursor.Session < 1 {
			return nil, fmt.Errorf("玩家历史日志游标无效: %s", path)
		}
	}
	return s, nil
}

func resolveShardDir(clusterDir, shard string) (string, error) {
	if shard == "" || shard == "." || shard == ".." || strings.ContainsAny(shard, "/\\") {
		return "", fmt.Errorf("分片名无效")
	}
	base, err := filepath.EvalSymlinks(clusterDir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dir, err := filepath.EvalSymlinks(filepath.Join(clusterDir, shard))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(base, dir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("分片日志路径超出当前房间")
	}
	return dir, nil
}

func readLog(path string, cursor *logCursor, reset func(), consume func(string, int64)) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("日志不是普通文件: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return err
	}
	fingerprint, fingerprintErr := logFingerprint(f, *cursor)
	if info.Size() < cursor.Offset || (cursor.PrefixSize > 0 && (fingerprintErr != nil || fingerprint != cursor.PrefixHash)) {
		*cursor = logCursor{Session: cursor.Session + 1}
		reset()
	}
	if _, err := f.Seek(cursor.Offset, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break // Keep the cursor before an incomplete line so its later suffix is not lost.
		}
		if err != nil {
			return err
		}
		consume(strings.TrimRight(line, "\r\n"), cursor.Offset)
		cursor.Offset += int64(len(line))
	}
	cursor.PrefixSize = int(min(cursor.Offset, 4096))
	cursor.PrefixHash, err = logFingerprint(f, *cursor)
	if err != nil {
		return err
	}
	info, err = f.Stat()
	if err != nil {
		return err
	}
	cursor.Modified = info.ModTime()
	return nil
}

func logFingerprint(f *os.File, cursor logCursor) (string, error) {
	data := make([]byte, cursor.PrefixSize*2)
	if _, err := f.ReadAt(data[:cursor.PrefixSize], 0); err != nil {
		return "", err
	}
	if _, err := f.ReadAt(data[cursor.PrefixSize:], cursor.Offset-int64(cursor.PrefixSize)); err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func (s *historyState) prepareChat(path string) (bool, error) {
	if s.ChatServerSession == s.Server.Session {
		return true, nil
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("日志不是普通文件: %s", path)
	}
	// The old chat file may remain until the new game writes its first announcement.
	if (!s.StartedAt.IsZero() && info.ModTime().Before(s.StartedAt)) || (s.StartedAt.IsZero() && info.ModTime().Equal(s.Chat.Modified)) {
		return false, nil
	}
	s.Chat = logCursor{Session: s.Chat.Session + 1}
	s.ChatServerSession = s.Server.Session
	s.dirty = true
	return true, nil
}
