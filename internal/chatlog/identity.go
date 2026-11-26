package chatlog

import (
	"encoding/json"
	"net"
	"regexp"
	"strings"
	"time"
)

type identity struct {
	Names          map[string]bool  `json:"names"`
	LastName       string           `json:"lastName,omitempty"`
	LastNameAt     time.Duration    `json:"lastNameAt,omitempty"`
	LastNameDirect bool             `json:"lastNameDirect,omitempty"`
	SteamID        string           `json:"steamid,omitempty"`
	IP             string           `json:"ip,omitempty"`
	Prefab         string           `json:"prefab,omitempty"`
	Changes        []identityChange `json:"changes,omitempty"`
}

type identityChange struct {
	At      time.Duration `json:"at"`
	SteamID string        `json:"steamid,omitempty"`
	IP      string        `json:"ip,omitempty"`
	Prefab  string        `json:"prefab,omitempty"`
}

var (
	serverPrefix = regexp.MustCompile(`^\[([0-9]+:[0-5][0-9]:[0-5][0-9])\]:\s*(.*)$`)
	authLine     = regexp.MustCompile(`^Client authenticated: \((KU_[A-Za-z0-9_-]+)\) (.+)$`)
	connectLine  = regexp.MustCompile(`^(?:New incoming connection|Client connected from)\s+(?:\[LAN\]\s+)?(.+?)\|[0-9]+\s+<([0-9]+)>`)
	clientLine   = regexp.MustCompile(`^\[ClientObject\] Initialized \(authenticated\) on server: guid=([0-9]+) userid=(KU_[A-Za-z0-9_-]+) netid=([0-9]*)`)
	ownership    = regexp.MustCompile(`^User ID\s+(KU_[A-Za-z0-9_-]+)\s+assigned ownership to entity\s+[0-9]+\s+-\s+([A-Za-z0-9_]+)`)
	spawnLine    = regexp.MustCompile(`^Spawn request:\s*([A-Za-z0-9_]+)\s+from\s+(.+)$`)
	panelReply   = regexp.MustCompile(`^DST_PANEL_PLAYERS_[0-9a-f]+:(\{.*\})$`)
)

func (s *historyState) observeServer(line string) {
	parts := serverPrefix.FindStringSubmatch(strings.TrimSpace(line))
	if parts == nil {
		return
	}
	body := parts[2]
	at := uptime(parts[1])
	if text, ok := strings.CutPrefix(body, "Current time: "); ok {
		if at, err := time.ParseInLocation(time.ANSIC, text, time.Local); err == nil {
			s.StartedAt = at.Add(-uptime(parts[1]))
		}
	}
	if m := connectLine.FindStringSubmatch(body); m != nil {
		address := strings.Trim(m[1], "[]")
		if ip := net.ParseIP(address); ip != nil {
			s.Connections[m[2]] = ip.String()
		}
	}
	if m := authLine.FindStringSubmatch(body); m != nil {
		p := s.player(m[1])
		p.LastName = strings.TrimSpace(m[2])
		p.Names[p.LastName] = true
		p.LastNameAt = at
		s.putPlayer(m[1], p, at)
	}
	if m := clientLine.FindStringSubmatch(body); m != nil {
		p := s.player(m[2])
		p.SteamID = m[3]
		p.IP = s.Connections[m[1]]
		s.putPlayer(m[2], p, at)
	}
	if m := ownership.FindStringSubmatch(body); m != nil {
		p := s.player(m[1])
		p.Prefab = m[2]
		s.putPlayer(m[1], p, at)
	}
	if m := spawnLine.FindStringSubmatch(body); m != nil {
		name := strings.TrimSpace(m[2])
		s.Spawns[name] = append(s.Spawns[name], identityChange{At: at, Prefab: m[1]})
	}
	if m := panelReply.FindStringSubmatch(body); m != nil {
		var reply struct {
			Player *struct {
				UserID string `json:"userid"`
				Name   string `json:"name"`
				Prefab string `json:"prefab"`
			} `json:"player"`
		}
		if json.Unmarshal([]byte(m[1]), &reply) == nil && reply.Player != nil && strings.HasPrefix(reply.Player.UserID, "KU_") {
			p := s.player(reply.Player.UserID)
			p.LastName = reply.Player.Name
			p.Names[reply.Player.Name] = true
			p.LastNameAt = at
			p.LastNameDirect = true
			p.Prefab = reply.Player.Prefab
			s.putPlayer(reply.Player.UserID, p, at)
		}
	}
}

