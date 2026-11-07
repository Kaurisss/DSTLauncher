package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dst-windows/internal/automation"
	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/server"
	"dst-windows/internal/workshop"
	"dst-windows/internal/worldsettings"
)

func (s *Server) StartAutomation() func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if s.automationErr != nil {
			log.Printf("自动维护未启动: %v", s.automationErr)
			return
		}
		s.automation.Serve(ctx, s.runMaintenanceWithNotification, func(err error) { log.Printf("自动维护: %v", err) })
	}()
	return func() { cancel(); <-done }
}

func (s *Server) runMaintenanceWithNotification(ctx context.Context, kind string, cfg automation.Config) (string, error) {
	result, err := s.runMaintenance(ctx, kind, cfg)
	if err != nil && ctx.Err() == nil {
		event := "maintenance_failed"
		if kind == "backup" {
			event = "backup_failed"
		}
		s.publishNotification(event, 0, "", kind+": "+err.Error())
	}
	return result, err
}

func (s *Server) handleAutomation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.automationErr != nil {
		respond(w, 200, nil, fmt.Errorf("读取 automation.json 失败: %w", s.automationErr))
		return
	}
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, s.automation.Snapshot(), nil)
	case http.MethodPut:
		var c automation.Config
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&c); err != nil {
			respond(w, 200, nil, err)
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			respond(w, 200, nil, fmt.Errorf("仅接受一份计划配置"))
			return
		}
		if err := s.automation.Configure(c, time.Now()); err != nil {
			respond(w, 200, nil, err)
			return
		}
		respond(w, 200, s.automation.Snapshot(), nil)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// Older command endpoints use GET; they must also be excluded during maintenance.
func (s *Server) guardMaintenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutation := r.Method != http.MethodGet && r.Method != http.MethodHead
		if r.URL.Path == "/api/migration/upload" || r.URL.Path == "/api/migration/job" {
			mutation = false
		}
		switch r.URL.Path {
		case "/api/start", "/api/stop", "/api/restart", "/api/cmd", "/api/mods/toggle", "/api/mods/config", "/api/mods/delete", "/api/backups/create", "/api/backups/delete", "/api/backups/restore":
			mutation = true
		}
		if mutation {
			if !s.maintenanceMu.TryRLock() {
				respond(w, 200, nil, fmt.Errorf("自动维护正在执行，请完成后再操作"))
				return
			}
			defer s.maintenanceMu.RUnlock()
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleWorldState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	type item struct {
		Shard string                    `json:"shard"`
		State *worldsettings.WorldState `json:"state"`
		Error string                    `json:"error,omitempty"`
	}
	items := []item{}
	for _, shard := range rm.ShardNames() {
		state, err := worldsettings.ReadWorldState(filepath.Join(config.ClusterDir(s.rootDir, s.confDir, rm.ID), shard))
		row := item{Shard: shard, State: state}
		if err != nil {
			row.Error = err.Error()
		}
		items = append(items, row)
	}
	respond(w, 200, items, nil)
}

