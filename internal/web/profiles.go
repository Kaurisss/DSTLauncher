package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dst-windows/internal/config"
	"dst-windows/internal/room"
	"dst-windows/internal/roomprofile"
	"dst-windows/internal/server"
)

func readJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("读取请求失败: %w", err)
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("只接受一份 JSON 对象")
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("解析请求体失败: %w", err)
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("只接受一份 JSON 对象")
	}
	return nil
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	store := roomprofile.New(filepath.Join(s.rootDir, "profiles"))
	switch r.Method {
	case http.MethodGet:
		if id := query(r, "id"); id != "" {
			p, err := store.Load(id)
			if err != nil {
				respond(w, 200, nil, err)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="dst-profile-`+p.ID+`.json"`)
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(p)
			return
		}
		items, err := store.List()
		respond(w, 200, items, err)
	case http.MethodPost:
		var in struct {
			Room int    `json:"room"`
			Name string `json:"name"`
		}
		if err := readJSON(w, r, &in, 4096); err != nil {
			respond(w, 200, nil, err)
			return
		}
		s.taskMu.Lock()
		defer s.taskMu.Unlock()
		p, err := s.captureProfile(in.Room, in.Name)
		if err == nil {
			p, err = store.Save(p)
		}
		respond(w, 200, p, err)
	case http.MethodDelete:
		s.profileMu.Lock()
		defer s.profileMu.Unlock()
		respond(w, 200, nil, store.Delete(query(r, "id")))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) captureProfile(id int, name string) (*roomprofile.Profile, error) {
	rm, ok := s.rooms.Get(id)
	if !ok {
		return nil, fmt.Errorf("房间不存在")
	}
	if s.mgr.AnyRunning(rm.Key()) {
		return nil, fmt.Errorf("请先停止源房间，再复制或保存配置方案")
	}
	return roomprofile.Capture(rm.ClusterDir(s.rootDir, s.confDir), rm, name)
}

func (s *Server) handleProfileImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p roomprofile.Profile
	if err := readJSON(w, r, &p, roomprofile.MaxSize); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	result, err := roomprofile.New(filepath.Join(s.rootDir, "profiles")).Save(&p)
	respond(w, 200, result, err)
}

func (s *Server) handleProfileApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		ID   string `json:"id"`
		Room int    `json:"room"`
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in, 4096); err != nil {
		respond(w, 200, nil, err)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		respond(w, 200, nil, fmt.Errorf("请填写新房间名称"))
		return
	}
	if err := rejectINIValue(in.Name); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	var p *roomprofile.Profile
	var err error
	if r.URL.Path == "/api/rooms/copy" {
		p, err = s.captureProfile(in.Room, in.Name)
	} else {
		p, err = roomprofile.New(filepath.Join(s.rootDir, "profiles")).Load(in.ID)
	}
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	p.Room.Name = strings.TrimSpace(in.Name)
	created, err := s.rooms.Create(p.Room)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	dir := created.ClusterDir(s.rootDir, s.confDir)
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		respond(w, 200, nil, errors.Join(err, s.rooms.Delete(created.ID)))
		return
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		respond(w, 200, nil, errors.Join(fmt.Errorf("新房间目录已存在或无法创建: %w", err), s.rooms.Delete(created.ID)))
		return
	}
	// 先恢复世界定义，Generate 才能正确判断是否需要补默认预设。
	var writeErr error
	for shard, files := range p.Files {
		for name, text := range files {
			if writeErr = config.WriteFileAtomic(filepath.Join(dir, shard, name), []byte(text)); writeErr != nil {
				break
			}
		}
		if writeErr != nil {
			break
		}
	}
	if writeErr == nil {
		writeErr = room.Generate(s.rootDir, s.confDir, created)
	}
	if writeErr != nil {
		respond(w, 200, nil, errors.Join(writeErr, s.rooms.Delete(created.ID), os.RemoveAll(dir)))
		return
	}
	s.mgr.Register(server.NewCluster(created))
	s.syncModsSetup()
	respond(w, 200, created, nil)
}

func rejectINIValue(value string) error {
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("房间名称不能包含换行或空字符")
	}
	return nil
}
