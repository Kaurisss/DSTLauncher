package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// go:embed 里的文件没有修改时间，http.FileServer 因此不会发 Last-Modified/ETag，
// 浏览器只能每次都重新下载。世界配置页一次要 120+ 张图标，必须给强缓存。
func TestStaticCacheHeaders(t *testing.T) {
	staticFS := fstest.MapFS{
		"index.html":               {Data: []byte("<html></html>")},
		"world-icons/common/a.png": {Data: []byte("x")},
	}
	s := &Server{staticFS: fs.FS(staticFS)}
	handler := s.Handler()

	cases := map[string]string{
		"/":                         "no-cache",
		"/world-icons/common/a.png": "public, max-age",
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		got := rec.Header().Get("Cache-Control")
		if rec.Code != http.StatusOK {
			t.Errorf("%s 状态码 = %d, 期望 200", path, rec.Code)
			continue
		}
		if want == "no-cache" {
			if got != "no-cache, must-revalidate" {
				t.Errorf("%s 的 Cache-Control = %q, 期望禁用缓存", path, got)
			}
			continue
		}
		if got != "public, max-age=86400" {
			t.Errorf("%s 的 Cache-Control = %q, 期望强缓存", path, got)
		}
	}
}
