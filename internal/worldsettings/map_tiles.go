package worldsettings

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
)

// DST's versioned tile stream has a NUL-terminated VRSN tag followed by a
// little-endian uint32 version and exactly width*height little-endian uint16 IDs.
// The 9-byte header and ID coverage are checked against real forest/cave/IA saves.
func decodeMapTiles(encoded string, width, height int, mapping *levelTable) ([]uint16, map[uint16]string, error) {
	if width < 1 || height < 1 || int64(width)*int64(height) > 4*1024*1024 {
		return nil, nil, fmt.Errorf("地块数量超过地图预览上限")
	}
	expected := 9 + width*height*2
	if len(encoded) > base64.StdEncoding.EncodedLen(expected) {
		return nil, nil, fmt.Errorf("地块数据长度与地图尺寸不符")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("地块 Base64 损坏: %w", err)
	}
	if len(raw) != expected {
		return nil, nil, fmt.Errorf("地块数据长度与地图尺寸不符")
	}
	if string(raw[:5]) != "VRSN\x00" || binary.LittleEndian.Uint32(raw[5:9]) != 1 {
		return nil, nil, fmt.Errorf("不支持的地块编码版本")
	}
	if mapping == nil {
		return nil, nil, fmt.Errorf("保存点缺少地形名称表")
	}
	names := map[uint16]string{}
	for _, f := range mapping.fields {
		n, err := mapNumber(f.value)
		if err != nil || n < 0 || n > 65535 || math.Trunc(n) != n || f.name == "" {
			return nil, nil, fmt.Errorf("地形名称表包含非法条目")
		}
		id := uint16(n)
		if _, exists := names[id]; exists {
			return nil, nil, fmt.Errorf("地形名称表包含重复 ID %d", id)
		}
		names[id] = f.name
	}
	tiles := make([]uint16, width*height)
	for i := range tiles {
		id := binary.LittleEndian.Uint16(raw[9+i*2:])
		if _, exists := names[id]; !exists {
			return nil, nil, fmt.Errorf("地块 ID %d 未在保存点的地形表中定义", id)
		}
		tiles[i] = id
	}
	return tiles, names, nil
}
