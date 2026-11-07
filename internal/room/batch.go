package room

import "fmt"

// CreateBatch prepares all new directories before publishing any room definitions.
// prepare must undo its own filesystem writes if it returns an error.
func (s *Store) CreateBatch(templates []*Room, prepare func([]*Room) error, rollback func() error) ([]*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	original := append([]*Room(nil), s.rooms...)
	created := make([]*Room, 0, len(templates))
	for _, template := range templates {
		r := clone(template)
		r.ID = s.nextID()
		r.ClusterKey = RandomKey(14)
		r.MasterIP = "127.0.0.1"
		r.Normalize()
		r.MasterPort = 0
		for i := range r.Shards {
			r.Shards[i].ServerPort, r.Shards[i].MasterServerPort, r.Shards[i].AuthenticationPort = 0, 0, 0
		}
		s.allocPorts(r, 0)
		if err := r.Validate(); err != nil {
			s.rooms = original
			return nil, err
		}
		if conflicts := r.ConflictPorts(s.rooms, 0); len(conflicts) > 0 {
			s.rooms = original
			return nil, fmt.Errorf("端口冲突: %v", conflicts)
		}
		s.rooms = append(s.rooms, r)
		created = append(created, clone(r))
	}
	if err := prepare(created); err != nil {
		s.rooms = original
		return nil, err
	}
	if err := s.save(); err != nil {
		s.rooms = original
		if cleanup := rollback(); cleanup != nil {
			return nil, fmt.Errorf("%w；清理新房间失败: %v", err, cleanup)
		}
		return nil, err
	}
	return created, nil
}
