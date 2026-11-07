package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"dst-windows/internal/notify"
)

func (s *Server) publishNotification(event string, roomID int, shard, message string) {
	if s.notificationErr != nil || s.notifications == nil {
		return
	}
	name := ""
	for _, rm := range s.rooms.List() {
		if rm.ID == roomID {
			name = rm.Name
		}
		for _, secret := range []string{rm.Token, rm.Password, rm.ClusterKey} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[已隐藏]")
			}
		}
	}
	s.notifications.Publish(notify.Event{Event: event, RoomID: roomID, RoomName: name, Shard: shard, Message: message, Version: s.version})
}

func (s *Server) StartNotifications() func() {
	if s.notificationErr != nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	id, events := s.mgr.SubscribeEvents("ready", "stop", "crash_limit", "update_failed", "start_failed")
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		s.notifications.Serve(ctx)
	}()
	go func() {
		defer workers.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				typ := event.Type
				switch typ {
				case "ready":
					typ = "started"
				case "stop":
					typ = "stopped"
				case "crash_limit", "update_failed", "start_failed":
				default:
					continue
				}
				roomID, _ := strconv.Atoi(strings.TrimPrefix(event.Cluster, "Cluster_"))
				s.publishNotification(typ, roomID, event.Shard, event.Message)
			}
		}
	}()
	return func() {
		cancel()
		s.mgr.Unsubscribe(id)
		workers.Wait()
	}
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.notificationErr != nil {
		respond(w, 200, nil, fmt.Errorf("读取通知配置失败: %w", s.notificationErr))
		return
	}
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, s.notifications.Snapshot(), nil)
	case http.MethodPut:
		var in struct {
			Enabled bool     `json:"enabled"`
			URL     string   `json:"url"`
			Secret  *string  `json:"secret"`
			Events  []string `json:"events"`
			Rooms   []int    `json:"rooms"`
		}
		if err := readJSON(w, r, &in, 16384); err != nil {
			respond(w, 200, nil, err)
			return
		}
		for _, id := range in.Rooms {
			if _, ok := s.rooms.Get(id); !ok {
				respond(w, 200, nil, fmt.Errorf("房间不存在: %d", id))
				return
			}
		}
		err := s.notifications.Configure(notify.Config{Enabled: in.Enabled, URL: in.URL, Events: in.Events, Rooms: in.Rooms}, in.Secret)
		respond(w, 200, s.notifications.Snapshot(), err)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNotificationTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.notificationErr != nil {
		respond(w, 200, nil, s.notificationErr)
		return
	}
	result := s.notifications.Test(r.Context(), s.version)
	if result.Error != "" {
		respond(w, 200, result, fmt.Errorf("%s", result.Error))
		return
	}
	respond(w, 200, result, nil)
}
