package web

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"time"
)

func (s *Server) handleModIcon(w http.ResponseWriter, r *http.Request) {
	rm, err := s.roomByID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	img, err := s.modsFor(rm).ModIcon(query(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// 图标只显示 56px，限制响应尺寸以免把整张大图集传给浏览器。
	box := img.Bounds()
	if max(box.Dx(), box.Dy()) > 128 {
		scale := max(box.Dx(), box.Dy())
		thumb := image.NewNRGBA(image.Rect(0, 0, max(1, box.Dx()*128/scale), max(1, box.Dy()*128/scale)))
		for y := 0; y < thumb.Bounds().Dy(); y++ {
			for x := 0; x < thumb.Bounds().Dx(); x++ {
				thumb.Set(x, y, img.At(box.Min.X+(2*x+1)*box.Dx()/(2*thumb.Bounds().Dx()),
					box.Min.Y+(2*y+1)*box.Dy()/(2*thumb.Bounds().Dy())))
			}
		}
		img = thumb
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "无法生成模组图标", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(buf.Bytes())))
	http.ServeContent(w, r, "icon.png", time.Time{}, bytes.NewReader(buf.Bytes()))
}
