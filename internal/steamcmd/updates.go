package steamcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// CheckUpdate uses Valve's public game-version endpoint, not the Steam depot build ID.
func (m *Manager) CheckUpdate(ctx context.Context) (bool, error) {
	version := m.Version()
	if _, err := strconv.ParseUint(version, 10, 32); err != nil {
		return false, fmt.Errorf("本地游戏版本不可用")
	}
	endpoint := "https://api.steampowered.com/ISteamApps/UpToDateCheck/v1/?" + url.Values{"appid": {"322330"}, "version": {version}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Steam 更新检测返回 HTTP %d", resp.StatusCode)
	}
	return parseUpdate(io.LimitReader(resp.Body, 64*1024))
}

func parseUpdate(r io.Reader) (bool, error) {
	var body struct {
		Response struct {
			Success         bool   `json:"success"`
			UpToDate        *bool  `json:"up_to_date"`
			RequiredVersion uint64 `json:"required_version"`
		} `json:"response"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return false, err
	}
	if !body.Response.Success || body.Response.UpToDate == nil {
		return false, fmt.Errorf("Steam 未返回有效版本检查结果")
	}
	if !*body.Response.UpToDate && body.Response.RequiredVersion == 0 {
		return false, fmt.Errorf("Steam 未返回目标版本")
	}
	return !*body.Response.UpToDate, nil
}
