package probe

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

type Endpoint struct {
	ID       int64
	Protocol string
	Hostname string
	Port     int
}

type ProbeFunc func(context.Context, string, int) Result

type Worker struct {
	DB          *sql.DB
	Concurrency int
	TelnetProbe ProbeFunc
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
				result := telnetProbe(ctx, e.Hostname, e.Port)
				if _, err := w.DB.ExecContext(ctx, `INSERT INTO probe_result(endpoint_id,status,connect_ms,banner_bytes,error) VALUES(?,?,?,?,?)`, e.ID, result.Status, nullableConnectMS(result.ConnectMS), result.BannerBytes, result.Error); err != nil {
					errCh <- fmt.Errorf("store probe result for endpoint %d: %w", e.ID, err)
				}
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
		return 0, err
	}
	for err := range errCh {
		if err != nil {
			return 0, err
		}
	}

	count := 0
	for _, e := range endpoints {
		if e.Protocol == "telnet" {
			count++
		}
	}
	return count, nil
}

func nullableConnectMS(ms int64) any {
	if ms <= 0 {
		return nil
	}
	return ms
}
