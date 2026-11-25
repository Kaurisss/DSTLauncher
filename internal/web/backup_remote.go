package web

import (
	"fmt"
	"net/http"

	"dst-windows/internal/backup"
	"dst-windows/internal/backupremote"
)

func (s *Server) StartBackupRemote() func() {
	if s.backupRemoteErr != nil {
		return func() {}
	}
	return s.backupRemote.Start()
}
func (s *Server) queueRemoteBackup(room int, item *backup.BackupItem) (*backupremote.Result, error) {
	if s.backupRemoteErr != nil {
		return nil, s.backupRemoteErr
	}
	return s.backupRemote.Enqueue(room, item.Name, item.Path, false)
}
func (s *Server) handleBackupRemote(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.backupRemoteErr != nil {
		respond(w, 200, nil, s.backupRemoteErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, s.backupRemote.Snapshot(), nil)
	case http.MethodPut:
		var in struct {
			Enabled   bool    `json:"enabled"`
			Kind      string  `json:"kind"`
			Directory string  `json:"directory"`
			URL       string  `json:"url"`
			Username  string  `json:"username"`
			Password  *string `json:"password"`
			Keep      int     `json:"keep"`
		}
		if err := readJSON(w, r, &in, 16384); err != nil {
			respond(w, 200, nil, err)
			return
		}
		err := s.backupRemote.Configure(backupremote.Config{Enabled: in.Enabled, Kind: in.Kind, Directory: in.Directory, URL: in.URL, Username: in.Username, Keep: in.Keep}, in.Password)
		respond(w, 200, s.backupRemote.Snapshot(), err)
	}
}
func (s *Server) handleBackupRemoteTest(w http.ResponseWriter, r *http.Request) {
	if s.backupRemoteErr != nil {
		respond(w, 200, nil, s.backupRemoteErr)
		return
	}
	respond(w, 200, nil, s.backupRemote.Test(r.Context()))
}
func (s *Server) handleBackupRemoteSync(w http.ResponseWriter, r *http.Request) {
	if s.backupRemoteErr != nil {
		respond(w, 200, nil, s.backupRemoteErr)
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
	if err := readJSON(w, r, &in, 4096); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	file, err := s.bakFor(rm).ArchivePath(in.Name)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	result, err := s.backupRemote.Enqueue(rm.ID, in.Name, file, true)
	if err != nil {
		respond(w, 200, nil, fmt.Errorf("未加入异地同步队列: %w", err))
		return
	}
	respond(w, 200, result, nil)
}
