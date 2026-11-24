package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dst-windows/internal/backup"
	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/worldsettings"
)

type backupCheck struct {
	ID                string             `json:"id"`
	Room              int                `json:"room"`
	Name              string             `json:"name"`
	State             string             `json:"state"`
	Done              int                `json:"done"`
	Total             int                `json:"total"`
	Error             string             `json:"error,omitempty"`
	Report            *backup.Inspection `json:"report,omitempty"`
	PreviousDirectory string             `json:"previousDirectory,omitempty"`
	dir               string
	payload           string
	created           time.Time
	cancel            context.CancelFunc
	discard           bool
}
type backupChecks struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed bool
	items  map[string]*backupCheck
}

func newBackupChecks() *backupChecks {
	ctx, cancel := context.WithCancel(context.Background())
	return &backupChecks{ctx: ctx, cancel: cancel, items: map[string]*backupCheck{}}
}
func (s *Server) CancelBackupChecks() { s.backupChecks.cancel() }
func (s *Server) StopBackupChecks() {
	m := s.backupChecks
	m.mu.Lock()
	m.closed = true
	m.cancel()
	m.mu.Unlock()
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.items {
		_ = os.RemoveAll(j.dir)
	}
}
func (s *Server) handleBackupCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	m := s.backupChecks
	if r.Method == http.MethodGet || r.Method == http.MethodDelete {
		m.mu.Lock()
		defer m.mu.Unlock()
		j, ok := m.items[query(r, "id")]
		if !ok {
			respond(w, 200, nil, fmt.Errorf("备份检查已过期，请重新检查"))
			return
		}
		if r.Method == http.MethodDelete {
			if j.State == "checking" {
				j.discard = true
				j.cancel()
				respond(w, 200, nil, nil)
				return
			}
			if j.State == "restoring" {
				respond(w, 200, nil, fmt.Errorf("备份任务正在执行"))
				return
			}
			err := os.RemoveAll(j.dir)
			if err == nil {
				delete(m.items, j.ID)
			}
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, j, nil)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err = readJSON(w, r, &in, 4096); err != nil {
		respond(w, 200, nil, err)
		return
	}
	if _, err = s.bakFor(rm).ArchivePath(in.Name); err != nil {
		respond(w, 200, nil, err)
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		respond(w, 200, nil, fmt.Errorf("面板正在关闭"))
		return
	}
	for _, job := range m.items {
		if job.State == "checking" || job.State == "restoring" {
			m.mu.Unlock()
			respond(w, 200, nil, fmt.Errorf("已有备份检查或恢复正在执行"))
			return
		}
	}
	for id, j := range m.items {
		if time.Since(j.created) > time.Hour {
			_ = os.RemoveAll(j.dir)
			delete(m.items, id)
		}
	}
	if len(m.items) >= 3 {
		var oldest *backupCheck
		for _, j := range m.items {
			if oldest == nil || j.created.Before(oldest.created) {
				oldest = j
			}
		}
		if err = os.RemoveAll(oldest.dir); err != nil {
			m.mu.Unlock()
			respond(w, 200, nil, err)
			return
		}
		delete(m.items, oldest.ID)
	}
	base := filepath.Join(s.rootDir, "backup-checks")
	if err = os.MkdirAll(base, 0700); err != nil {
		m.mu.Unlock()
		respond(w, 200, nil, err)
		return
	}
	dir, err := os.MkdirTemp(base, "check-")
	if err != nil {
		m.mu.Unlock()
		respond(w, 200, nil, err)
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	j := &backupCheck{ID: rand.Text(), Room: rm.ID, Name: in.Name, State: "checking", dir: dir, payload: filepath.Join(dir, "world", s.confDir, config.ClusterName(rm.ID)), created: time.Now(), cancel: cancel}
	m.items[j.ID] = j
	m.wg.Add(1)
	m.mu.Unlock()
	names := []string{}
	for _, shard := range rm.Shards {
		names = append(names, shard.Name)
	}
	go func() {
		defer m.wg.Done()
		defer cancel()
		s.backupMu.Lock()
		report, err := s.bakFor(rm).Stage(ctx, in.Name, j.payload, names, func(d, t int) { m.mu.Lock(); j.Done, j.Total = d, t; m.mu.Unlock() })
		s.backupMu.Unlock()
		m.mu.Lock()
		defer m.mu.Unlock()
		if j.discard {
			_ = os.RemoveAll(j.dir)
			delete(m.items, j.ID)
			return
		}
		if err != nil {
			j.State, j.Error = "failed", err.Error()
			_ = os.RemoveAll(j.dir)
		} else {
			j.State, j.Report = "ready", report
		}
	}()
	respond(w, 200, map[string]string{"id": j.ID}, nil)
}
func (s *Server) handleBackupCheckMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	m := s.backupChecks
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.items[query(r, "id")]
	if !ok || j.State != "ready" {
		respond(w, 200, nil, fmt.Errorf("请先完成备份检查"))
		return
	}
	shard := query(r, "shard")
	found := false
	for _, item := range j.Report.Shards {
		if item.Name == shard && item.Snapshots > 0 {
			found = true
		}
	}
	if !found {
		respond(w, 200, nil, fmt.Errorf("此分片没有世界保存点"))
		return
	}
	if !s.mapMu.TryLock() {
		respond(w, 200, nil, fmt.Errorf("地图正在读取，请稍后重试"))
		return
	}
	defer s.mapMu.Unlock()
	data, err := worldsettings.ReadSavedMap(filepath.Join(j.payload, shard))
	if err == nil && data.Terrain == nil {
		err = fmt.Errorf("此保存点缺少可读取的地块数据")
	}
	respond(w, 200, data, err)
}
func (s *Server) handleCheckedRestore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string `json:"id"`
		SHA256 string `json:"sha256"`
	}
	if err := readJSON(w, r, &in, 4096); err != nil {
		respond(w, 200, nil, err)
		return
	}
	target, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	m := s.backupChecks
	m.mu.Lock()
	j, ok := m.items[in.ID]
	if m.closed || !ok || j.Room != target.ID || j.State != "ready" || !j.Report.CanRestore || j.Report.SHA256 != in.SHA256 {
		m.mu.Unlock()
		respond(w, 200, nil, fmt.Errorf("请先完成有效备份检查再恢复"))
		return
	}
	j.State = "restoring"
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		s.maintenanceMu.Lock()
		defer s.maintenanceMu.Unlock()
		s.taskMu.Lock()
		defer s.taskMu.Unlock()
		s.backupMu.Lock()
		defer s.backupMu.Unlock()
		resume := s.mgr.PauseRecovery()
		defer resume()
		rm, exists := s.rooms.Get(j.Room)
		var err error
		previous := ""
		if !exists {
			err = fmt.Errorf("房间已不存在")
		} else if s.mgr.AnyRunning(rm.Key()) {
			err = fmt.Errorf("请先停止服务器再恢复存档")
		} else if err = m.ctx.Err(); err == nil {
			if err = s.busyTaskError(""); err == nil {
				if err = room.Generate(filepath.Join(j.dir, "world"), s.confDir, rm); err == nil {
					previous, err = s.bakFor(rm).RestoreStaged(m.ctx, j.Name, j.payload, j.Report.SHA256)
				}
			}
		}
		if err == nil {
			s.syncModsSetup()
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			j.State, j.Error = "failed", err.Error()
		} else {
			j.State, j.PreviousDirectory = "restored", previous
		}
	}()
	respond(w, 200, map[string]string{"id": in.ID}, nil)
}
