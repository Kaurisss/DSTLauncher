package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dst-windows/internal/room"
)

const publicIPCacheTTL = 10 * time.Minute

type publicIPState struct {
	IP        string    `json:"ip"`
	Source    string    `json:"source,omitempty"`
	FetchedAt time.Time `json:"fetchedAt,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// defaultPublicIPProbes 按顺序使用轻量的纯文本公网 IPv4 探测源（含国内外节点），
// 全部返回纯文本 IP，并严格通过 netip.ParseAddr 校验。
var defaultPublicIPProbes = []struct {
	name string
	url  string
}{
	{name: "ipw.cn", url: "https://4.ipw.cn"},
	{name: "ipify", url: "https://api.ipify.org"},
	{name: "icanhazip", url: "https://ipv4.icanhazip.com"},
	{name: "ifconfig.me", url: "https://ifconfig.me/ip"},
}

func (s *Server) probePublicIP(ctx context.Context) publicIPState {
	if s.publicIPFetcher != nil {
		return s.publicIPFetcher(ctx)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	type probeRes struct {
		ip     string
		source string
	}
	ch := make(chan probeRes, len(defaultPublicIPProbes))
	for _, p := range defaultPublicIPProbes {
		go func(name, endpoint string) {
			req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
			if err != nil {
				ch <- probeRes{}
				return
			}
			req.Header.Set("User-Agent", "dst-launcher")
			resp, err := client.Do(req)
			if err != nil {
				ch <- probeRes{}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				ch <- probeRes{}
				return
			}
			raw, err := io.ReadAll(io.LimitReader(resp.Body, 128))
			if err != nil {
				ch <- probeRes{}
				return
			}
			text := strings.TrimSpace(string(raw))
			addr, err := netip.ParseAddr(text)
			if err != nil {
				ch <- probeRes{}
				return
			}
			ch <- probeRes{ip: addr.Unmap().String(), source: name}
		}(p.name, p.url)
	}

	for range defaultPublicIPProbes {
		select {
		case res := <-ch:
			if res.ip != "" {
				cancel()
				return publicIPState{
					IP:        res.ip,
					Source:    res.source,
					FetchedAt: time.Now().UTC(),
				}
			}
		case <-probeCtx.Done():
			return publicIPState{
				FetchedAt: time.Now().UTC(),
				Error:     "外网 IP 自动探测超时，可点击刷新或使用自定义地址",
			}
		}
	}
	return publicIPState{
		FetchedAt: time.Now().UTC(),
		Error:     "未能自动探测外网 IP，可点击刷新或使用自定义地址",
	}
}

// handlePublicIP 返回自动探测到的公网 IP（带 10 分钟缓存，?refresh=1 强制重新探测）。
func (s *Server) handlePublicIP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	refresh := query(r, "refresh") == "1"

	s.publicIPMu.Lock()
	if !refresh && s.publicIPCache.IP != "" && time.Since(s.publicIPCache.FetchedAt) < publicIPCacheTTL {
		cached := s.publicIPCache
		s.publicIPMu.Unlock()
		respond(w, 200, cached, nil)
		return
	}
	s.publicIPMu.Unlock()

	res := s.probePublicIP(r.Context())

	s.publicIPMu.Lock()
	if res.IP != "" {
		s.publicIPCache = res
	} else if !refresh && s.publicIPCache.IP != "" {
		// 缓存过期但重新探测失败时，回退上次可用值并附带提示
		res = s.publicIPCache
	}
	s.publicIPMu.Unlock()

	respond(w, 200, res, nil)
}

// handleRoomConnect 更新单个房间的自定义直连地址与端口，无需重启游戏进程。
func (s *Server) handleRoomConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rm, err := s.roomByID(r)
	if err != nil {
		respond(w, 200, nil, err)
		return
	}
	var payload struct {
		ConnectIP    string         `json:"connectIp"`
		ConnectPort  int            `json:"connectPort"`
		ConnectPorts map[string]int `json:"connectPorts"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		respond(w, 200, nil, fmt.Errorf("直连配置参数无效: %w", err))
		return
	}
	if dec.Decode(new(any)) != io.EOF {
		respond(w, 200, nil, fmt.Errorf("直连配置只能包含一份参数"))
		return
	}
	if err := room.ValidateConnect(payload.ConnectIP, payload.ConnectPort, payload.ConnectPorts); err != nil {
		respond(w, 200, nil, err)
		return
	}
	updated, err := s.rooms.UpdateConnect(rm.ID, payload.ConnectIP, payload.ConnectPort, payload.ConnectPorts)
	respond(w, 200, updated, err)
}
