package modmgr

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var modIconFieldRe = regexp.MustCompile(`\b(icon_atlas|icon)\s*=\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')`)
var modIconTableRe = regexp.MustCompile(`\b(icon_atlas|icon)\s*=\s*([a-zA-Z_]\w*)\s*\[`)

type iconElement struct {
	Name string  `xml:"name,attr"`
	U1   float64 `xml:"u1,attr"`
	U2   float64 `xml:"u2,attr"`
	V1   float64 `xml:"v1,attr"`
	V2   float64 `xml:"v2,attr"`
}

type modIconSource struct {
	texture []byte
	region  iconElement
}

func (m *Manager) loadModIcon(id string) (*modIconSource, error) {
	if !filepath.IsLocal(id) || id != strings.TrimRight(id, ". ") || strings.ContainsAny(id, `/\:`) {
		return nil, fmt.Errorf("无效的模组 ID")
	}
	if num, workshop := strings.CutPrefix(id, "workshop-"); workshop && (num == "" || strings.Trim(num, "0123456789") != "") {
		return nil, fmt.Errorf("无效的创意工坊模组 ID")
	}
	dir := m.findModDir(id)
	if dir == "" {
		return nil, os.ErrNotExist
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := readIconFile(root, "modinfo.lua", 1<<20)
	if err != nil {
		return nil, err
	}
	text := stripLuaComments(stripBOM(string(info)))
	if a, b := braceBlockRange(text, "configuration_options"); a >= 0 && b > a {
		text = text[:a] + text[b+1:]
	}
	icon, atlasPath := "modicon.tex", "modicon.xml"
	for _, field := range modIconFieldRe.FindAllStringSubmatch(text, -1) {
		value := unescapeLuaString(field[2][1 : len(field[2])-1])
		if field[1] == "icon" {
			icon = value
		} else {
			atlasPath = value
		}
	}
	// shortcut: 语言表使用首个图标资源；需要严格匹配游戏语言时再解析语言条件。
	for _, field := range modIconTableRe.FindAllStringSubmatch(text, -1) {
		ext := ".tex"
		if field[1] == "icon_atlas" {
			ext = ".xml"
		}
		for _, value := range luaStringsIn(extractBraceBlock(text, field[2])) {
			if strings.ToLower(filepath.Ext(value)) != ext {
				continue
			}
			if field[1] == "icon" {
				icon = value
			} else {
				atlasPath = value
			}
			break
		}
	}
	atlasPath = filepath.FromSlash(strings.ReplaceAll(atlasPath, `\`, "/"))
	region := iconElement{U2: 1, V2: 1}
	texturePath := filepath.Join(filepath.Dir(atlasPath), icon)
	data, err := readIconFile(root, atlasPath, 1<<20)
	if err == nil {
		var atlas struct {
			Texture struct {
				Filename string `xml:"filename,attr"`
			} `xml:"Texture"`
			Elements []iconElement `xml:"Elements>Element"`
		}
		if err := xml.Unmarshal(data, &atlas); err != nil {
			return nil, err
		}
		found := false
		for _, element := range atlas.Elements {
			if element.Name == icon {
				region, found = element, true
				break
			}
		}
		if !found {
			if len(atlas.Elements) != 1 {
				return nil, fmt.Errorf("图集中没有模组图标")
			}
			region = atlas.Elements[0]
		}
		if atlas.Texture.Filename != "" {
			texturePath = filepath.Join(filepath.Dir(atlasPath), atlas.Texture.Filename)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	data, err = readIconFile(root, texturePath, 32<<20)
	if os.IsNotExist(err) && texturePath != icon {
		data, err = readIconFile(root, icon, 32<<20)
	}
	if err != nil {
		return nil, err
	}
	if !(region.U1 >= 0 && region.U1 < region.U2 && region.U2 <= 1 && region.V1 >= 0 && region.V1 < region.V2 && region.V2 <= 1) {
		return nil, fmt.Errorf("图标 UV 坐标无效")
	}
	return &modIconSource{texture: data, region: region}, nil
}

// ModIconVersion 随图片或裁切区域变化，让刷新列表和更新模组后能重新加载图标。
func (m *Manager) ModIconVersion(id string) string {
	source, err := m.loadModIcon(id)
	if err != nil {
		return ""
	}
	hash := sha256.New()
	hash.Write(source.texture)
	fmt.Fprintf(hash, "/%x/%x/%x/%x", math.Float64bits(source.region.U1), math.Float64bits(source.region.U2),
		math.Float64bits(source.region.V1), math.Float64bits(source.region.V2))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// ModIcon 只读取模组声明的本地图片，不执行 modinfo.lua。
func (m *Manager) ModIcon(id string) (image.Image, error) {
	source, err := m.loadModIcon(id)
	if err != nil {
		return nil, err
	}
	img, err := decodeTEX(source.texture)
	if err != nil {
		return nil, err
	}
	region := source.region
	w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	box := image.Rect(int(math.Floor(region.U1*w)), int(math.Floor((1-region.V2)*h)),
		int(math.Ceil(region.U2*w)), int(math.Ceil((1-region.V1)*h)))
	return img.SubImage(box), nil
}

func readIconFile(root *os.Root, name string, limit int64) ([]byte, error) {
	name = filepath.FromSlash(strings.ReplaceAll(name, `\`, "/"))
	if !filepath.IsLocal(name) || strings.Contains(name, ":") {
		return nil, fmt.Errorf("图标路径必须在模组目录内")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("图标文件无效或过大")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if len(data) > int(limit) {
		return nil, fmt.Errorf("图标文件过大")
	}
	return data, err
}
