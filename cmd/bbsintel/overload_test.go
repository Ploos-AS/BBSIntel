package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestInflightLimiterRejectsWhenSaturated(t *testing.T) {
	limiter := newInflightLimiter(1)
	entered := make(chan struct{})
	release := make(chan struct{})
	h := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/bbs", nil))
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter handler")
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/bbs", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
	}
	if rr.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After=%q want=1", rr.Header().Get("Retry-After"))
	}

	close(release)
	wg.Wait()
}

func TestInflightLimiterExemptsHealthChecks(t *testing.T) {
	limiter := newInflightLimiter(1)
	limiter.sem <- struct{}{}
	defer func() { <-limiter.sem }()

	h := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/healthz", "/readyz"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("%s status=%d want=%d", path, rr.Code, http.StatusNoContent)
		}
	}
}
