package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"dst-windows/internal/server"
)

func (s *Server) handleQuickAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if query(r, "room") == "" {
		respond(w, 200, nil, fmt.Errorf("请明确指定要操作的房间"))
		return
	}
	var action server.QuickAction
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&action); err != nil {
		respond(w, 200, nil, fmt.Errorf("快捷操作参数无效: %w", err))
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		respond(w, 200, nil, fmt.Errorf("快捷操作只能包含一份参数"))
		return
	}
	if err := action.Validate(); err != nil {
		respond(w, 200, nil, err)
		return
	}
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	result, err := s.mgr.Quick(r.Context(), rm.Key(), action)
	respond(w, 200, result, err)
}
