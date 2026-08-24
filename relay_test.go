package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSplitTargetsTrimsAndDropsEmpty(t *testing.T) {
	got := splitTargets(" http://a , ,http://b ")
	if len(got) != 2 || got[0] != "http://a" || got[1] != "http://b" {
		t.Fatalf("splitTargets returned %#v", got)
	}
}

func TestRelayFansOutToEveryTarget(t *testing.T) {
	var a, b int32
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&a, 1)
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&b, 1)
	}))
	defer srvB.Close()

	relay := NewRelay([]string{srvA.URL, srvB.URL}, 1, srvA.Client())
	req := httptest.NewRequest(http.MethodPost, "/hook", strings.NewReader(`{"ok":true}`))
	rec := httptest.NewRecorder()
	relay.Handle(rec, req)
	relay.Shutdown()

	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", rec.Code)
	}
	if atomic.LoadInt32(&a) != 1 || atomic.LoadInt32(&b) != 1 {
		t.Fatalf("fan-out missed a target: a=%d b=%d", a, b)
	}
}

func TestRelayRejectsNonPost(t *testing.T) {
	relay := NewRelay([]string{"http://example.invalid"}, 1, &http.Client{Timeout: time.Second})
	defer relay.Shutdown()
	rec := httptest.NewRecorder()
	relay.Handle(rec, httptest.NewRequest(http.MethodGet, "/hook", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}
