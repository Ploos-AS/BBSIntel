package probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Result struct {
	Status           string `json:"status"`
	ConnectMS        int64  `json:"connect_ms"`
	BannerBytes      int    `json:"banner_bytes"`
	BannerSHA256     string `json:"banner_sha256,omitempty"`
	BannerPreview    string `json:"banner_preview,omitempty"`
	DetectedSoftware string `json:"detected_software,omitempty"`
	Error            string `json:"error,omitempty"`
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func Telnet(ctx context.Context, hostname string, port int) Result {
	start := time.Now()
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(hostname, strconv.Itoa(port)))
	if err != nil {
		status := "offline"
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			status = "dns_fail"
		}
		return Result{Status: status, Error: err.Error()}
	}
	defer conn.Close()
	connectMS := time.Since(start).Milliseconds()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if n > 0 {
		clean := cleanTelnetBanner(buf[:n])
		sum := sha256.Sum256(clean)
		preview := bannerPreview(clean, 240)
		return Result{
			Status:           "online",
			ConnectMS:        connectMS,
			BannerBytes:      n,
			BannerSHA256:     hex.EncodeToString(sum[:]),
			BannerPreview:    preview,
			DetectedSoftware: detectSoftware(preview),
		}
	}
	if err != nil && !errors.Is(err, io.EOF) {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return Result{Status: "tcp_only", ConnectMS: connectMS}
		}
		return Result{Status: "tcp_only", ConnectMS: connectMS, Error: err.Error()}
	}
	return Result{Status: "tcp_only", ConnectMS: connectMS}
}

func cleanTelnetBanner(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); {
		if in[i] != 255 {
			out = append(out, in[i])
			i++
			continue
		}
		if i+1 >= len(in) {
			break
		}
		cmd := in[i+1]
		switch cmd {
		case 255:
			out = append(out, 255)
			i += 2
		case 251, 252, 253, 254:
			if i+2 < len(in) {
				i += 3
			} else {
				i = len(in)
			}
		case 250:
			i += 2
			for i+1 < len(in) {
				if in[i] == 255 && in[i+1] == 240 {
					i += 2
					break
				}
				i++
			}
		default:
			i += 2
		}
	}
	out = ansiRE.ReplaceAll(out, nil)
	out = bytes.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' || unicode.IsPrint(r) {
			return r
		}
		return -1
	}, out)
	return out
}

func bannerPreview(in []byte, limit int) string {
	s := strings.TrimSpace(string(in))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > limit {
		s = s[:limit]
	}
	return s
}

func detectSoftware(preview string) string {
	s := strings.ToLower(preview)
	signatures := []struct {
		needle string
		name   string
	}{
		{"synchronet", "Synchronet"},
		{"mystic bbs", "Mystic"},
		{"mystic", "Mystic"},
		{"enigma 1/2", "ENiGMA 1/2"},
		{"enigma1/2", "ENiGMA 1/2"},
		{"wwiv", "WWIV"},
		{"renegade bbs", "Renegade"},
		{"telegard", "Telegard"},
		{"wildcat!", "Wildcat!"},
		{"pcboard", "PCBoard"},
		{"majorbbs", "MajorBBS"},
		{"worldgroup", "Worldgroup"},
		{"citadel", "Citadel"},
	}
	for _, sig := range signatures {
		if strings.Contains(s, sig.needle) {
			return sig.name
		}
	}
	return ""
}
