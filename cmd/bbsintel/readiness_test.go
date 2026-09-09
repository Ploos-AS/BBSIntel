package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestReady(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	srv := &server{db: s.DB}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	srv.ready(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
