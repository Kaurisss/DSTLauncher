package roomprofile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"dst-windows/internal/config"
	"dst-windows/internal/modmgr"
	"dst-windows/internal/room"
	"dst-windows/internal/worldsettings"
)

const MaxSize = 8 << 20

var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var FileNames = []string{worldsettings.OverrideFileName, worldsettings.LevelDataFileName, "modoverrides.lua"}

type Profile struct {
	Version   int                          `json:"version"`
	ID        string                       `json:"id"`
	Name      string                       `json:"name"`
	CreatedAt time.Time                    `json:"createdAt"`
	Room      *room.Room                   `json:"room"`
	Files     map[string]map[string]string `json:"files"`
}

type Summary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	Shards    int       `json:"shards"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Store { return &Store{dir: dir} }

func sanitize(r *room.Room) *room.Room {
	c := *r
	c.Shards = append([]room.Shard(nil), r.Shards...)
	c.ID, c.MasterPort = 0, 0
	c.Token, c.Password, c.ClusterKey = "", "", ""
	c.MasterIP = "127.0.0.1"
	for i := range c.Shards {
		c.Shards[i].ServerPort = 0
		c.Shards[i].MasterServerPort = 0
		c.Shards[i].AuthenticationPort = 0
	}
	return &c
}

func Capture(clusterDir string, r *room.Room, name string) (*Profile, error) {
	p := &Profile{Version: 1, Name: name, Room: sanitize(r), Files: map[string]map[string]string{}}
	root, err := os.OpenRoot(clusterDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, shard := range r.Shards {
		files := map[string]string{}
		for _, file := range FileNames {
			f, err := root.Open(filepath.Join(shard.Name, file))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			info, err := f.Stat()
			if err != nil || !info.Mode().IsRegular() || info.Size() > MaxSize {
				f.Close()
				return nil, fmt.Errorf("%s/%s 不是可读取的配置文件", shard.Name, file)
			}
			data, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
			f.Close()
			if err != nil {
				return nil, err
			}
			if len(data) > MaxSize {
				return nil, fmt.Errorf("配置文件过大")
			}
			files[file] = string(data)
		}
		p.Files[shard.Name] = files
	}
	return p, p.Validate()
}

func (p *Profile) Validate() error {
	if p.Version != 1 || p.Room == nil {
		return fmt.Errorf("不支持的配置方案格式")
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len([]rune(p.Name)) > 80 {
		return fmt.Errorf("方案名称须为 1–80 个字符")
	}
	p.Room = sanitize(p.Room)
	if strings.ContainsAny(p.Room.Name+p.Room.Description, "\r\n\x00") {
		return fmt.Errorf("房间名称和描述不能包含换行或空字符")
	}
	check := *p.Room
	check.Shards = append([]room.Shard(nil), p.Room.Shards...)
	check.Normalize()
	if err := room.ValidateTemplate(&check); err != nil {
		return err
	}
	// Validate original shard names too: normalization must not silently rename imported paths.
	allowed := map[string]bool{}
	for _, shard := range check.Shards {
		allowed[shard.Name] = true
	}
	for shard, files := range p.Files {
		if !allowed[shard] {
			return fmt.Errorf("方案包含未知分片: %s", shard)
		}
		for name, text := range files {
			var err error
			switch name {
			case "modoverrides.lua":
				_, err = modmgr.ParseOverridesChecked(text)
			case worldsettings.OverrideFileName:
				_, err = worldsettings.ParseOverrideChecked(text)
			case worldsettings.LevelDataFileName:
				ld := worldsettings.ParseLevelData(text)
				_, err = ld.WithOverrides(nil)
				if err == nil && ld.Location() == "" {
					err = fmt.Errorf("缺少世界位置")
				}
			default:
				return fmt.Errorf("方案不允许包含文件: %s", name)
			}
			if err != nil {
				return fmt.Errorf("%s/%s: %w", shard, name, err)
			}
		}
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > MaxSize {
		return fmt.Errorf("方案不能超过 8 MiB")
	}
	return nil
}

func (s *Store) Save(p *Profile) (*Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := p.Validate(); err != nil {
		return nil, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	p.ID, p.CreatedAt = hex.EncodeToString(id[:]), time.Now()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSize {
		return nil, fmt.Errorf("方案不能超过 8 MiB")
	}
	if err := config.WriteFileAtomic(filepath.Join(s.dir, p.ID+".json"), data); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) Load(id string) (*Profile, error) {
	if !validID.MatchString(id) {
		return nil, fmt.Errorf("方案 ID 不合法")
	}
	f, err := os.Open(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var p Profile
	if err := json.NewDecoder(io.LimitReader(f, MaxSize+1)).Decode(&p); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) List() ([]Summary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	result := []Summary{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		p, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, fmt.Errorf("读取 %s: %w", entry.Name(), err)
		}
		result = append(result, Summary{p.ID, p.Name, p.CreatedAt, len(p.Room.Shards)})
	}
	slices.SortFunc(result, func(a, b Summary) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return result, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID.MatchString(id) {
		return fmt.Errorf("方案 ID 不合法")
	}
	return os.Remove(filepath.Join(s.dir, id+".json"))
}
