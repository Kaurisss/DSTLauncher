package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignedDeliveryFiltersAndPersistence(t *testing.T) {
	received := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte("secret"))
		mac.Write([]byte(r.Header.Get("X-DST-Timestamp") + "."))
		mac.Write(body)
		if r.Method != http.MethodPost || r.Header.Get("X-DST-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("Webhook 请求/签名错误")
		}
		received <- string(body)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "notifications.json")
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	secret := "secret"
	c := Config{Enabled: true, URL: srv.URL, Events: []string{"crash_limit"}, Rooms: []int{2}}
	if err := e.Configure(c, &secret); err != nil {
		t.Fatal(err)
	}
	loaded, err := New(path)
	if err != nil || !loaded.Snapshot().SecretSet || loaded.Snapshot().Secret != "" {
		t.Fatal(err, loaded)
	}
	if err := loaded.Configure(c, nil); err != nil {
		t.Fatal(err)
	}
	if loaded.config.Secret != secret {
		t.Fatal("未填写密钥时没有保留")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); loaded.Serve(ctx) }()
	defer func() { cancel(); <-done }()
	loaded.Publish(Event{Event: "started", RoomID: 2})
	loaded.Publish(Event{Event: "crash_limit", RoomID: 1})
	if len(loaded.queue) != 0 {
		t.Fatal("未过滤未订阅的事件和房间")
	}
	loaded.Publish(Event{Event: "crash_limit", RoomID: 2, Message: "达到重启上限", Version: "test"})
	select {
	case body := <-received:
		if !strings.Contains(body, `"roomId":2`) || !strings.Contains(body, `"id":`) {
			t.Fatal(body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("未收到通知")
	}
}

func TestFailureFeedbackAndNoRedirect(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer srv.Close()
	e, _ := New(filepath.Join(t.TempDir(), "notifications.json"))
	if err := e.Configure(Config{URL: srv.URL, Events: []string{}}, nil); err != nil {
		t.Fatal(err)
	}
	result := e.Test(context.Background(), "test")
	if result.Error == "" || result.Status != 302 || redirected || len(e.Snapshot().Results) != 1 {
		t.Fatal(result, redirected)
	}
	for _, address := range []string{"file:///tmp/a", "https://user:password@example.com/a", "http://"} {
		if err := e.Configure(Config{Enabled: true, URL: address, Events: []string{"crash_limit"}}, nil); err == nil {
			t.Fatal("接受了非法地址", address)
		}
	}
}
