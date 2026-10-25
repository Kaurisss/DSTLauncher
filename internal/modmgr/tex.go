package modmgr

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// KTEX 的文件头与 mipmap 布局：https://github.com/oblivioncth/Stexatlaser#klei-tex-format
// shortcut: 支持 PC 的 DXT1/3/5、RGB(A)；需要移动端 ETC 图标时再扩展。
func decodeTEX(data []byte) (*image.NRGBA, error) {
	if len(data) < 18 || string(data[:4]) != "KTEX" {
		return nil, fmt.Errorf("无效的 KTEX 文件")
	}
	header := binary.LittleEndian.Uint32(data[4:8])
	format, mipmaps := (header>>4)&31, (header>>13)&31
	if header&0xffffc000 == 0xffffc000 { // 兼容旧版 TEX 文件头。
		format, mipmaps = (header>>3)&7, (header>>9)&15
	}
	offset := 8 + int(mipmaps)*10
	if mipmaps == 0 || offset > len(data) {
		return nil, fmt.Errorf("无效的 KTEX mipmap")
	}
	w, h := int(binary.LittleEndian.Uint16(data[8:10])), int(binary.LittleEndian.Uint16(data[10:12]))
	if w == 0 || h == 0 || int64(w)*int64(h) > 4<<20 {
		return nil, fmt.Errorf("KTEX 图片尺寸无效或过大")
	}
	stride, blockSize := 0, 16
	switch format {
	case 0:
		blockSize = 8
	case 1, 2:
	case 4, 5:
		channels := 4
		if format == 5 {
			channels = 3
		}
		stride = int(binary.LittleEndian.Uint16(data[12:14]))
		if stride == 0 {
			stride = w * channels
		}
		if stride < w*channels {
			return nil, fmt.Errorf("无效的 KTEX 行宽")
		}
	default:
		return nil, fmt.Errorf("不支持的 KTEX 像素格式：%d", format)
	}
	needed := int64((w+3)/4) * int64((h+3)/4) * int64(blockSize)
	if stride > 0 {
		needed = int64(stride) * int64(h)
	}
	size := uint64(binary.LittleEndian.Uint32(data[14:18]))
	if size < uint64(needed) || size > uint64(len(data)-offset) {
		return nil, fmt.Errorf("KTEX 像素数据不完整")
	}
	pixels := data[offset : offset+int(size)]
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	if stride > 0 {
		channels := 4
		if format == 5 {
			channels = 3
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*stride + x*channels
				p := [4]byte{pixels[i], pixels[i+1], pixels[i+2], 255}
				if channels == 4 {
					p[3] = pixels[i+3]
				}
				setTEXPixel(out, x, y, p)
			}
		}
		return out, nil
	}
	pos := 0
	for by := 0; by < h; by += 4 {
		for bx := 0; bx < w; bx += 4 {
			block := pixels[pos : pos+blockSize]
			pos += blockSize
			colorBlock := block
			if format != 0 {
				colorBlock = block[8:]
			}
			c0, c1 := binary.LittleEndian.Uint16(colorBlock), binary.LittleEndian.Uint16(colorBlock[2:])
			palette := [4][4]byte{texRGB565(c0), texRGB565(c1)}
			for channel := 0; channel < 3; channel++ {
				a, b := int(palette[0][channel]), int(palette[1][channel])
				if c0 > c1 || format != 0 {
					palette[2][channel] = byte((2*a + b) / 3)
					palette[3][channel] = byte((a + 2*b) / 3)
				} else {
					palette[2][channel] = byte((a + b) / 2)
				}
			}
			palette[2][3] = 255
			if c0 > c1 || format != 0 {
				palette[3][3] = 255
			}
			var alpha [8]byte
			var alphaBits uint64
			if format == 2 {
				alpha[0], alpha[1] = block[0], block[1]
				if alpha[0] > alpha[1] {
					for i := 2; i < 8; i++ {
						alpha[i] = byte(((8-i)*int(alpha[0]) + (i-1)*int(alpha[1])) / 7)
					}
				} else {
					for i := 2; i < 6; i++ {
						alpha[i] = byte(((6-i)*int(alpha[0]) + (i-1)*int(alpha[1])) / 5)
					}
					alpha[7] = 255
				}
				alphaBits = binary.LittleEndian.Uint64(block[:8]) >> 16
			}
			indices := binary.LittleEndian.Uint32(colorBlock[4:])
			for n := 0; n < 16; n++ {
				x, y := bx+n%4, by+n/4
				if x >= w || y >= h {
					continue
				}
				p := palette[(indices>>uint(2*n))&3]
				if format == 1 {
					p[3] = ((block[n/2] >> uint(4*(n%2))) & 15) * 17
				} else if format == 2 {
					p[3] = alpha[(alphaBits>>uint(3*n))&7]
				}
				setTEXPixel(out, x, y, p)
			}
		}
	}
	return out, nil
}

func texRGB565(v uint16) [4]byte {
	r, g, b := byte(v>>11), byte(v>>5)&63, byte(v)&31
	return [4]byte{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, 255}
}

func setTEXPixel(dst *image.NRGBA, x, y int, p [4]byte) {
	// Klei 的像素从底部开始，且颜色通常已乘 alpha；PNG 使用顶部原点和独立 alpha。
	if p[3] != 0 && p[3] != 255 {
		for c := 0; c < 3; c++ {
			p[c] = byte(min(255, int(p[c])*255/int(p[3])))
		}
	}
	if p[3] == 0 {
		p[0], p[1], p[2] = 0, 0, 0
	}
	dst.SetNRGBA(x, dst.Bounds().Dy()-1-y, color.NRGBA{R: p[0], G: p[1], B: p[2], A: p[3]})
}
