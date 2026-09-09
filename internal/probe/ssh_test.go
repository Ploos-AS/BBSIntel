package probe

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestSSHReadsIdentificationWithoutAuthentication(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("notice before banner\r\nSSH-2.0-ExampleBBS_1.0\r\n"))
	}()

	host, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := SSH(ctx, host, port)
	<-done

	if result.Status != "online" {
		t.Fatalf("status=%q, want online", result.Status)
	}
	if result.BannerPreview != "SSH-2.0-ExampleBBS_1.0" {
		t.Fatalf("banner=%q", result.BannerPreview)
	}
	if result.BannerSHA256 == "" {
		t.Fatal("expected SSH identification hash")
	}
}
