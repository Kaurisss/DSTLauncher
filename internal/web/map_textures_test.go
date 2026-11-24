package web

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dst-windows/internal/steamcmd"
	"dst-windows/internal/worldsettings"
)

func buildTestKTEXRGBA(w, h int, r, g, b, a byte) []byte {
	return buildTestKTEX(w, h, func(int, int) [4]byte {
		return [4]byte{r, g, b, a}
	})
}

func buildTestKTEX(w, h int, pixel func(x, y int) [4]byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("KTEX")
	// format=4 (RGBA), mipmaps=1
	header := uint32(4<<4) | uint32(1<<13)
	_ = binary.Write(&buf, binary.LittleEndian, header)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(w))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(h))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(w*4))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(w*h*4))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := pixel(x, y)
			buf.Write(p[:])
		}
	}
	return buf.Bytes()
}

func TestMapTextureEndpointDecodesCachesAndAuthenticates(t *testing.T) {
	s, _ := newWorldConfigServer(t, []string{"Master"}, nil)
	s.steam = steamcmd.NewManager(t.TempDir(), steamcmd.ArchAuto)
	texDir := filepath.Join(s.steam.GameDir(), "data", "levels", "textures")
	if err := os.MkdirAll(texDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 600x600 covers a non-integral scale factor and must stay within 256x256.
	grassPath := filepath.Join(texDir, "mini_grass_noise.tex")
	if err := os.WriteFile(grassPath, buildTestKTEXRGBA(600, 600, 120, 180, 90, 255), 0600); err != nil {
		t.Fatal(err)
	}

	// availableMapTextures lists only existing tiles.
	avail := s.availableMapTextures(&worldsettings.SavedMap{
		Terrain: &worldsettings.MapTerrain{
			Specs: []worldsettings.MapTileSpec{
				{ID: 6, Name: "GRASS"},
				{ID: 7, Name: "FOREST"},
			},
		},
	})
	if len(avail) != 1 || avail[0] != "GRASS" {
		t.Fatalf("expected only GRASS available, got %v", avail)
	}

	// Valid request returns decoded & downsampled PNG.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/texture?tile=GRASS", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected response: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("invalid png: %v", err)
	}
	if img.Bounds().Dx() > 256 || img.Bounds().Dy() > 256 {
		t.Fatalf("expected downsampled image within 256x256, got %v", img.Bounds())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=86400" {
		t.Fatalf("authenticated image must be private-cacheable, got %q", got)
	}

	if err := os.Remove(grassPath); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/texture?tile=GRASS", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cached texture should survive source removal, got %d", rec.Code)
	}

	// Path traversal or unknown tile returns 404.
	for _, bad := range []string{"../secret", "FOREST", ""} {
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/texture?tile="+bad, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %q, got %d", bad, rec.Code)
		}
	}

	// Auth protection.
	s.envPassword = "secret"
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/texture?tile=GRASS", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}

func TestMapIconEndpointSlicesAtlasCachesAndAuthenticates(t *testing.T) {
	s, rm := newWorldConfigServer(t, []string{"Master"}, nil)
	s.steam = steamcmd.NewManager(t.TempDir(), steamcmd.ArchAuto)
	miniDir := filepath.Join(s.steam.GameDir(), "data", "minimap")
	if err := os.MkdirAll(miniDir, 0755); err != nil {
		t.Fatal(err)
	}
	atlasPath := filepath.Join(miniDir, "minimap_atlas2.tex")
	atlas := buildTestKTEX(128, 128, func(x, y int) [4]byte {
		if y >= 64 {
			if x < 64 {
				return [4]byte{220, 40, 40, 255}
			}
			return [4]byte{40, 220, 40, 255}
		}
		if x < 64 {
			return [4]byte{40, 40, 220, 255}
		}
		return [4]byte{220, 220, 40, 255}
	})
	if err := os.WriteFile(atlasPath, atlas, 0600); err != nil {
		t.Fatal(err)
	}
	xmlData := `<Atlas><Texture filename="minimap_atlas2.tex" /><Elements><Element name="pigking.png" u1="0" u2="0.5" v1="0.5" v2="1" /><Element name="wormhole.png" u1="0.5" u2="1" v1="0.5" v2="1" /></Elements></Atlas>`
	if err := os.WriteFile(filepath.Join(miniDir, "minimap_data2.xml"), []byte(xmlData), 0600); err != nil {
		t.Fatal(err)
	}
	texDir := filepath.Join(s.steam.GameDir(), "data", "levels", "textures")
	if err := os.MkdirAll(texDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(texDir, "mini_grass_noise.tex"), buildTestKTEXRGBA(2, 2, 120, 180, 90, 255), 0600); err != nil {
		t.Fatal(err)
	}

	avail := s.availableMapIcons(&worldsettings.SavedMap{
		Landmarks: []worldsettings.MapLandmark{
			{ID: "pigking:0", Kind: "pigking", Name: "猪王"},
			{ID: "wormhole:0", Kind: "wormhole", Name: "虫洞"},
		},
	})
	if len(avail) != 2 || avail[0] != "pigking" || avail[1] != "wormhole" {
		t.Fatalf("expected pigking and wormhole icons available, got %v", avail)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/icon?kind=pigking", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected icon response: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("invalid icon png: %v", err)
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Fatalf("expected cropped 64x64 icon, got %v", img.Bounds())
	}
	if got := color.NRGBAModel.Convert(img.At(8, 8)).(color.NRGBA); got.R < 200 || got.G > 80 || got.B > 80 {
		t.Fatalf("expected top-left atlas quadrant after V flip, got %#v", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=86400" {
		t.Fatalf("authenticated icon must be private-cacheable, got %q", got)
	}

	mapFile := filepath.Join(rm.ClusterDir(s.rootDir, s.confDir), "Master", "save", "session", "WORLD", "0000000001")
	if err := os.MkdirAll(filepath.Dir(mapFile), 0755); err != nil {
		t.Fatal(err)
	}
	tiles := base64.StdEncoding.EncodeToString([]byte{'V', 'R', 'S', 'N', 0, 1, 0, 0, 0, 7, 0, 7, 0, 7, 0, 7, 0})
	mapText := `return {map={width=2,height=2,tiles="` + tiles + `",world_tile_map={GRASS=7}},ents={pigking={{x=0,z=0}}}}`
	if err := os.WriteFile(mapFile, []byte(mapText), 0600); err != nil {
		t.Fatal(err)
	}
	_, body := doJSON(t, s.Handler(), http.MethodGet, "/api/map?room=1&shard=Master", nil, nil)
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("map endpoint returned unexpected data: %#v", body["data"])
	}
	if !containsString(data["textures"], "GRASS") || !containsString(data["icons"], "pigking") {
		t.Fatalf("map endpoint must advertise usable textures and icons: %#v", data)
	}

	if err := os.Remove(atlasPath); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/icon?kind=wormhole", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("pre-cached icon should survive source removal, got %d", rec.Code)
	}

	for _, bad := range []string{"../secret", "unknown", ""} {
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/icon?kind="+bad, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %q, got %d", bad, rec.Code)
		}
	}

	s.envPassword = "secret"
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/map/icon?kind=pigking", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}

func containsString(value interface{}, want string) bool {
	items, ok := value.([]interface{})
	if !ok {
		return false
	}
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
