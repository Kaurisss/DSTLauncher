package worldsettings

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

func tileStream(ids ...uint16) string {
	b := make([]byte, 9+2*len(ids))
	copy(b, "VRSN\x00")
	binary.LittleEndian.PutUint32(b[5:], 1)
	for i, id := range ids {
		binary.LittleEndian.PutUint16(b[9+2*i:], id)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestTerrainMapDoesNotNeedTopologyAndPairsActualWormholeReferences(t *testing.T) {
	text := `return {map={width=2,height=2,tiles="TILES",world_tile_map={GRASS=7,OCEAN_COASTAL=201,MOD_GROUND=10504}},ents={wormhole={{id=101,x=0,z=0,data={teleporter={target=202}}},{id=202,x=4,z=0,data={teleporter={target=101}}},{id=303,x=0,z=4}}}}`
	text = strings.Replace(text, "TILES", tileStream(7, 201, 10504, 7), 1)
	m, err := parseSavedMap(text)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != "terrain" || m.Terrain == nil || len(m.Regions) != 0 {
		t.Fatal("依赖了拓扑轮廓")
	}
	raw, _ := base64.StdEncoding.DecodeString(m.Terrain.Tiles)
	if len(raw) != 8 || binary.LittleEndian.Uint16(raw[4:]) != 10504 {
		t.Fatal("高 ID 地块损坏")
	}
	if m.Landmarks[0].Target != m.Landmarks[1].ID || m.Landmarks[1].Target != m.Landmarks[0].ID || m.Landmarks[0].Name != "虫洞 1" || m.Landmarks[2].Target != "" {
		t.Fatal("没有按实际引用匹配虫洞", m.Landmarks)
	}
}
func TestDecodeTilesUsesFullHeaderAndSavedNameMapping(t *testing.T) {
	root, err := scanLevelRoot(`return {GRASS=7,OCEAN_COASTAL=201,MOD_GROUND=10504}`)
	if err != nil {
		t.Fatal(err)
	}
	tiles, names, err := decodeMapTiles(tileStream(7, 201, 10504, 7), 2, 2, root)
	if err != nil {
		t.Fatal(err)
	}
	if tiles[0] != 7 || tiles[1] != 201 || tiles[2] != 10504 || names[10504] != "MOD_GROUND" {
		t.Fatal(tiles, names)
	}
}
func TestDecodeTilesRejectsUnknownVersionsLengthsAndIDs(t *testing.T) {
	root, _ := scanLevelRoot(`return {GRASS=7}`)
	valid := tileStream(7)
	version, _ := base64.StdEncoding.DecodeString(valid)
	version[5] = 2
	badHeader, _ := base64.StdEncoding.DecodeString(valid)
	badHeader[4] = 1
	for _, encoded := range []string{"not-base64", tileStream(7, 7), tileStream(8), base64.StdEncoding.EncodeToString(version), base64.StdEncoding.EncodeToString(badHeader), base64.StdEncoding.EncodeToString(append(version, 0))} {
		if _, _, err := decodeMapTiles(encoded, 1, 1, root); err == nil {
			t.Fatal("接受损坏的地块编码")
		}
	}
}

func TestShipwreckedAndVolcanoLandmarksAndBermudaPairing(t *testing.T) {
	text := `return {map={width=2,height=2,tiles="TILES",world_tile_map={BEACH=90}},ents={volcano={{x=0,z=0}},octopusking={{x=4,z=0}},volcano_altar={{x=0,z=4}},obsidian_workbench={{x=4,z=4}},bermudatriangle={{id=501,x=0,z=0,data={teleporter={target=502}}},{id=502,x=4,z=4,data={teleporter={target=501}}}}}}`
	text = strings.Replace(text, "TILES", tileStream(90, 90, 90, 90), 1)
	m, err := parseSavedMap(text)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string][]MapLandmark{}
	for _, lm := range m.Landmarks {
		byKind[lm.Kind] = append(byKind[lm.Kind], lm)
	}
	for _, want := range []string{"volcano", "octopusking", "volcano_altar", "obsidian_workbench", "bermudatriangle"} {
		if len(byKind[want]) == 0 {
			t.Fatalf("missing landmark %q in %+v", want, m.Landmarks)
		}
	}
	pair := byKind["bermudatriangle"]
	if len(pair) != 2 || pair[0].Target != pair[1].ID || pair[1].Target != pair[0].ID || pair[0].Name != "百慕大三角 1" {
		t.Fatalf("bermudatriangle did not pair properly: %+v", pair)
	}
}
