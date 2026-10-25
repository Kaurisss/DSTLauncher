package modmgr

import (
	"encoding/binary"
	"image/color"
	"testing"
)

func texFixture(format uint32, w, h uint16, pixels []byte) []byte {
	data := make([]byte, 18)
	copy(data, "KTEX")
	binary.LittleEndian.PutUint32(data[4:], 0xfffc2200|format<<4)
	binary.LittleEndian.PutUint16(data[8:], w)
	binary.LittleEndian.PutUint16(data[10:], h)
	binary.LittleEndian.PutUint32(data[14:], uint32(len(pixels)))
	return append(data, pixels...)
}

func TestDecodeTEXColorAndOrientation(t *testing.T) {
	// 存储顺序为底行红/绿、顶行蓝/半透明红。
	rgba := texFixture(4, 2, 2, []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 128, 0, 0, 128})
	img, err := decodeTEX(rgba)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		x, y int
		want color.NRGBA
	}{
		{0, 0, color.NRGBA{B: 255, A: 255}},
		{1, 0, color.NRGBA{R: 255, A: 128}},
		{0, 1, color.NRGBA{R: 255, A: 255}},
		{1, 1, color.NRGBA{G: 255, A: 255}},
	} {
		if got := img.NRGBAAt(tc.x, tc.y); got != tc.want {
			t.Fatalf("(%d,%d)=%v，期望 %v", tc.x, tc.y, got, tc.want)
		}
	}
	// RGB 每行六个像素字节，另有两个 padding 字节。
	rgb := texFixture(5, 2, 2, []byte{255, 0, 0, 0, 255, 0, 99, 99, 0, 0, 255, 255, 255, 255, 99, 99})
	binary.LittleEndian.PutUint16(rgb[12:], 8)
	img, err = decodeTEX(rgb)
	if err != nil || img.NRGBAAt(0, 0) != (color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("RGB 行宽与方向：%v, %v", img, err)
	}
	// mipmap 描述全部位于数据之前，第一层才是原图。
	mips := append(append(append([]byte{}, rgba[:18]...), make([]byte, 10)...), rgba[18:]...)
	binary.LittleEndian.PutUint32(mips[4:], 0xfffc4240)
	img, err = decodeTEX(mips)
	if err != nil || img.NRGBAAt(0, 0) != (color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("多级 mipmap 偏移：%v, %v", img, err)
	}
}

func TestDecodeTEXBlockCompression(t *testing.T) {
	// RGB565 红/蓝端点，四行分别使用颜色索引 0、2、3、1。
	block := []byte{0, 0xf8, 0x1f, 0, 0, 0xaa, 0xff, 0x55}
	img, err := decodeTEX(texFixture(0, 4, 4, block))
	if err != nil {
		t.Fatal(err)
	}
	for y, want := range []color.NRGBA{{B: 255, A: 255}, {R: 85, B: 170, A: 255}, {R: 170, B: 85, A: 255}, {R: 255, A: 255}} {
		if got := img.NRGBAAt(0, y); got != want {
			t.Fatalf("DXT1 第 %d 行=%v，期望 %v", y, got, want)
		}
	}
	transparent := []byte{0, 0, 255, 255, 255, 255, 255, 255}
	img, err = decodeTEX(texFixture(0, 4, 4, transparent))
	if err != nil || img.NRGBAAt(0, 0).A != 0 {
		t.Fatalf("DXT1 透明像素：%v", err)
	}
	red := []byte{0, 0xf8, 0, 0, 0, 0, 0, 0}
	dxt3 := append([]byte{0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0}, red...)
	img, err = decodeTEX(texFixture(1, 4, 4, dxt3))
	if err != nil || img.NRGBAAt(0, 0).A != 0 || img.NRGBAAt(1, 0) != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("DXT3 独立 alpha：%v", err)
	}
	for _, tc := range []struct {
		a0, a1 byte
		want   [8]byte
	}{
		{255, 0, [8]byte{255, 0, 218, 182, 145, 109, 72, 36}},
		{0, 255, [8]byte{0, 255, 51, 102, 153, 204, 0, 255}},
	} {
		data := make([]byte, 16)
		var bits uint64
		for i := 0; i < 16; i++ {
			bits |= uint64(i%8) << (3 * i)
		}
		binary.LittleEndian.PutUint64(data, bits<<16|uint64(tc.a1)<<8|uint64(tc.a0))
		img, err = decodeTEX(texFixture(2, 4, 4, data))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 16; i++ {
			if got := img.NRGBAAt(i%4, 3-i/4).A; got != tc.want[i%8] {
				t.Fatalf("DXT5 alpha 索引 %d=%d，期望 %d", i%8, got, tc.want[i%8])
			}
		}
	}
	img, err = decodeTEX(texFixture(0, 2, 3, red))
	if err != nil || img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 || img.NRGBAAt(1, 2).R != 255 {
		t.Fatalf("非四倍数边界：%v", err)
	}
}

func TestDecodeTEXRejectsMalformedData(t *testing.T) {
	valid := texFixture(4, 2, 2, make([]byte, 16))
	for i := 0; i < len(valid); i++ {
		if _, err := decodeTEX(valid[:i]); err == nil {
			t.Fatalf("不应接受截断数据：%d 字节", i)
		}
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { copy(b, "NOPE") },
		func(b []byte) { binary.LittleEndian.PutUint16(b[8:], 0) },
		func(b []byte) {
			binary.LittleEndian.PutUint16(b[8:], 65535)
			binary.LittleEndian.PutUint16(b[10:], 65535)
		},
		func(b []byte) { binary.LittleEndian.PutUint32(b[14:], 0xffffffff) },
		func(b []byte) { binary.LittleEndian.PutUint16(b[12:], 1) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 0xfffc2270) },
	} {
		bad := append([]byte{}, valid...)
		mutate(bad)
		if _, err := decodeTEX(bad); err == nil {
			t.Fatal("不应接受无效纹理")
		}
	}
}

func FuzzDecodeTEX(f *testing.F) {
	f.Add([]byte("KTEX"))
	f.Add(texFixture(4, 1, 1, []byte{255, 0, 0, 255}))
	f.Add(texFixture(2, 4, 4, make([]byte, 16)))
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = decodeTEX(data) })
}
