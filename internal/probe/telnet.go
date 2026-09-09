package probe

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

type Result struct {
	Status      string `json:"status"`
	ConnectMS   int64  `json:"connect_ms"`
	BannerBytes int    `json:"banner_bytes"`
	Error       string `json:"error,omitempty"`
}

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
		return Result{Status: "online", ConnectMS: connectMS, BannerBytes: n}
	}
	if err != nil && !errors.Is(err, io.EOF) {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return Result{Status: "tcp_only", ConnectMS: connectMS}
		}
		return Result{Status: "tcp_only", ConnectMS: connectMS, Error: err.Error()}
	}
	return Result{Status: "tcp_only", ConnectMS: connectMS}
}