func pause(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Server) runMaintenance(ctx context.Context, kind string, c automation.Config) (string, error) {
	if !s.maintenanceMu.TryLock() {
		return "", fmt.Errorf("面板操作正在执行，本轮维护已跳过")
	}
	defer s.maintenanceMu.Unlock()
	s.taskMu.Lock()
	err := s.busyTaskError("")
	s.taskMu.Unlock()
	if err != nil {
		return "", err
	}
	rooms := s.rooms.List()
	if kind == "backup" {
		var failures []error
		created := 0
		for _, rm := range rooms {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			hasSave := false
			for _, shard := range rm.Shards {
				hasSave = hasSave || config.ShardHasSave(filepath.Join(config.ClusterDir(s.rootDir, s.confDir, rm.ID), shard.Name))
			}
			if !hasSave {
				continue
			}
			if err := s.saveRoom(ctx, rm); err != nil {
				failures = append(failures, err)
				continue
			}
			if _, err := s.bakFor(rm).CreateAutomatic(c.Keep); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", rm.Name, err))
				continue
			}
			created++
		}
		return fmt.Sprintf("已备份 %d 个房间", created), errors.Join(failures...)
	}
	var updateServer bool
	var mods []string
	if kind == "update" {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		updateServer, err = s.steam.CheckUpdate(checkCtx)
		cancel()
		if err != nil {
			return "", err
		}
		mods, err = s.pendingMods(rooms)
		if err != nil {
			return "", err
		}
		if !updateServer && len(mods) == 0 {
			return "服务端和已启用模组均为最新", nil
		}
		if c.UpdateMode == "notify" {
			message := fmt.Sprintf("发现 %d 个模组更新", len(mods))
			if updateServer {
				message = "服务端有更新；" + message
			}
			return message, nil
		}
		if updateServer && s.steam.IsExternal() {
			return "", fmt.Errorf("检测到服务端更新；外部 Steam 安装请自行更新，未停止任何房间")
		}
	}
	resumeRecovery := s.mgr.PauseRecovery()
	defer resumeRecovery()
	running, err := maintenanceTargets(s.mgr.Status(""))
	if err != nil {
		return "", err
	}
	if kind == "update" && c.UpdateMode == "idle" {
		if err := s.ensureEmpty(ctx, running); err != nil {
			return "", err
		}
	}
	// Announce twice; only shards that were running will be restarted.
	if len(running) > 0 {
		for _, seconds := range []int{60, 30} {
			for _, st := range running {
				if err := s.mgr.SendCmd(st.Cluster, st.Shard, fmt.Sprintf(`c_announce("服务器将在 %d 秒后保存并重启维护")`, seconds)); err != nil {
					return "", err
				}
			}
			if err := pause(ctx, 30*time.Second); err != nil {
				return "", err
			}
		}
	}
	if kind == "update" && c.UpdateMode == "idle" {
		if err := s.ensureEmpty(ctx, running); err != nil {
			return "", err
		}
	}
	for _, rm := range rooms {
		if err := s.saveRoom(ctx, rm); err != nil {
			return "", err
		}
	}
	if kind == "update" && c.UpdateMode == "idle" {
		if err := s.ensureEmpty(ctx, running); err != nil {
			return "", err
		}
	}
	for _, rm := range rooms {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err := s.mgr.Stop(rm.Key(), 30*time.Second); err != nil {
			return "", err
		}
	}
	if err := s.requireNoRunningShards(); err != nil {
		return "", err
	}
	if updateServer {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err := s.steam.Update(); err != nil {
			return "", err
		}
		for s.steam.IsInstalling() {
			if err := pause(ctx, time.Second); err != nil {
				return "", err
			}
		}
		if _, message, _ := s.steam.Progress(); message != "" {
			return "", fmt.Errorf("服务端更新失败，房间保持停止: %s", message)
		}
	}
	for _, id := range mods {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if len(rooms) == 0 {
			break
		}
		if err := s.mgr.UpdateMods(rooms[0].Key(), "", id); err != nil {
			return "", err
		}
		for s.mgr.ModsUpdating() {
			if err := pause(ctx, time.Second); err != nil {
				return "", err
			}
		}
		if status := s.mgr.ModsUpdateStatus(); status.Error != "" {
			return "", fmt.Errorf("模组更新失败，房间保持停止: %s", status.Error)
		}
	}
	for _, st := range running {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err := s.mgr.StartShard(st.Cluster, st.Shard); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("维护完成，已恢复 %d 个分片", len(running)), nil
}

func maintenanceTargets(statuses []server.Status) ([]server.Status, error) {
	var result []server.Status
	for _, st := range statuses {
		if st.State == "starting" || st.State == "stopping" {
			return nil, fmt.Errorf("%s/%s 正在启停，本轮维护已跳过", st.Cluster, st.Shard)
		}
		if st.State == "running" {
			result = append(result, st)
		}
	}
	return result, nil
}

func (s *Server) ensureEmpty(ctx context.Context, targets []server.Status) error {
	for _, st := range targets {
		if err := s.mgr.Maintenance(ctx, st.Cluster, st.Shard, "empty"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) saveRoom(ctx context.Context, rm *room.Room) error {
	targets, err := maintenanceTargets(s.mgr.Status(rm.Key()))
	if err != nil {
		return err
	}
	for _, st := range targets {
		if err := s.mgr.Maintenance(ctx, st.Cluster, st.Shard, "save"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) pendingMods(rooms []*room.Room) ([]string, error) {
	ids := map[string]bool{}
	for _, rm := range rooms {
		enabled, err := s.modsFor(rm).EnabledWorkshopIDs()
		if err != nil {
			return nil, err
		}
		for _, id := range enabled {
			ids[strings.TrimPrefix(id, "workshop-")] = true
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if s.mgr.UgcDir() == "" {
		return nil, fmt.Errorf("共享模组目录未配置")
	}
	local, err := workshop.LoadInstalled(s.mgr.UgcDir())
	if err != nil {
		return nil, err
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Strings(list)
	items, err := workshop.Details(list)
	if err != nil {
		return nil, err
	}
	remote := map[string]int64{}
	for _, item := range items {
		remote[item.ID] = item.TimeUpdated
	}
	var updates []string
	for _, id := range list {
		if remote[id] <= 0 {
			return nil, fmt.Errorf("无法确认模组 %s 的远端版本", id)
		}
		if local[id].TimeUpdated == 0 || remote[id] > local[id].TimeUpdated {
			updates = append(updates, id)
		}
	}
	return updates, nil
}
