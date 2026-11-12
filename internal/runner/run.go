// Package runner drives a load test: it fans requests out to a fixed pool
// of workers and gathers one sample per request.
package runner

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Samir120/loadr/internal/stats"
)

// Result summarises a run.
type Result struct {
	// Requests is the number of requests that completed, failed ones
	// included. Requests abandoned because the run ended are not counted.
	Requests int

	// Errors is the number of completed requests that failed.
	Errors int

	// Elapsed is the wall-clock time of the whole run.
	Elapsed time.Duration
}

// Run performs the load test described by cfg, which must be valid.
//
// The run ends when cfg.Requests have been sent, when cfg.Duration has
// passed, or when ctx is cancelled, whichever comes first. Requests still in
// flight at that point are abandoned and left out of the Result, so a
// cancelled run returns a partial Result and a nil error.
//
// Failed requests are counted in the Result, not returned as an error; the
// error is reserved for problems with the run itself.
func Run(ctx context.Context, cfg Config) (Result, error) {
	client, err := newClient(cfg.Concurrency)
	if err != nil {
		return Result{}, err
	}
	defer client.CloseIdleConnections()

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
	if cfg.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Duration)
		defer cancel()
		limit = math.MaxInt64
	}

	for range cfg.Concurrency {
		wg.Go(func() {
			for ctx.Err() == nil && claimed.Add(1) <= limit {
				s := send(ctx, client, cfg)
				if s.Err != nil && ctx.Err() != nil {
					return // abandoned because the run ended; not a measurement
				}
				samples <- s
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

// newClient returns a client whose connection pool matches the worker pool.
// The default transport keeps only two idle connections per host, so with
// more workers than that most requests would dial a fresh connection.
func newClient(concurrency int) (*http.Client, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is %T, want *http.Transport", http.DefaultTransport)
	}
	t := base.Clone()
	t.MaxIdleConns = concurrency
	t.MaxIdleConnsPerHost = concurrency
	t.MaxConnsPerHost = concurrency
	return &http.Client{Transport: t}, nil
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
