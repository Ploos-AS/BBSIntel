package probe_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/probe"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestWorkerStoresProbeResult(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	res, err := s.DB.Exec(`INSERT INTO bbs(name) VALUES('Test BBS')`)
	if err != nil {
		t.Fatal(err)
	}
	bbsID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?)`, bbsID, "telnet", "example.invalid", 2323); err != nil {
		t.Fatal(err)
	}

	fake := func(context.Context, string, int) probe.Result {
		return probe.Result{Status: "online", ConnectMS: 42, BannerBytes: 128}
	}
	count, err := (probe.Worker{DB: s.DB, Concurrency: 2, TelnetProbe: fake}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}

	var status string
	var connectMS, bannerBytes int
	if err := s.DB.QueryRow(`SELECT status,connect_ms,banner_bytes FROM probe_result`).Scan(&status, &connectMS, &bannerBytes); err != nil {
		t.Fatal(err)
	}
	if status != "online" || connectMS != 42 || bannerBytes != 128 {
		t.Fatalf("stored result=(%q,%d,%d), want (online,42,128)", status, connectMS, bannerBytes)
	}
}
