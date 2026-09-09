package source

import (
	"strings"
	"testing"
)

func TestParseTelnetBBSGuideTelnetAndSSH(t *testing.T) {
	html := `<html><body>
<h2>Example BBS</h2>
<div>Telnet: bbs.example.org:2323</div>
<div>SSH: bbs.example.org:2222</div>
<div>Software: Mystic</div>
</body></html>`
	entries, err := parseTelnetBBSGuide(strings.NewReader(html), "https://example.invalid/list")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries)=%d, want 2", len(entries))
	}
	if entries[0].Protocol != "telnet" || entries[0].Hostname != "bbs.example.org" || entries[0].Port != 2323 {
		t.Fatalf("telnet entry=%+v", entries[0])
	}
	if entries[0].SourceKey != "bbs.example.org:2323" {
		t.Fatalf("telnet source key=%q", entries[0].SourceKey)
	}
	if entries[1].Protocol != "ssh" || entries[1].Hostname != "bbs.example.org" || entries[1].Port != 2222 {
		t.Fatalf("ssh entry=%+v", entries[1])
	}
	if entries[1].SourceKey != "ssh:bbs.example.org:2222" {
		t.Fatalf("ssh source key=%q", entries[1].SourceKey)
	}
	if entries[1].Name != "Example BBS" {
		t.Fatalf("ssh name=%q, want Example BBS", entries[1].Name)
	}
}

func TestParseSSHDefaultPort(t *testing.T) {
	html := `<html><body><h2>Secure BBS</h2><div>SSH:</div><div>ssh.example.net</div><div>Software: Synchronet</div></body></html>`
	entries, err := parseTelnetBBSGuide(strings.NewReader(html), "https://example.invalid/list")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries)=%d, want 1", len(entries))
	}
	if entries[0].Port != 22 || entries[0].Name != "Secure BBS" {
		t.Fatalf("entry=%+v", entries[0])
	}
}
