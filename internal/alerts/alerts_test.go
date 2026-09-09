package alerts_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/alerts"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestListProjectsCurrentAndDeduplicatedAlerts(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	res, err := s.DB.Exec(`INSERT INTO bbs(name,software) VALUES('Alert BBS','Mystic')`)
	if err != nil {
		t.Fatal(err)
	}
	bbsID, _ := res.LastInsertId()
	res, err = s.DB.Exec(`INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?)`, bbsID, "telnet", "alert.example", 23)
	if err != nil {
		t.Fatal(err)
	}
	endpointID, _ := res.LastInsertId()

	if _, err := s.DB.Exec(`INSERT INTO source_entry(bbs_id,source,source_key,reported_name,active,missing_since) VALUES(?,?,?,?,0,'2026-09-09 01:00:00')`, bbsID, "guide", "alert-key", "Alert BBS"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO source_health(source,current_state,state_changed_at,last_error) VALUES('guide','stale','2026-09-09 02:00:00','old data')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO probe_result(endpoint_id,checked_at,status,error) VALUES(?,'2026-09-09 02:15:00',?,?)`, endpointID, "offline", "connection refused"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO change_event(bbs_id,endpoint_id,occurred_at,kind,field,old_value,new_value) VALUES(?,?,?, 'software_changed','observed_software','Mystic','Synchronet')`, bbsID, endpointID, "2026-09-09 02:10:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO change_event(bbs_id,endpoint_id,occurred_at,kind,field,old_value,new_value) VALUES(?,?,?, 'software_changed','observed_software','Synchronet','WWIV')`, bbsID, endpointID, "2026-09-09 02:20:00"); err != nil {
		t.Fatal(err)
	}

	got, err := alerts.List(context.Background(), s.DB, 20, "info")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("alerts=%d, want 4: %#v", len(got), got)
	}
	if got[0].Severity != "high" || got[1].Severity != "high" {
		t.Fatalf("top severities=%q,%q, want high,high", got[0].Severity, got[1].Severity)
	}

	softwareChanges := 0
	for _, alert := range got {
		if alert.Category == "software_change" {
			softwareChanges++
			if !strings.Contains(alert.Detail, "Synchronet -> WWIV") {
				t.Fatalf("software alert detail=%q", alert.Detail)
			}
		}
	}
	if softwareChanges != 1 {
		t.Fatalf("software change alerts=%d, want 1", softwareChanges)
	}

	highOnly, err := alerts.List(context.Background(), s.DB, 20, "high")
	if err != nil {
		t.Fatal(err)
	}
	if len(highOnly) != 2 {
		t.Fatalf("high alerts=%d, want 2", len(highOnly))
	}

	missingOnly, err := alerts.Query(context.Background(), s.DB, alerts.Filter{Category: "source_missing", Source: "GUIDE", BBSID: bbsID})
	if err != nil {
		t.Fatal(err)
	}
	if len(missingOnly) != 1 || missingOnly[0].Category != "source_missing" {
		t.Fatalf("filtered alerts=%#v, want one source_missing", missingOnly)
	}

	stats, err := alerts.Summarize(context.Background(), s.DB, alerts.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 4 || stats.BySeverity["high"] != 2 || stats.BySeverity["warning"] != 1 || stats.BySeverity["info"] != 1 {
		t.Fatalf("stats=%#v", stats)
	}
	if stats.ByCategory["software_change"] != 1 || stats.ByCategory["endpoint_status"] != 1 || stats.BySource["guide"] != 2 {
		t.Fatalf("stats breakdown=%#v", stats)
	}

	page1, err := alerts.QueryPage(context.Background(), s.DB, alerts.Filter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 || page1.NextCursor == "" {
		t.Fatalf("page1=%#v", page1)
	}
	page2, err := alerts.QueryPage(context.Background(), s.DB, alerts.Filter{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 2 || page2.NextCursor != "" {
		t.Fatalf("page2=%#v", page2)
	}
	seen := map[string]bool{}
	for _, alert := range page1.Items {
		seen[alert.Key] = true
	}
	for _, alert := range page2.Items {
		if seen[alert.Key] {
			t.Fatalf("duplicate alert across pages: %s", alert.Key)
		}
	}

	recent, err := alerts.Query(context.Background(), s.DB, alerts.Filter{Since: "2026-09-09 02:05:00"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent alerts=%#v, want endpoint and software change", recent)
	}

	if _, err := alerts.QueryPage(context.Background(), s.DB, alerts.Filter{Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("invalid cursor unexpectedly accepted")
	}
}
