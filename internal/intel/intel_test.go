package intel_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Ploos-AS/BBSIntel/internal/intel"
	"github.com/Ploos-AS/BBSIntel/internal/store"
)

func TestForBBSSummarizesConflictsAndObservedSoftware(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bbsintel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	res, err := s.DB.Exec(`INSERT INTO bbs(name,software) VALUES('Test BBS','Mystic')`)
	if err != nil {
		t.Fatal(err)
	}
	bbsID, _ := res.LastInsertId()
	_, err = s.DB.Exec(`INSERT INTO source_entry(bbs_id,source,source_key,reported_name,reported_software,reported_country,reported_description) VALUES
(?,?,?,?,?,?,?),(?,?,?,?,?,?,?)`,
		bbsID, "one", "one", "Test BBS", "Mystic", "NO", "First description",
		bbsID, "two", "two", "Test BBS", "Synchronet", "NO", "Second description")
	if err != nil {
		t.Fatal(err)
	}
	res, err = s.DB.Exec(`INSERT INTO endpoint(bbs_id,protocol,hostname,port) VALUES(?,?,?,?)`, bbsID, "telnet", "bbs.example", 23)
	if err != nil {
		t.Fatal(err)
	}
	endpointID, _ := res.LastInsertId()
	_, err = s.DB.Exec(`INSERT INTO probe_result(endpoint_id,status,detected_software,software_confidence) VALUES(?,?,?,?)`, endpointID, "online", "Synchronet", 0.95)
	if err != nil {
		t.Fatal(err)
	}

	summary, err := intel.ForBBS(context.Background(), s.DB, "1")
	if err != nil {
		t.Fatal(err)
	}
	if summary.SourceCount != 2 {
		t.Fatalf("source count=%d, want 2", summary.SourceCount)
	}
	if summary.NameConflict || summary.CountryConflict {
		t.Fatalf("unexpected name/country conflict: %#v", summary)
	}
	if !summary.SoftwareConflict || !summary.DescriptionConflict || summary.ConflictCount != 2 {
		t.Fatalf("unexpected conflicts: %#v", summary)
	}
	if summary.SourceAgreementPct != 83.33333333333334 {
		t.Fatalf("agreement=%v, want 83.33333333333334", summary.SourceAgreementPct)
	}
	if summary.ObservedSoftware != "Synchronet" || !summary.ObservedMatchesSource {
		t.Fatalf("unexpected observed software summary: %#v", summary)
	}
}
