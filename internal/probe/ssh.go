package probe

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

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

	r := bufio.NewReaderSize(conn, 4096)
	for i := 0; i < 8; i++ {
		line, err := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SSH-") {
			if len(line) > 255 {
				line = line[:255]
			}
			sum := sha256.Sum256([]byte(line))
			return Result{
				Status:        "online",
				ConnectMS:     connectMS,
				BannerBytes:   len(line),
				BannerSHA256:  hex.EncodeToString(sum[:]),
				BannerPreview: line,
			}
		}
		if err != nil {
			break
		}
	}
	return Result{Status: "tcp_only", ConnectMS: connectMS}
}
