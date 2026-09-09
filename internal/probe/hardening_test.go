package probe

import (
	"bufio"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBackoffNeverShorterThanBase(t *testing.T) {
	base := 8 * time.Hour
	if got := backoffInterval([]string{"offline", "offline"}, base); got != base {
		t.Fatalf("two-failure backoff=%s, want base %s", got, base)
	}
	if got := backoffInterval([]string{"offline", "offline", "offline"}, base); got != base {
		t.Fatalf("three-failure backoff=%s, want base %s", got, base)
	}
	if got := backoffInterval([]string{"offline", "offline", "offline", "offline"}, 30*time.Hour); got != 30*time.Hour {
		t.Fatalf("four-failure backoff=%s, want 30h base", got)
	}
}

func TestTelnetNegotiationOnlyCleansToEmpty(t *testing.T) {
	clean := cleanTelnetBanner([]byte{255, 251, 1, 255, 253, 3})
	if len(clean) != 0 {
		t.Fatalf("clean negotiation=%v, want empty", clean)
	}
}

func TestBannerPreviewDoesNotSplitUTF8(t *testing.T) {
	preview := bannerPreview([]byte("åååå"), 5)
	if !utf8.ValidString(preview) {
		t.Fatalf("preview is not valid UTF-8: %q", preview)
	}
	if len(preview) > 5 {
		t.Fatalf("preview bytes=%d, want <=5", len(preview))
	}
}

func TestReadBoundedSSHLineCapsLongInput(t *testing.T) {
	r := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", 10000)), 32)
	line, err := readBoundedSSHLine(r, 128)
	if err == nil {
		t.Fatal("expected bounded read error")
	}
	if len(line) != 128 {
		t.Fatalf("line bytes=%d, want 128", len(line))
	}
}
