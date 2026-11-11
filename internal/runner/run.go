// Package runner drives a load test: it fans requests out to a fixed pool
// of workers and gathers one sample per request.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Samir120/loadr/internal/stats"
)

// Result summarises a completed run.
type Result struct {
	// Requests is the number of requests attempted, failed ones included.
	Requests int

	// Errors is the number of requests that failed.
	Errors int

	// Elapsed is the wall-clock time of the whole run.
	Elapsed time.Duration
}

// Run performs the load test described by cfg, which must be valid.
//
// Failed requests are counted in the Result, not returned as an error; the
// error is reserved for problems with the run itself.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Duration > 0 {
		return Result{}, errors.New("duration mode is not implemented yet")
	}

	client := http.DefaultClient
	samples := make(chan stats.Sample, cfg.Concurrency)
	done := make(chan Result)
	go func() {
		done <- collect(samples)
	}()

	var (
		claimed atomic.Int64
		wg      sync.WaitGroup
	)
	limit := int64(cfg.Requests)
	start := time.Now()
	for range cfg.Concurrency {
		wg.Go(func() {
			for claimed.Add(1) <= limit {
				samples <- send(ctx, client, cfg)
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	close(samples)

	res := <-done
	res.Elapsed = elapsed
	return res, nil
}

// collect is the only reader of samples and the only writer of the Result,
// so neither needs a lock. It returns once samples is closed and drained.
func collect(samples <-chan stats.Sample) Result {
	var res Result
	for s := range samples {
		res.Requests++
		if s.Err != nil {
			res.Errors++
		}
	}
	return res
}

// send performs one request and reports what happened. Failures are part of
// the measurement, so they travel inside the Sample instead of as an error.
func send(ctx context.Context, client *http.Client, cfg Config) stats.Sample {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return stats.Sample{Err: fmt.Errorf("building request: %w", err)}
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return stats.Sample{Latency: time.Since(start), Err: fmt.Errorf("sending request: %w", err)}
	}
	defer resp.Body.Close()

	n, err := io.Copy(io.Discard, resp.Body)
	s := stats.Sample{Latency: time.Since(start), Status: resp.StatusCode, Bytes: n}
	if err != nil {
		s.Err = fmt.Errorf("reading body: %w", err)
	}
	return s
}
