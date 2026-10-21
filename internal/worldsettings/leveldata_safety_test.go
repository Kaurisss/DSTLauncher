package worldsettings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelDataBytePatches(t *testing.T) {
	for _, indent := range []string{"\t", "    ", ""} {
		t.Run("indent="+indent, func(t *testing.T) {
			src := "return { location='forest', overrides={\n" + indent + "bamboo = 'default', -- keep }\n" + indent + "nested={bamboo='nested', text=[=[ } ]=]},\n" + indent + "other=\"quoted \\\" }\",\n} }\n"
			out, err := ParseLevelData(src).WithOverrides(map[string]string{"bamboo": "often"})
			want := strings.Replace(src, "bamboo = 'default'", "bamboo = 'often'", 1)
			if err != nil || out != want {
				t.Fatalf("err=%v\ngot %q\nwant %q", err, out, want)
			}
		})
	}
	for _, src := range []string{
		`return {location="forest", overrides={bamboo="default", nested={bamboo="nested"}}}`,
		"return {\r\n    location=\"forest\",\r\n    overrides={bamboo=\"default\" -- }\r\n    },\r\n}\r\n",
		`-- overrides={fake="x"}
return {location="forest", note="overrides={}", overrides={--[==[ } { ]==]
bamboo="default", nested={text='\\', bamboo='nested'}}}`,
	} {
		out, err := ParseLevelData(src).WithOverrides(map[string]string{"bamboo": "often"})
		if want := strings.Replace(src, `bamboo="default"`, `bamboo="often"`, 1); err != nil || out != want {
			t.Errorf("err=%v\ngot %q\nwant %q", err, out, want)
		}
	}
}

func TestLevelDataAppendAndNoop(t *testing.T) {
	for _, src := range []string{
		`return {location='forest', overrides={bamboo='default'}}`,
		"return {location='forest', overrides={\r\n\tbamboo='default' -- keep }\r\n}}",
		`return {location='forest', overrides={}}`,
		"return {location='forest', overrides={ -- }\n}}",
	} {
		ld := ParseLevelData(src)
		if out, err := ld.WithOverrides(ld.Overrides); err != nil || out != src {
			t.Fatalf("noop: %v %q", err, out)
		}
		out, err := ld.WithOverrides(map[string]string{"newkey": "often"})
		if err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		p := LevelDataPath(root, "Master")
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(out), 0644)
		again, ok, err := ReadLevelData(root, "Master")
		if err != nil || !ok || again.Overrides["newkey"] != "often" {
			t.Fatalf("append invalid: %v %q", err, out)
		}
		if strings.Contains(src, "default' --") && !strings.Contains(out, "default', -- keep }") {
			t.Errorf("missing separator before comment: %q", out)
		}
	}
}

func TestLevelDataTypesAndStrings(t *testing.T) {
	src := `return {location='forest', desc="first\
second", overrides={flag=true, count=4, stringbool='true', stringnum="4", escaped='it\'s }', long=[=[
hello }]=]}}`
	ld := ParseLevelData(src)
	if ld.Location() != "forest" || ld.Scalar["desc"] != "first\nsecond" || ld.Overrides["escaped"] != "it's }" || ld.Overrides["long"] != "hello }" {
		t.Fatalf("decoded: %#v %#v", ld.Scalar, ld.Overrides)
	}
	if out, err := ld.WithOverrides(ld.Overrides); err != nil || out != src {
		t.Fatalf("noop: %v %q", err, out)
	}
	out, err := ld.WithOverrides(map[string]string{"flag": "false", "count": "5", "stringbool": "false", "stringnum": "5"})
	want := strings.NewReplacer("flag=true", "flag=false", "count=4", "count=5", "stringbool='true'", "stringbool='false'", `stringnum="4"`, `stringnum="5"`).Replace(src)
	if err != nil || out != want {
		t.Fatalf("types: %v %q", err, out)
	}
	for _, update := range []map[string]string{{"flag": "often"}, {"count": "default"}, {"return": "x"}, {"bamboo": "x\nend"}} {
		if _, err := ld.WithOverrides(update); err == nil {
			t.Errorf("accepted: %v", update)
		}
	}
}

