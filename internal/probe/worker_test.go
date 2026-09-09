package probe_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ploos-AS/BBSIntel/internal/probe"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestWorkerStoresProbeResult(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	insertEndpoint(t, s, "example.invalid", 2323)

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

func TestWorkerBacksOffRepeatedFailures(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	endpointID := insertEndpoint(t, s, "dead.invalid", 23)
	base := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		checked := base.Add(time.Duration(i) * 30 * time.Minute).Format("2006-01-02 15:04:05")
		if _, err := s.DB.Exec(`INSERT INTO probe_result(endpoint_id,checked_at,status,error) VALUES(?,?,?,?)`, endpointID, checked, "offline", "timeout"); err != nil {
			t.Fatal(err)
		}
	}

	var calls int32
	fake := func(context.Context, string, int) probe.Result {
		atomic.AddInt32(&calls, 1)
		return probe.Result{Status: "online"}
	}
	worker := probe.Worker{
		DB:           s.DB,
		Concurrency:  1,
		TelnetProbe:  fake,
		BaseInterval: 30 * time.Minute,
		Now:          func() time.Time { return base.Add(2*time.Hour + 30*time.Minute) },
	}
	if _, err := worker.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("probe calls=%d, want 0 during 6h backoff", got)
	}

	worker.Now = func() time.Time { return base.Add(7 * time.Hour) }
	if _, err := worker.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("probe calls=%d, want 1 after backoff expires", got)
	}
}

func TestHealthyEndpointUsesBaseInterval(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	endpointID := insertEndpoint(t, s, "good.invalid", 23)
	base := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	if _, err := s.DB.Exec(`INSERT INTO probe_result(endpoint_id,checked_at,status) VALUES(?,?,?)`, endpointID, base.Format("2006-01-02 15:04:05"), "online"); err != nil {
		t.Fatal(err)
	}

	var calls int32
	fake := func(context.Context, string, int) probe.Result {
		atomic.AddInt32(&calls, 1)
		return probe.Result{Status: "online"}
	}
	worker := probe.Worker{
		DB:           s.DB,
		Concurrency:  1,
		TelnetProbe:  fake,
		BaseInterval: 30 * time.Minute,
		Now:          func() time.Time { return base.Add(31 * time.Minute) },
	}
	if _, err := worker.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("probe calls=%d, want 1", got)
	}
}

func insertEndpoint(t *testing.T, s *store.Store, host string, port int) int64 {
	t.Helper()
	res, err := s.DB.Exec(`INSERT INTO bbs(name) VALUES('Test BBS')`)
	if err != nil {
		t.Fatal(err)
	}
	bbsID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	res, err = s.DB.Exec(`INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?)`, bbsID, "telnet", host, port)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
