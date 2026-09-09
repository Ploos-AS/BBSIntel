package probe

import "testing"

func TestCleanTelnetBannerAndDetectSoftware(t *testing.T) {
	input := []byte{255, 251, 1, 27, '[', '3', '1', 'm', 'S', 'y', 'n', 'c', 'h', 'r', 'o', 'n', 'e', 't', 27, '[', '0', 'm', '\r', '\n'}
	clean := cleanTelnetBanner(input)
	preview := bannerPreview(clean, 240)
	if preview != "Synchronet" {
		t.Fatalf("preview=%q, want Synchronet", preview)
	}
	if got := detectSoftware(preview); got != "Synchronet" {
		t.Fatalf("detected software=%q, want Synchronet", got)
	}
}

func TestCleanTelnetBannerSkipsSubnegotiation(t *testing.T) {
	input := []byte{'M', 'y', 's', 't', 'i', 'c', ' ', 255, 250, 24, 1, 2, 3, 255, 240, 'B', 'B', 'S'}
	preview := bannerPreview(cleanTelnetBanner(input), 240)
	if preview != "Mystic BBS" {
		t.Fatalf("preview=%q, want Mystic BBS", preview)
	}
	if got := detectSoftware(preview); got != "Mystic" {
		t.Fatalf("detected software=%q, want Mystic", got)
	}
}
