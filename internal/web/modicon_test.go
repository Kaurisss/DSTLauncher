package web

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dst-windows/internal/room"
)

func TestModIconHTTP(t *testing.T) {
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.mgr.UgcDir(), "content", "322330", "123")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte("name='本地图片测试'\nversion='1.2.3'"), 0644); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 18+256*128*4)
	copy(data, "KTEX")
	binary.LittleEndian.PutUint32(data[4:], 0xfffc2240)
	binary.LittleEndian.PutUint16(data[8:], 256)
	binary.LittleEndian.PutUint16(data[10:], 128)
	binary.LittleEndian.PutUint32(data[14:], uint32(len(data)-18))
	for i := 18; i < len(data); i += 4 {
		copy(data[i:], []byte{25, 50, 100, 255})
	}
	texture := filepath.Join(dir, "modicon.tex")
	if err := os.WriteFile(texture, data, 0644); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	_, body := doJSON(t, h, http.MethodGet, fmt.Sprintf("/api/mods?room=%d", rm.ID), nil, nil)
	items := body["data"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("模组列表：%v", body)
	}
	iconURL := items[0].(map[string]interface{})["iconUrl"].(string)
	if items[0].(map[string]interface{})["version"] != "1.2.3" {
		t.Fatalf("列表缺少本地版本号: %v", items[0])
	}
	request := func(method, path, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := request(http.MethodGet, iconURL, "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("图标响应：%d, %v", rec.Code, rec.Header())
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 128 || img.Bounds().Dy() != 64 || color.NRGBAModel.Convert(img.At(0, 0)) != (color.NRGBA{R: 25, G: 50, B: 100, A: 255}) {
		t.Fatalf("缩略图应保留比例与颜色：%v", img)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("应提供图标 ETag")
	}
	if rec := request(http.MethodGet, iconURL, etag); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("未改变的图标应返回 304：%d", rec.Code)
	}
	if rec := request(http.MethodHead, iconURL, ""); rec.Code != http.StatusOK || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
		t.Fatalf("HEAD 响应：%d", rec.Code)
	}
	for i := 18; i < len(data); i += 4 {
		data[i] = 200
	}
	if err := os.WriteFile(texture, data, 0644); err != nil {
		t.Fatal(err)
	}
	if rec := request(http.MethodGet, iconURL, etag); rec.Code != http.StatusOK || rec.Header().Get("ETag") == etag {
		t.Fatalf("模组更新后图标缓存应失效：%d", rec.Code)
	}
	_, refreshed := doJSON(t, h, http.MethodGet, fmt.Sprintf("/api/mods?room=%d", rm.ID), nil, nil)
	newURL := refreshed["data"].([]interface{})[0].(map[string]interface{})["iconUrl"].(string)
	if newURL == iconURL {
		t.Fatal("刷新列表后应提供新版本的图标 URL")
	}
	for _, path := range []string{"/api/mods/icon?room=999&id=workshop-123", "/api/mods/icon?id=..%2f123", "/api/mods/icon?id=missing"} {
		if rec := request(http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s：%d，期望 404", path, rec.Code)
		}
	}
	if rec := request(http.MethodPost, iconURL, ""); rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("不应接受写入请求：%d", rec.Code)
	}
	s.envPassword = "test-password"
	if rec := request(http.MethodGet, iconURL, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("本地图标也应受登录保护：%d", rec.Code)
	}
}
