package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"dst-windows/internal/modmgr"
	"dst-windows/internal/room"
)

func TestModsStreamNDJSONOrderFilterAndParity(t *testing.T) {
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for id, info := range map[string]string{
		"101": `name="客户端滤除" client_only_mod=true`,
		"102": `name="服务端一号" version="1.0"`,
		"103": `name="服务端二号" version="2.0"`,
		"104": `name="未配置已安装" version="3.0"`,
	} {
		dir := filepath.Join(s.mgr.UgcDir(), "content", "322330", id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte(info), 0644); err != nil {
			t.Fatal(err)
		}
		if id == "102" {
			if err := os.WriteFile(filepath.Join(dir, "modicon.tex"), buildTestKTEXRGBA(1, 1, 255, 120, 40, 255), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte(`name="服务端一号" version="1.0" icon="modicon.tex"`), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	mods := s.modsFor(rm)
	o := modmgr.NewOverrides()
	o.Order = []string{"workshop-103", "workshop-101", "workshop-999", "workshop-102"}
	o.Enabled["workshop-103"] = true
	o.Enabled["workshop-999"] = true
	if err := mods.SaveOverrides(o); err != nil {
		t.Fatal(err)
	}

	h := s.Handler()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/mods?room=%d&stream=1", rm.ID), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/x-ndjson") {
		t.Fatalf("Content-Type 应为 NDJSON，实际 %q", ct)
	}

	var events []modStreamEvent
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev modStreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("NDJSON 解析失败 (%q): %v", line, err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 || events[0].Type != "meta" || events[len(events)-1].Type != "done" {
		t.Fatalf("流事件头尾不正确: %+v", events)
	}
	if events[0].Total != 5 {
		t.Fatalf("meta 候选总数应为 5，实际 %d", events[0].Total)
	}

	var streamedMods []modmgr.Mod
	for _, ev := range events[1 : len(events)-1] {
		if ev.Type != "mod" || ev.Mod == nil {
			t.Fatalf("中间事件应为 mod: %+v", ev)
		}
		streamedMods = append(streamedMods, *ev.Mod)
	}
	wantOrder := []string{"workshop-103", "workshop-999", "workshop-102", "workshop-104"}
	var streamedIDs []string
	for _, mod := range streamedMods {
		streamedIDs = append(streamedIDs, mod.ID)
	}
	if strings.Join(streamedIDs, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("流式顺序不符: got %v, want %v", streamedIDs, wantOrder)
	}
	if events[len(events)-1].Total != len(wantOrder) {
		t.Fatalf("done 计数不符: %d != %d", events[len(events)-1].Total, len(wantOrder))
	}

	// 与普通 JSON 接口返回完全一致
	_, body := doJSON(t, h, http.MethodGet, fmt.Sprintf("/api/mods?room=%d", rm.ID), nil, nil)
	rawItems := body["data"].([]interface{})
	var jsonIDs []string
	for _, item := range rawItems {
		jsonIDs = append(jsonIDs, item.(map[string]interface{})["id"].(string))
	}
	if strings.Join(jsonIDs, ",") != strings.Join(streamedIDs, ",") {
		t.Fatalf("JSON 与流式结果不一致: json=%v stream=%v", jsonIDs, streamedIDs)
	}
	var jsonMods []modmgr.Mod
	rawJSON, err := json.Marshal(body["data"])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawJSON, &jsonMods); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonMods, streamedMods) {
		t.Fatalf("JSON 与流式字段不一致: json=%+v stream=%+v", jsonMods, streamedMods)
	}

	// 鉴权保护
	s.envPassword = "secret"
	unauthRec := httptest.NewRecorder()
	h.ServeHTTP(unauthRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/mods?room=%d&stream=1", rm.ID), nil))
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("流式接口应受鉴权保护，实际 %d", unauthRec.Code)
	}
}

type failingStreamWriter struct {
	header   http.Header
	writes   int
	failFrom int
}

func (w *failingStreamWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingStreamWriter) WriteHeader(int) {}

func (w *failingStreamWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failFrom {
		return 0, fmt.Errorf("test stream write failure")
	}
	w.writes++
	return len(p), nil
}

func (w *failingStreamWriter) Flush() {}

func TestModsStreamWriteFailureStopsWorkers(t *testing.T) {
	s := newAuthTestServer(t, "")
	rm, err := s.rooms.Create(room.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 24)
	for i := 0; i < 24; i++ {
		id := fmt.Sprintf("%d", 200+i)
		ids = append(ids, "workshop-"+id)
		dir := filepath.Join(s.mgr.UgcDir(), "content", "322330", id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "modinfo.lua"), []byte(`name="stream fixture"`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mods := s.modsFor(rm)
	o := modmgr.NewOverrides()
	o.Order = ids
	if err := mods.SaveOverrides(o); err != nil {
		t.Fatal(err)
	}

	before := runtime.NumGoroutine()
	writer := &failingStreamWriter{failFrom: 1}
	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/mods?room=%d&stream=1", rm.ID), nil)
		s.handleMods(writer, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("写入失败后流处理未返回")
	}

	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before+2 {
		t.Fatalf("写入失败后仍有 worker 泄漏: before=%d after=%d", before, got)
	}
}
