package intel

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type Summary struct {
	SourceCount               int      `json:"source_count"`
	NameConflict              bool     `json:"name_conflict"`
	SoftwareConflict          bool     `json:"software_conflict"`
	CountryConflict           bool     `json:"country_conflict"`
	DescriptionConflict       bool     `json:"description_conflict"`
	ConflictCount             int      `json:"conflict_count"`
	SourceAgreementPct        float64  `json:"source_agreement_pct"`
	Names                     []string `json:"names"`
	ReportedSoftware          []string `json:"reported_software"`
	Countries                 []string `json:"countries"`
	ObservedSoftware          string   `json:"observed_software"`
	ObservedSoftwareConfidence float64 `json:"observed_software_confidence"`
	ObservedMatchesSource     bool     `json:"observed_matches_source"`
}

type sourceRow struct {
	name        string
	software    string
	country     string
	description string
}

func ForBBS(ctx context.Context, db *sql.DB, bbsID string) (Summary, error) {
	var out Summary
	if db == nil {
		return out, fmt.Errorf("intel: nil database")
	}

	rows, err := db.QueryContext(ctx, `
SELECT reported_name,reported_software,reported_country,reported_description
FROM source_entry WHERE bbs_id=? ORDER BY source,source_key`, bbsID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	var sources []sourceRow
	for rows.Next() {
		var r sourceRow
		if err := rows.Scan(&r.name, &r.software, &r.country, &r.description); err != nil {
			return out, err
		}
		sources = append(sources, r)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.SourceCount = len(sources)

	nameStats := valueStats(sources, func(r sourceRow) string { return r.name })
	softwareStats := valueStats(sources, func(r sourceRow) string { return r.software })
	countryStats := valueStats(sources, func(r sourceRow) string { return r.country })
	descriptionStats := valueStats(sources, func(r sourceRow) string { return r.description })

	out.Names = nameStats.values
	out.ReportedSoftware = softwareStats.values
	out.Countries = countryStats.values
	out.NameConflict = len(nameStats.values) > 1
	out.SoftwareConflict = len(softwareStats.values) > 1
	out.CountryConflict = len(countryStats.values) > 1
	out.DescriptionConflict = len(descriptionStats.values) > 1
	for _, conflict := range []bool{out.NameConflict, out.SoftwareConflict, out.CountryConflict, out.DescriptionConflict} {
		if conflict {
			out.ConflictCount++
		}
	}

	var ratios []float64
	for _, st := range []stats{nameStats, softwareStats, countryStats} {
		if st.nonEmpty > 0 {
			ratios = append(ratios, float64(st.maxCount)/float64(st.nonEmpty))
		}
	}
	if len(ratios) > 0 {
		var total float64
		for _, r := range ratios {
			total += r
		}
		out.SourceAgreementPct = total / float64(len(ratios)) * 100
	}

	_ = db.QueryRowContext(ctx, `
SELECT COALESCE(p.detected_software,''),COALESCE(p.software_confidence,0)
FROM probe_result p JOIN endpoint e ON e.id=p.endpoint_id
WHERE e.bbs_id=? AND trim(p.detected_software)<>''
ORDER BY p.checked_at DESC,p.id DESC LIMIT 1`, bbsID).Scan(&out.ObservedSoftware, &out.ObservedSoftwareConfidence)

	if out.ObservedSoftware != "" {
		observed := normalize(out.ObservedSoftware)
		for _, value := range out.ReportedSoftware {
			if normalize(value) == observed {
				out.ObservedMatchesSource = true
				break
			}
		}
	}
	return out, nil
}

type stats struct {
	values   []string
	nonEmpty int
	maxCount int
}

func valueStats(rows []sourceRow, get func(sourceRow) string) stats {
	counts := map[string]int{}
	display := map[string]string{}
	var out stats
	for _, row := range rows {
		value := strings.TrimSpace(get(row))
		if value == "" {
			continue
		}
		key := normalize(value)
		counts[key]++
		out.nonEmpty++
		if _, ok := display[key]; !ok {
			display[key] = value
		}
		if counts[key] > out.maxCount {
			out.maxCount = counts[key]
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out.values = append(out.values, display[key])
	}
	return out
}

func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}
