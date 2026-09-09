package probe

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Endpoint struct {
	ID       int64
	Protocol string
	Hostname string
	Port     int
}

type ProbeFunc func(context.Context, string, int) Result

type Worker struct {
	DB           *sql.DB
	Concurrency  int
	TelnetProbe  ProbeFunc
	Now          func() time.Time
	BaseInterval time.Duration
}

func (w Worker) Run(ctx context.Context) (int, error) {
	if w.DB == nil {
		return 0, fmt.Errorf("probe worker: nil database")
	}
	concurrency := w.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	telnetProbe := w.TelnetProbe
	if telnetProbe == nil {
		telnetProbe = Telnet
	}
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	baseInterval := w.BaseInterval
	if baseInterval <= 0 {
		baseInterval = 30 * time.Minute
	}

	rows, err := w.DB.QueryContext(ctx, `SELECT id,protocol,hostname,port FROM endpoint ORDER BY id`)
	if err != nil {
		return 0, err
	}

	var endpoints []Endpoint
	for rows.Next() {
		var e Endpoint
		if err := rows.Scan(&e.ID, &e.Protocol, &e.Hostname, &e.Port); err != nil {
			rows.Close()
			return 0, err
		}
		endpoints = append(endpoints, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	jobs := make(chan Endpoint)
	errCh := make(chan error, len(endpoints))
	var probed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range jobs {
				if ctx.Err() != nil {
					return
				}
				if e.Protocol != "telnet" {
					continue
				}
				due, err := w.endpointDue(ctx, e.ID, now(), baseInterval)
				if err != nil {
					errCh <- err
					continue
				}
				if !due {
					continue
				}
				result := telnetProbe(ctx, e.Hostname, e.Port)
				if _, err := w.DB.ExecContext(ctx, `INSERT INTO probe_result(endpoint_id,status,connect_ms,banner_bytes,error) VALUES(?,?,?,?,?)`, e.ID, result.Status, nullableConnectMS(result.ConnectMS), result.BannerBytes, result.Error); err != nil {
					errCh <- fmt.Errorf("store probe result for endpoint %d: %w", e.ID, err)
					continue
				}
				probed.Add(1)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, e := range endpoints {
			select {
			case <-ctx.Done():
				return
			case jobs <- e:
			}
		}
	}()

	wg.Wait()
	close(errCh)
	if err := ctx.Err(); err != nil {
		return int(probed.Load()), err
	}
	for err := range errCh {
		if err != nil {
			return int(probed.Load()), err
		}
	}
	return int(probed.Load()), nil
}

func (w Worker) endpointDue(ctx context.Context, endpointID int64, now time.Time, base time.Duration) (bool, error) {
	rows, err := w.DB.QueryContext(ctx, `SELECT status,checked_at FROM probe_result WHERE endpoint_id=? ORDER BY checked_at DESC,id DESC LIMIT 4`, endpointID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var statuses []string
	var latest time.Time
	for rows.Next() {
		var status, checkedAt string
		if err := rows.Scan(&status, &checkedAt); err != nil {
			return false, err
		}
		t, err := time.Parse("2006-01-02 15:04:05", checkedAt)
		if err != nil {
			return false, fmt.Errorf("parse probe timestamp for endpoint %d: %w", endpointID, err)
		}
		if latest.IsZero() {
			latest = t
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(statuses) == 0 {
		return true, nil
	}

	interval := backoffInterval(statuses, base)
	return !now.Before(latest.Add(interval)), nil
}

func backoffInterval(statuses []string, base time.Duration) time.Duration {
	if len(statuses) == 0 || isHealthy(statuses[0]) {
		return base
	}
	failures := 0
	for _, status := range statuses {
		if isHealthy(status) {
			break
		}
		failures++
	}
	switch {
	case failures >= 4:
		return 24 * time.Hour
	case failures == 3:
		return 6 * time.Hour
	case failures == 2:
		return time.Hour
	default:
		return base
	}
}

func isHealthy(status string) bool {
	return status == "online" || status == "tcp_only"
}

func nullableConnectMS(ms int64) any {
	if ms <= 0 {
		return nil
	}
	return ms
}
