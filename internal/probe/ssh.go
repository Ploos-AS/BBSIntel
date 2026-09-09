package probe

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const maxSSHPreBannerBytes = 4096

func SSH(ctx context.Context, hostname string, port int) Result {
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

	r := bufio.NewReaderSize(conn, 1024)
	total := 0
	for total < maxSSHPreBannerBytes {
		line, err := readBoundedSSHLine(r, maxSSHPreBannerBytes-total)
		total += len(line)
		trimmed := strings.TrimSpace(string(line))
		if strings.HasPrefix(trimmed, "SSH-") {
			if len(trimmed) > 255 {
				return Result{Status: "tcp_only", ConnectMS: connectMS, Error: "SSH identification exceeds 255 bytes"}
			}
			sum := sha256.Sum256([]byte(trimmed))
			return Result{
				Status:        "online",
				ConnectMS:     connectMS,
				BannerBytes:   len(trimmed),
				BannerSHA256:  hex.EncodeToString(sum[:]),
				BannerPreview: trimmed,
			}
		}
		if err != nil {
			break
		}
	}
	return Result{Status: "tcp_only", ConnectMS: connectMS}
}

func readBoundedSSHLine(r *bufio.Reader, remaining int) ([]byte, error) {
	if remaining <= 0 {
		return nil, io.EOF
	}
	line := make([]byte, 0, min(remaining, 256))
	for len(line) < remaining {
		fragment, err := r.ReadSlice('\n')
		if len(fragment) > remaining-len(line) {
			fragment = fragment[:remaining-len(line)]
		}
		line = append(line, fragment...)
		if err == nil {
			return line, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
	return line, io.ErrShortBuffer
}
