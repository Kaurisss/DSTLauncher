package worldsettings

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const mapFixture = `return {map={width=100,height=80,topology={nodes={{poly={{-40,-40},{40,-40},{40,40},{-40,40}},c=1}},ids={"Forest"},colours={{r=.2,g=.6,b=.3,a=1}}}},ents={pigking={{x=8,z=-12}},wormhole={{x=-8,z=12}}}}`

func compressedMap(text string) []byte {
	var packed bytes.Buffer
	writer := zlib.NewWriter(&packed)
	_, _ = writer.Write([]byte(text))
	_ = writer.Close()
	header := make([]byte, 16)
	binary.LittleEndian.PutUint32(header, 1)
	binary.LittleEndian.PutUint32(header[4:], 16)
	binary.LittleEndian.PutUint32(header[8:], uint32(len(text)))
	binary.LittleEndian.PutUint32(header[12:], uint32(packed.Len()))
	return []byte("KLEI     1D" + base64.StdEncoding.EncodeToString(append(header, packed.Bytes()...)))
}
func TestSavedMapKLEIAndWorldCoordinates(t *testing.T) {
	for _, input := range [][]byte{[]byte(mapFixture), []byte("KLEI     1 " + mapFixture), compressedMap(mapFixture)} {
		text, err := decodeMapSnapshot(input)
		if err != nil {
			t.Fatal(err)
		}
		m, err := parseSavedMap(text)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Regions) != 1 || m.Regions[0].Name != "Forest" || m.Regions[0].Color != "#33994d" || m.Regions[0].Points[0] != (MapPoint{40, 30}) {
			t.Fatal(m)
		}
		if len(m.Landmarks) != 2 || m.Landmarks[0].X != 52 || m.Landmarks[0].Y != 37 {
			t.Fatal(m.Landmarks)
		}
	}
}
func TestLatestWorldSnapshotDoesNotReadPlayersOrCompareSessionNumbers(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i, name := range []string{"old/0000009999", "new/0000000002", "new/player/9999999999"} {
		path := filepath.Join(dir, "save", "session", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, compressedMap(mapFixture), 0600); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(i) * time.Minute)
		_ = os.Chtimes(path, modified, modified)
	}
	m, err := ReadSavedMap(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Snapshot != "0000000002" || !m.SavedAt.Equal(base.Add(time.Minute)) {
		t.Fatal(m)
	}
}
func TestMapRejectsUnknownCompressionAndExecutableLua(t *testing.T) {
	for _, b := range [][]byte{[]byte("KLEI     1X..."), []byte("KLEI     1Dnot-base64"), compressedMap(mapFixture)[:25]} {
		if _, err := decodeMapSnapshot(b); err == nil {
			t.Fatal("接受损坏封装")
		}
	}
	for _, text := range []string{`os.execute("bad"); ` + mapFixture, mapFixture + `; os.execute("bad")`, strings.Replace(mapFixture, "width=100", "width=90000", 1), strings.Replace(mapFixture, "c=1", "c=99", 1), `return {map={width=100,height=80,tiles="unknown"}}`} {
		if _, err := parseSavedMap(text); err == nil {
			t.Fatal("接受非法地图", text)
		}
	}
}

func TestMapExplicitArrayIndicesAndParserBudget(t *testing.T) {
	text := `return {map={width=100,height=80,topology={nodes={[4]={poly={ [1]={[2]=-40,[1]=-40},[2]={40,-40},[3]={40,40}},c=7}},ids={[4]="Sparse"},colours={[7]={r=1,g=0,b=0}}}}}`
	m, err := parseSavedMap(text)
	if err != nil {
		t.Fatal(err)
	}
	if m.Regions[0].Name != "Sparse" || m.Regions[0].Color != "#ff0000" || m.Regions[0].Points[0] != (MapPoint{40, 30}) {
		t.Fatal(m)
	}
	if _, err := scanLevelRootLimit(`return {a={1,2,3}}`, 2); err == nil {
		t.Fatal("未限制字段数量")
	}
}

func TestSerializedSavedataFunctionsAreReadAsData(t *testing.T) {
	root, err := scanLevelRoot(mapFixture)
	if err != nil {
		t.Fatal(err)
	}
	value := mapField(root, "map")
	mapLiteral := mapFixture[value.start:value.end]
	text := `local savedata = {}
local tablefunctions = {}
tablefunctions["unrelated_fn"] = function()
return {note="os.execute(must-never-run)"}
end
savedata["meta"] = tablefunctions["unrelated_fn"]()
savedata["ents"] = {}
tablefunctions["ents_pigking_fn"] = function()
return {{x=8,z=-12}}
end
savedata["ents"]["pigking"] = tablefunctions["ents_pigking_fn"]()
tablefunctions["map_fn"] = function()
return ` + mapLiteral + `
end
savedata["map"] = tablefunctions["map_fn"]()
return savedata` + "\x00"
	m, err := parseSavedMap(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Regions) != 1 || len(m.Landmarks) != 1 || m.Landmarks[0].X != 52 {
		t.Fatal(m)
	}
	for _, bad := range []string{
		strings.Replace(text, "width=100", "width=os.execute(\"must-never-run\")", 1),
		strings.Replace(text, "return savedata", "os.execute(\"must-never-run\"); return savedata", 1),
	} {
		if _, err := parseSavedMap(bad); err == nil {
			t.Fatal("接受了动态地图数据或额外执行语句")
		}
	}
}

func TestMapAcceptsByteAndNormalizedColors(t *testing.T) {
	text := strings.Replace(mapFixture, "r=.2,g=.6,b=.3,a=1", "r=255,g=0,b=0,a=255", 1)
	m, err := parseSavedMap(text)
	if err != nil {
		t.Fatal(err)
	}
	if m.Regions[0].Color != "#ff0000" {
		t.Fatal(m.Regions[0].Color)
	}
}
