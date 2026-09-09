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
	if _, err := s.DB.Exec(`INSERT INTO probe_result(endpoint_id,status,error) VALUES(?,?,?)`, endpointID, "offline", "connection refused"); err != nil {
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
}
