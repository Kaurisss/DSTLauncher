package web

import (
	"net/http"
	"path/filepath"

	"dst-windows/internal/hoststats"
)

func (s *Server) handleHostStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, hoststats.Read(filepath.Join(s.rootDir, s.confDir)), nil)
}
