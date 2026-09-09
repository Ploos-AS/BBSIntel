package probe

import "testing"

func TestCleanTelnetBannerAndDetectSoftware(t *testing.T) {
	input := []byte{255, 251, 1, 27, '[', '3', '1', 'm', 'S', 'y', 'n', 'c', 'h', 'r', 'o', 'n', 'e', 't', 27, '[', '0', 'm', '\r', '\n'}
	clean := cleanTelnetBanner(input)
	preview := bannerPreview(clean, 240)
	if preview != "Synchronet" {
		t.Fatalf("preview=%q, want Synchronet", preview)
	}
	software, confidence, evidence := detectSoftware(preview)
	if software != "Synchronet" {
		t.Fatalf("detected software=%q, want Synchronet", software)
	}
	if confidence < 0.9 {
		t.Fatalf("confidence=%f, want >=0.9", confidence)
	}
	if evidence != "synchronet" {
		t.Fatalf("evidence=%q, want synchronet", evidence)
	}
}

func TestCleanTelnetBannerSkipsSubnegotiation(t *testing.T) {
	input := []byte{'M', 'y', 's', 't', 'i', 'c', ' ', 255, 250, 24, 1, 2, 3, 255, 240, 'B', 'B', 'S'}
	preview := bannerPreview(cleanTelnetBanner(input), 240)
	if preview != "Mystic BBS" {
		t.Fatalf("preview=%q, want Mystic BBS", preview)
	}
	software, confidence, evidence := detectSoftware(preview)
	if software != "Mystic" {
		t.Fatalf("detected software=%q, want Mystic", software)
	}
	if confidence != 0.99 || evidence != "mystic bbs" {
		t.Fatalf("fingerprint=(%f,%q), want (0.99,mystic bbs)", confidence, evidence)
	}
}