func (s *historyState) player(id string) identity {
	p := s.Players[id]
	if p.Names == nil {
		p.Names = map[string]bool{}
	}
	return p
}

func (s *historyState) putPlayer(id string, p identity, at time.Duration) {
	previous := s.Players[id]
	if len(p.Changes) == 0 || p.SteamID != previous.SteamID || p.IP != previous.IP || p.Prefab != previous.Prefab {
		p.Changes = append(p.Changes, identityChange{At: at, SteamID: p.SteamID, IP: p.IP, Prefab: p.Prefab})
	}
	s.Players[id] = p
}

func (p identity) at(at time.Duration) identityChange {
	var result identityChange
	future := 0
	for future < len(p.Changes) && p.Changes[future].At <= at {
		result = p.Changes[future]
		future++
	}
	// A join announcement can precede its connection record; fill missing connection fields only.
	for _, change := range p.Changes[future:] {
		if result.SteamID == "" {
			result.SteamID = change.SteamID
		}
		if result.IP == "" {
			result.IP = change.IP
		}
		if result.SteamID != "" && result.IP != "" {
			break
		}
	}
	return result
}

func (s *historyState) roleAt(id, name string, at time.Duration) string {
	sources := [][]identityChange{s.Players[id].Changes}
	if id != "" && s.identityFor(name) == id {
		sources = [][]identityChange{s.Spawns[name], s.Players[id].Changes}
	}
	var before, after time.Duration
	var past, future string
	for _, changes := range sources {
		for _, change := range changes {
			if change.Prefab == "" {
				continue
			}
			if change.At <= at {
				if past == "" || change.At >= before {
					past, before = change.Prefab, change.At
				}
			} else if future == "" || change.At <= after {
				future, after = change.Prefab, change.At
			}
		}
	}
	if past != "" {
		return past
	}
	return future
}

func (s *historyState) announcementPlayer(text string) (string, string) {
	name, message := "", text
	for _, player := range s.Players {
		for candidate := range player.Names {
			rest, matches := strings.CutPrefix(text, candidate+" ")
			if !matches || candidate == "" || len(candidate) <= len(name) {
				continue
			}
			for _, separator := range announcementSeparators {
				if strings.HasPrefix(rest, separator) {
					name, message = candidate, rest
					break
				}
			}
		}
	}
	return name, message
}

func (s *historyState) identityFor(name string) string {
	if name == "" {
		return ""
	}
	id := ""
	for userID, p := range s.Players {
		if p.Names[name] {
			// A nickname shared by different accounts must never become a ban target.
			if id != "" && id != userID {
				return ""
			}
			id = userID
		}
	}
	return id
}

func (s *historyState) enrich() {
	for i := range s.Entries {
		r := &s.Entries[i]
		if r.Session != s.Server.Session {
			continue
		}
		before := r.Entry
		if r.Announcement != "" {
			r.Name, r.Message = s.announcementPlayer(r.Announcement)
		}
		if !r.DirectID {
			r.UserID = s.identityFor(r.Name)
		}
		p := s.Players[r.UserID].at(uptime(r.Time))
		r.SteamID, r.IP = p.SteamID, p.IP
		r.Prefab = s.roleAt(r.UserID, r.Name, uptime(r.Time))
		if !s.StartedAt.IsZero() {
			at := s.StartedAt.Add(uptime(r.Time))
			if r.RecordedAt == nil || !r.RecordedAt.Equal(at) {
				r.RecordedAt = &at
			}
		}
		if r.Entry != before {
			s.dirty = true
		}
	}
}

func uptime(text string) time.Duration {
	clock := strings.Replace(text, ":", "h", 1)
	clock = strings.Replace(clock, ":", "m", 1) + "s"
	value, _ := time.ParseDuration(clock)
	return value
}