func TestLevelDataRejectsUnsafeInput(t *testing.T) {
	for _, src := range []string{
		`return {location="forest", overrides={bamboo="a"}`,
		`return {location="forest", overrides={bamboo="a" junk}}`,
		"return {location=\"forest\", overrides={bamboo=\"a\nb\"}}",
		`return {location="forest", overrides={bamboo='a}}`,
		`return {location="forest", overrides={ --[[ } }`,
		`return {location="forest", overrides={bamboo=[=[ x }}}`,
		`return {location="forest", overrides={bamboo="a", bamboo="b"}}`,
		`return {location="forest", overrides={bamboo="a"}} os.exit()`,
		`return {location="forest", overrides={bamboo="a" .. "b"}}`,
		`return {location="forest", overrides={bamboo=choose("a", "b")}}`,
	} {
		t.Run(src, func(t *testing.T) {
			root := t.TempDir()
			p := LevelDataPath(root, "Master")
			os.MkdirAll(filepath.Dir(p), 0755)
			os.WriteFile(p, []byte(src), 0644)
			if _, ok, err := ReadLevelData(root, "Master"); err == nil || ok || !strings.Contains(err.Error(), LevelDataFileName) {
				t.Errorf("read accepted: ok=%v err=%v", ok, err)
			}
			if out, err := ParseLevelData(src).WithOverrides(map[string]string{"bamboo": "often"}); err == nil || out != "" {
				t.Errorf("update accepted: %v %q", err, out)
			}
			b, _ := os.ReadFile(p)
			if string(b) != src {
				t.Fatal("disk changed")
			}
			if _, err := os.Stat(p + ".bak"); !os.IsNotExist(err) {
				t.Fatal("backup changed on rejection")
			}
		})
	}
}

func TestLevelDataTruncatedInputs(t *testing.T) {
	// 每个截断点都必须安全返回，不能越界或写出半张表。
	for i := 0; i < len(realLevelData); i++ {
		ld := ParseLevelData(realLevelData[:i])
		_, _ = ld.WithOverrides(map[string]string{"bamboo": "often"})
	}
}

func TestLevelDataNumberBeforeComment(t *testing.T) {
	src := "return {location='forest', overrides={count=4-- }\r\n}}"
	out, err := ParseLevelData(src).WithOverrides(map[string]string{"count": "5", "newkey": "often"})
	want := "return {location='forest', overrides={count=5,-- }\r\n newkey=\"often\",}}"
	if err != nil || out != want {
		t.Fatalf("err=%v got=%q want=%q", err, out, want)
	}
}

func TestLevelDataWriteBackupAndRejection(t *testing.T) {
	root := t.TempDir()
	p := LevelDataPath(root, "Master")
	os.MkdirAll(filepath.Dir(p), 0755)
	src := `return {location="forest", overrides={bamboo="default"}}`
	os.WriteFile(p, []byte(src), 0644)
	ld, _, err := ReadLevelData(root, "Master")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ld.WithOverrides(map[string]string{"bamboo": "often"})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteLevelData(root, "Master", out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p + ".bak")
	if string(b) != src {
		t.Fatal("wrong backup")
	}
	b, _ = os.ReadFile(p)
	if string(b) != out {
		t.Fatal("wrong output")
	}
	if err := WriteLevelData(root, "Master", `return {broken`); err == nil {
		t.Fatal("accepted malformed write")
	}
	b, _ = os.ReadFile(p)
	if string(b) != out {
		t.Fatal("rejected write changed file")
	}
	b, _ = os.ReadFile(p + ".bak")
	if string(b) != src {
		t.Fatal("rejected write changed backup")
	}
}
