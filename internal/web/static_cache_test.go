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
		"index.html":                 {Data: []byte("<html></html>")},
		"world-icons/common/a.png":   {Data: []byte("x")},
		"assets/index-AbCd1234.js":   {Data: []byte("export default 1")},
		"assets/index-AbCd1234.css":  {Data: []byte("body {}")},
		"assets/font-AbCd1234.woff2": {Data: []byte("font")},
	}
	s := &Server{staticFS: fs.FS(staticFS)}
	handler := s.Handler()

	cases := []struct {
		path   string
		cache  string
		status int
	}{
		{"/", "no-cache, must-revalidate", http.StatusOK},
		{"/world-icons/common/a.png", "public, max-age=86400", http.StatusOK},
		{"/assets/index-AbCd1234.js", "public, max-age=31536000, immutable", http.StatusOK},
		{"/assets/index-AbCd1234.css?v=1", "public, max-age=31536000, immutable", http.StatusOK},
		{"/assets/font-AbCd1234.woff2", "public, max-age=31536000, immutable", http.StatusOK},
		{"/assets/missing.js", "", http.StatusNotFound},
		{"/missing.png", "", http.StatusNotFound},
		{"/assets/", "no-cache, must-revalidate", http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		got := rec.Header().Get("Cache-Control")
		if rec.Code != tc.status {
			t.Errorf("%s 状态码 = %d, 期望 %d", tc.path, rec.Code, tc.status)
		}
		if got != tc.cache {
			t.Errorf("%s 的 Cache-Control = %q, 期望 %q", tc.path, got, tc.cache)
		}
	}
}
