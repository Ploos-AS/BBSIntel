package source

import (
	"strings"
	"testing"
)

func TestParseSynchronet(t *testing.T) {
	html := `<table><tr><th>BBS</th><th>Since</th><th>Operators</th><th>Location</th><th>Terminal Services</th><th>Networks</th><th>Verification Results</th></tr>
<tr><td>Example BBS<br>Example description</td><td>2024</td><td>Sysop</td><td>Oslo, Norway</td><td>
example.org (telnet) ANSI<br>
example.org:2222 (ssh) SSH
example.org (rlogin)
</td><td>DOVE-Net</td><td>2026-09-08 Synchronet BBS for Linux Version 3.22</td></tr></table>`
	entries, err := parseSynchronet(strings.NewReader(html), "https://example.test/list")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(entries))
	}
	if entries[0].Name != "Example BBS" || entries[0].Software != "Synchronet" || entries[0].Protocol != "telnet" || entries[0].Hostname != "example.org" || entries[0].Port != 23 {
		t.Fatalf("unexpected telnet entry: %#v", entries[0])
	}
	if entries[1].Protocol != "ssh" || entries[1].Port != 2222 {
		t.Fatalf("unexpected ssh entry: %#v", entries[1])
	}
	if entries[0].SourceKey != entries[1].SourceKey {
		t.Fatalf("source keys differ: %q vs %q", entries[0].SourceKey, entries[1].SourceKey)
	}
	if entries[0].Description != "Example description" {
		t.Fatalf("description=%q", entries[0].Description)
	}
}
