package main

import (
	"net/http"
	"strconv"
)

type inflightLimiter struct {
	sem chan struct{}
}

func newInflightLimiter(max int) *inflightLimiter {
	if max < 1 {
		max = 64
	}
	return &inflightLimiter{sem: make(chan struct{}, max)}
}

func (l *inflightLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep liveness/readiness observable even when normal public traffic is saturated.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case l.sem <- struct{}{}:
			defer func() { <-l.sem }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", strconv.Itoa(1))
			http.Error(w, "server busy", http.StatusServiceUnavailable)
		}
	})
}
