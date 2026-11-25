package backupremote

import (
	"cmp"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
)

func (e *Engine) davRequest(ctx context.Context, c Config, method, target string, body io.Reader, headers map[string]string, length int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("无法创建 WebDAV 请求")
	}
	if c.Username != "" || c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.ContentLength = length
	}
	res, err := e.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("WebDAV 连接失败或超时")
	}
	return res, nil
}
func davStatus(res *http.Response, method string, allowed ...int) error {
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	for _, status := range allowed {
		if res.StatusCode == status {
			return nil
		}
	}
	return fmt.Errorf("WebDAV %s 返回 HTTP %d", method, res.StatusCode)
}
func (e *Engine) davDirectory(ctx context.Context, c Config, room int) (string, error) {
	base, err := url.JoinPath(c.URL, "dstlauncher-"+e.namespace)
	if err != nil {
		return "", err
	}
	targets := []string{base}
	if room > 0 {
		dir, err := url.JoinPath(base, fmt.Sprintf("Cluster_%d", room))
		if err != nil {
			return "", err
		}
		targets = append(targets, dir)
	}
	for _, target := range targets {
		res, err := e.davRequest(ctx, c, "MKCOL", target+"/", nil, nil, 0)
		if err != nil {
			return "", err
		}
		if err = davStatus(res, "MKCOL", 201, 405); err != nil {
			return "", err
		}
	}
	return targets[len(targets)-1] + "/", nil
}
func (e *Engine) davWrite(ctx context.Context, c Config, target string, reader io.Reader, size int64) error {
	res, err := e.davRequest(ctx, c, http.MethodPut, target, reader, map[string]string{"Content-Type": "application/zip"}, size)
	if err != nil {
		return err
	}
	return davStatus(res, "PUT", 200, 201, 204)
}
func (e *Engine) davDelete(ctx context.Context, c Config, target string) error {
	res, err := e.davRequest(ctx, c, http.MethodDelete, target, nil, nil, 0)
	if err != nil {
		return err
	}
	return davStatus(res, "DELETE", 200, 204, 404)
}
func (e *Engine) syncDAV(ctx context.Context, c Config, room int, name string, file *os.File) (err error) {
	dir, err := e.davDirectory(ctx, c, room)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	temp := dir + ".upload-" + name
	defer func() {
		if err != nil {
			_ = e.davDelete(ctx, c, temp)
		}
	}()
	if err = e.davWrite(ctx, c, temp, file, info.Size()); err != nil {
		return err
	}
	res, err := e.davRequest(ctx, c, "MOVE", temp, nil, map[string]string{"Destination": dir + url.PathEscape(name), "Overwrite": "T"}, 0)
	if err != nil {
		return err
	}
	if err = davStatus(res, "MOVE", 201, 204); err != nil {
		return err
	}
	return e.pruneDAV(ctx, c, dir, room, name)
}

type davMultiStatus struct {
	Responses []struct {
		Href  string `xml:"href"`
		Props []struct {
			Status string `xml:"status"`
			Prop   struct {
				Type struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func (e *Engine) pruneDAV(ctx context.Context, c Config, dir string, room int, current string) error {
	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`
	res, err := e.davRequest(ctx, c, "PROPFIND", dir, strings.NewReader(body), map[string]string{"Depth": "1", "Content-Type": "application/xml; charset=utf-8"}, int64(len(body)))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 207 {
		return fmt.Errorf("同步已完成，WebDAV PROPFIND 返回 HTTP %d", res.StatusCode)
	}
	reader := io.LimitReader(res.Body, 8<<20+1)
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if len(b) > 8<<20 {
		return fmt.Errorf("WebDAV 目录清单过大")
	}
	var status davMultiStatus
	if err = xml.Unmarshal(b, &status); err != nil {
		return fmt.Errorf("WebDAV 目录清单不是有效 XML")
	}
	dirURL, _ := url.Parse(dir)
	names := []string{}
	seen := map[string]bool{}
	for _, response := range status.Responses {
		href, err := url.Parse(response.Href)
		if err != nil {
			continue
		}
		href = dirURL.ResolveReference(href)
		if href.IsAbs() && (href.Scheme != dirURL.Scheme || href.Host != dirURL.Host) {
			continue
		}
		filePath := path.Clean(href.Path)
		if path.Dir(filePath) != strings.TrimSuffix(dirURL.Path, "/") {
			continue
		}
		name := path.Base(filePath)
		if !ownBackupName.MatchString(name) || !strings.HasPrefix(name, fmt.Sprintf("Cluster_%d_", room)) || seen[name] {
			continue
		}
		regular := false
		for _, property := range response.Props {
			if strings.Contains(property.Status, " 200 ") && property.Prop.Type.Collection == nil {
				regular = true
			}
		}
		if regular {
			names = append(names, name)
			seen[name] = true
		}
	}
	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(backupOrder(b), backupOrder(a)) })
	retained := 1
	for _, name := range names {
		if name == current {
			continue
		}
		if retained < c.Keep {
			retained++
			continue
		}
		if err = e.davDelete(ctx, c, dir+url.PathEscape(name)); err != nil {
			return fmt.Errorf("同步已完成，清理旧 WebDAV 备份失败: %w", err)
		}
	}
	return nil
}
