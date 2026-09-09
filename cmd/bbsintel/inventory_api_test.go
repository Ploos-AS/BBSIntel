package main

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestBBSInventoryPaginationAndQueryValidation(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.DB.Exec(`INSERT INTO bbs(id,name,software,country) VALUES
(1,'Alpha','Synchronet','US'),
(2,'Beta','Mystic','NO'),
(3,'Gamma','Synchronet','NO')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO endpoint(id,bbs_id,protocol,hostname,port) VALUES
(1,1,'telnet','alpha.test',23),(2,2,'ssh','beta.test',22),(3,3,'telnet','gamma.test',23)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO source_entry(bbs_id,source,source_key,active) VALUES
(1,'telnetbbsguide','a',1),(2,'synchronet','b',1),(3,'telnetbbsguide','g',1)`); err != nil {
		t.Fatal(err)
	}

	srv := &server{db: s.DB}

	req := httptest.NewRequest("GET", "/api/v1/bbs?limit=2&country=NO", nil)
	rr := httptest.NewRecorder()
	srv.listBBS(rr, req)
	if rr.Code != 200 {
		t.Fatalf("first page status=%d body=%s", rr.Code, rr.Body.String())
	}
	var first []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0]["name"] != "Beta" || first[1]["name"] != "Gamma" {
		t.Fatalf("unexpected first page: %#v", first)
	}
	if got := rr.Header().Get("X-Next-Cursor"); got != "" {
		t.Fatalf("unexpected cursor when filtered result fits one page: %q", got)
	}

	req = httptest.NewRequest("GET", "/api/v1/bbs?limit=2", nil)
	rr = httptest.NewRecorder()
	srv.listBBS(rr, req)
	cursor := rr.Header().Get("X-Next-Cursor")
	if rr.Code != 200 || cursor == "" {
		t.Fatalf("expected first page cursor, status=%d cursor=%q body=%s", rr.Code, cursor, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0]["name"] != "Alpha" || first[1]["name"] != "Beta" {
		t.Fatalf("unexpected unfiltered first page: %#v", first)
	}

	req = httptest.NewRequest("GET", "/api/v1/bbs?limit=2&cursor="+url.QueryEscape(cursor), nil)
	rr = httptest.NewRecorder()
	srv.listBBS(rr, req)
	var second []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if rr.Code != 200 || len(second) != 1 || second[0]["name"] != "Gamma" {
		t.Fatalf("unexpected second page: status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/v1/bbs?limit=2&country=NO&cursor="+url.QueryEscape(cursor), nil)
	rr = httptest.NewRecorder()
	srv.listBBS(rr, req)
	if rr.Code != 400 {
		t.Fatalf("expected filter/cursor mismatch 400, got %d", rr.Code)
	}

	for _, raw := range []string{
		"/api/v1/bbs?limit=101",
		"/api/v1/bbs?bogus=1",
		"/api/v1/bbs?source=telnetbbsguide&source=synchronet",
	} {
		req = httptest.NewRequest("GET", raw, nil)
		rr = httptest.NewRecorder()
		srv.listBBS(rr, req)
		if rr.Code != 400 {
			t.Fatalf("expected 400 for %s, got %d", raw, rr.Code)
		}
	}

	req = httptest.NewRequest("GET", "/api/v1/bbs?q=beta&protocol=ssh&source=synchronet", nil)
	rr = httptest.NewRecorder()
	srv.listBBS(rr, req)
	var filtered []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if rr.Code != 200 || len(filtered) != 1 || filtered[0]["name"] != "Beta" {
		t.Fatalf("unexpected filtered result: status=%d body=%s", rr.Code, rr.Body.String())
	}
}
