package web

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dst-windows/internal/migration"
	"dst-windows/internal/room"
	"dst-windows/internal/roomprofile"
	"dst-windows/internal/server"
)

type migrationPreviewRoom struct {
	Name   string       `json:"name"`
	Shards []room.Shard `json:"shards"`
}
type migrationPreview struct {
	CreatedAt time.Time              `json:"createdAt"`
	Rooms     []migrationPreviewRoom `json:"rooms"`
	Profiles  int                    `json:"profiles"`
	Files     int                    `json:"files"`
	Bytes     int64                  `json:"bytes"`
	HasPlan   bool                   `json:"hasPlan"`
}
type migrationJob struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	State    string            `json:"state"`
	Done     int               `json:"done"`
	Total    int               `json:"total"`
	Error    string            `json:"error,omitempty"`
	Preview  *migrationPreview `json:"preview,omitempty"`
	RoomIDs  []int             `json:"roomIds,omitempty"`
	dir      string
	manifest *migration.Manifest
	updated  time.Time
}
type migrationJobs struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	items  map[string]*migrationJob
	closed bool
}

func newMigrationJobs() *migrationJobs {
	ctx, cancel := context.WithCancel(context.Background())
	return &migrationJobs{ctx: ctx, cancel: cancel, items: map[string]*migrationJob{}}
}
func (s *Server) StopMigrations() {
	m := s.migrations
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
func (s *Server) newMigrationJob(kind string) (*migrationJob, error) {
	m := s.migrations
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("面板正在关闭")
	}
	for id, job := range m.items {
		if job.State == "working" {
			return nil, fmt.Errorf("已有迁移任务正在执行")
		}
		if time.Since(job.updated) > 24*time.Hour {
			if err := os.RemoveAll(job.dir); err != nil {
				return nil, err
			}
			delete(m.items, id)
		}
	}
	if len(m.items) >= 4 {
		return nil, fmt.Errorf("临时迁移包已达 4 个，请删除已完成任务")
	}
	base := filepath.Join(s.rootDir, "migration-tmp")
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(base, "job-")
	if err != nil {
		return nil, err
	}
	j := &migrationJob{ID: rand.Text(), Kind: kind, State: "working", dir: dir, updated: time.Now()}
	m.items[j.ID] = j
	m.wg.Add(1)
	return j, nil
}
func (s *Server) migrationProgress(j *migrationJob, done, total int) {
	s.migrations.mu.Lock()
	defer s.migrations.mu.Unlock()
	j.Done, j.Total, j.updated = done, total, time.Now()
}
func previewMigration(m *migration.Manifest) *migrationPreview {
	p := &migrationPreview{CreatedAt: m.CreatedAt, Rooms: []migrationPreviewRoom{}, Profiles: len(m.Profiles), Files: len(m.Files), HasPlan: m.Plan != nil}
	for _, r := range m.Rooms {
		p.Rooms = append(p.Rooms, migrationPreviewRoom{r.Name, r.Shards})
	}
	for _, f := range m.Files {
		p.Bytes += f.Size
	}
	return p
}
func (s *Server) finishMigration(j *migrationJob, manifest *migration.Manifest, err error) {
	s.migrations.mu.Lock()
	defer s.migrations.mu.Unlock()
	j.updated = time.Now()
	if err != nil {
		j.State = "failed"
		j.Error = err.Error()
		if cleanup := os.RemoveAll(j.dir); cleanup != nil {
			j.Error += "；清理临时包失败: " + cleanup.Error()
		}
		return
	}
	j.State = "ready"
	if manifest != nil {
		j.manifest = manifest
		j.Preview = previewMigration(manifest)
	}
}
func (s *Server) migrationStopped() error {
	if s.automationErr != nil {
		return s.automationErr
	}
	for _, r := range s.rooms.List() {
		if s.mgr.AnyRunning(r.Key()) {
			return fmt.Errorf("请先停止全部房间再执行迁移")
		}
	}
	if err := s.busyTaskError("migration"); err != nil {
		return err
	}
	return s.migrations.ctx.Err()
}
func (s *Server) handleMigrationExport(w http.ResponseWriter, r *http.Request) {
	j, err := s.newMigrationJob("export")
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	id := j.ID
	go func() {
		defer s.migrations.wg.Done()
		s.maintenanceMu.Lock()
		defer s.maintenanceMu.Unlock()
		resume := s.mgr.PauseRecovery()
		defer resume()
		if err := s.migrationStopped(); err != nil {
			s.finishMigration(j, nil, err)
			return
		}
		store := roomprofile.New(filepath.Join(s.rootDir, "profiles"))
		items, err := store.List()
		profiles := []*roomprofile.Profile{}
		if err == nil {
			for _, item := range items {
				var p *roomprofile.Profile
				p, err = store.Load(item.ID)
				if err != nil {
					break
				}
				profiles = append(profiles, p)
			}
		}
		if err != nil {
			s.finishMigration(j, nil, err)
			return
		}
		manifest, err := migration.Create(s.migrations.ctx, filepath.Join(j.dir, "migration.zip"), migration.Options{Root: s.rootDir, ConfDir: s.confDir, Rooms: s.rooms.List(), Plan: s.automation.Snapshot().Config, Profiles: profiles, PanelVersion: s.version, GameVersion: s.steam.Version()}, func(d, t int) { s.migrationProgress(j, d, t) })
		s.finishMigration(j, manifest, err)
	}()
	respond(w, 200, map[string]string{"id": id}, nil)
}
func (s *Server) handleMigrationUpload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > migration.MaxUpload {
		respond(w, 200, nil, fmt.Errorf("迁移压缩包不能超过 8 GiB"))
		return
	}
	j, err := s.newMigrationJob("upload")
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	f, err := os.OpenFile(filepath.Join(j.dir, "migration.zip"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		stop := context.AfterFunc(s.migrations.ctx, func() { _ = r.Body.Close() })
		_, err = io.Copy(f, http.MaxBytesReader(w, r.Body, migration.MaxUpload))
		stop()
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		s.finishMigration(j, nil, err)
		s.migrations.wg.Done()
		respond(w, 200, map[string]string{"id": j.ID}, nil)
		return
	}
	id := j.ID
	go func() {
		defer s.migrations.wg.Done()
		manifest, err := migration.Extract(s.migrations.ctx, filepath.Join(j.dir, "migration.zip"), filepath.Join(j.dir, "payload"), func(d, t int) { s.migrationProgress(j, d, t) })
		s.finishMigration(j, manifest, err)
	}()
	respond(w, 200, map[string]string{"id": id}, nil)
}
func (s *Server) handleMigrationJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	m := s.migrations
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.items[query(r, "id")]
	if !ok {
		if r.Method == http.MethodDelete {
			respond(w, 200, nil, nil)
			return
		}
		respond(w, 200, nil, fmt.Errorf("迁移任务已过期"))
		return
	}
	if r.Method == http.MethodDelete {
		if j.State == "working" {
			respond(w, 200, nil, fmt.Errorf("任务正在执行"))
			return
		}
		if err := os.RemoveAll(j.dir); err != nil {
			respond(w, 200, nil, err)
			return
		}
		delete(m.items, j.ID)
		respond(w, 200, nil, nil)
		return
	}
	respond(w, 200, j, nil)
}
func (s *Server) handleMigrationDownload(w http.ResponseWriter, r *http.Request) {
	m := s.migrations
	m.mu.Lock()
	j, ok := m.items[query(r, "id")]
	if !ok || j.Kind != "export" || j.State != "ready" {
		m.mu.Unlock()
		respond(w, 200, nil, fmt.Errorf("迁移包尚未生成"))
		return
	}
	f, err := os.Open(filepath.Join(j.dir, "migration.zip"))
	j.updated = time.Now()
	m.mu.Unlock()
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="dst-migration.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "dst-migration.zip", info.ModTime(), f)
}
func (s *Server) handleMigrationApply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        string `json:"id"`
		ApplyPlan bool   `json:"applyPlan"`
	}
	if err := readJSON(w, r, &in, 4096); err != nil {
		respond(w, 200, nil, err)
		return
	}
	m := s.migrations
	m.mu.Lock()
	j, ok := m.items[in.ID]
	if m.closed || !ok || j.Kind != "upload" || j.State != "ready" {
		m.mu.Unlock()
		respond(w, 200, nil, fmt.Errorf("请先上传并校验迁移包"))
		return
	}
	for _, other := range m.items {
		if other.State == "working" {
			m.mu.Unlock()
			respond(w, 200, nil, fmt.Errorf("已有迁移任务正在执行"))
			return
		}
	}
	j.Kind, j.State = "import", "working"
	m.wg.Go(func() {
		s.maintenanceMu.Lock()
		defer s.maintenanceMu.Unlock()
		resume := s.mgr.PauseRecovery()
		defer resume()
		if err := s.migrationStopped(); err != nil {
			s.finishMigration(j, nil, err)
			return
		}
		ids, err := s.applyMigration(j, in.ApplyPlan)
		m.mu.Lock()
		j.RoomIDs = ids
		m.mu.Unlock()
		s.finishMigration(j, nil, err)
	})
	m.mu.Unlock()
	respond(w, 200, map[string]string{"id": in.ID}, nil)
}
func (s *Server) applyMigration(j *migrationJob, applyPlan bool) ([]int, error) {
	ctx := s.migrations.ctx
	var dirs []string
	var profileIDs []string
	profiles := roomprofile.New(filepath.Join(s.rootDir, "profiles"))
	oldPlan := s.automation.Snapshot().Config
	planChanged := false
	rollback := func() error {
		var failures []error
		for _, dir := range dirs {
			failures = append(failures, os.RemoveAll(dir))
		}
		for _, id := range profileIDs {
			failures = append(failures, profiles.Delete(id))
		}
		if planChanged {
			failures = append(failures, s.automation.Configure(oldPlan, time.Now()))
		}
		return errors.Join(failures...)
	}
	created, err := s.rooms.CreateBatch(j.manifest.Rooms, func(created []*room.Room) (err error) {
		defer func() {
			if err != nil {
				err = errors.Join(err, rollback())
			}
		}()
		for i, rm := range created {
			if err = ctx.Err(); err != nil {
				return err
			}
			dir := rm.ClusterDir(s.rootDir, s.confDir)
			if err = os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
				return err
			}
			if err = os.Mkdir(dir, 0700); err != nil {
				return err
			}
			dirs = append(dirs, dir)
			if err = copyMigrationTree(ctx, filepath.Join(j.dir, "payload", "rooms", fmt.Sprint(j.manifest.Rooms[i].ID)), dir); err != nil {
				return err
			}
			if err = room.Generate(s.rootDir, s.confDir, rm); err != nil {
				return err
			}
			s.migrationProgress(j, i+1, len(created))
		}
		for _, id := range j.manifest.Profiles {
			if err = ctx.Err(); err != nil {
				return err
			}
			var b []byte
			b, err = os.ReadFile(filepath.Join(j.dir, "payload", "profiles", id+".json"))
			if err != nil {
				return err
			}
			var p roomprofile.Profile
			if err = json.Unmarshal(b, &p); err != nil {
				return err
			}
			var saved *roomprofile.Profile
			saved, err = profiles.Save(&p)
			if err != nil {
				return err
			}
			profileIDs = append(profileIDs, saved.ID)
		}
		if applyPlan {
			if err = s.automation.Configure(*j.manifest.Plan, time.Now()); err != nil {
				return err
			}
			planChanged = true
		}
		return ctx.Err()
	}, rollback)
	if err != nil {
		return nil, err
	}
	ids := []int{}
	for _, rm := range created {
		s.mgr.Register(server.NewCluster(rm))
		ids = append(ids, rm.ID)
	}
	s.syncModsSetup()
	return ids, nil
}
func copyMigrationTree(ctx context.Context, source, target string) error {
	return filepath.WalkDir(source, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, file)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		src, err := os.Open(file)
		if err != nil {
			return err
		}
		defer src.Close()
		stop := context.AfterFunc(ctx, func() { _ = src.Close() })
		defer stop()
		dst, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		err = errors.Join(err, dst.Close())
		if err != nil {
			return err
		}
		return os.Chtimes(dest, info.ModTime(), info.ModTime())
	})
}
