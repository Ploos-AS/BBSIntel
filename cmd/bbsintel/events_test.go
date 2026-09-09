package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestEventFeedPaginationAndSince(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	res, err := s.DB.Exec(`INSERT INTO bbs(name) VALUES('Cursor BBS')`)
	if err != nil {
		t.Fatal(err)
	}
	bbsID, _ := res.LastInsertId()
	for _, occurredAt := range []string{"2026-09-09 02:00:00", "2026-09-09 03:00:00", "2026-09-09 03:00:00"} {
		if _, err := s.DB.Exec(`INSERT INTO change_event(bbs_id,occurred_at,kind) VALUES(?,?, 'test')`, bbsID, occurredAt); err != nil {
			t.Fatal(err)
		}
	}

	srv := &server{db: s.DB}
	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=1", nil)
	firstRec := httptest.NewRecorder()
	srv.listEvents(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", firstRec.Code, firstRec.Body.String())
	}
	var first []eventView
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].ID != 3 {
		t.Fatalf("first page=%#v, want id 3", first)
	}
	cursor := firstRec.Header().Get("X-Next-Cursor")
	if cursor == "" {
		t.Fatal("first page missing next cursor")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=1&cursor="+cursor, nil)
	secondRec := httptest.NewRecorder()
	srv.listEvents(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", secondRec.Code, secondRec.Body.String())
	}
	var second []eventView
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].ID != 2 {
		t.Fatalf("second page=%#v, want id 2", second)
	}

	sinceReq := httptest.NewRequest(http.MethodGet, "/api/v1/events?since=2026-09-09T02:30:00Z", nil)
	sinceRec := httptest.NewRecorder()
	srv.listEvents(sinceRec, sinceReq)
	if sinceRec.Code != http.StatusOK {
		t.Fatalf("since status=%d body=%s", sinceRec.Code, sinceRec.Body.String())
	}
	var recent []eventView
	if err := json.Unmarshal(sinceRec.Body.Bytes(), &recent); err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("since events=%#v, want 2", recent)
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/events?cursor=bad", nil)
	badRec := httptest.NewRecorder()
	srv.listEvents(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor status=%d, want 400", badRec.Code)
	}
}
