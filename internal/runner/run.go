// Package runner drives a load test: it fans requests out to a fixed pool
// of workers and gathers one sample per request.
package runner

import (
	"bytes"
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
//
// The embedded Summary covers completed requests only, failed ones included.
// Requests abandoned because the run ended are not counted.
type Result struct {
	stats.Summary

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
// When cfg.Rate is set, all workers wait on one shared ticker, so the run as
// a whole sends at most cfg.Rate requests per second. Under heavy load it
// falls short of that rate; it never exceeds it.
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
	done := make(chan stats.Summary)
	go func() {
		done <- collect(samples, cfg.Completed)
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

	// tick stays nil without a rate limit. The ticker starts after start is
	// taken, so its n-th tick, and with it the n-th request, comes no
	// earlier than n/Rate into the run.
	var tick <-chan time.Time
	if cfg.Rate > 0 {
		ticker := time.NewTicker(time.Second / time.Duration(cfg.Rate))
		defer ticker.Stop()
		tick = ticker.C
	}

	for range cfg.Concurrency {
		wg.Go(func() {
			for ctx.Err() == nil && claimed.Add(1) <= limit {
				if !wait(ctx, tick) {
					return // the run ended while waiting for the rate limiter
				}
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

	return Result{Summary: <-done, Elapsed: elapsed}, nil
}

// wait blocks until the rate limiter allows the next request and reports
// whether it did; false means ctx ended first. A nil tick means no limit.
// It is checked before the select because a receive from a nil channel
// blocks forever, which would leave only ctx.Done as a way out.
func wait(ctx context.Context, tick <-chan time.Time) bool {
	if tick == nil {
		return true
	}
	select {
	case <-tick:
		return true
	case <-ctx.Done():
		return false
	}
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

// collect is the only goroutine that touches the Collector, so it needs no
// lock. It returns once samples is closed and drained. When completed is
// non-nil it is kept equal to the number of samples added so far.
func collect(samples <-chan stats.Sample, completed *atomic.Int64) stats.Summary {
	var c stats.Collector
	for s := range samples {
		c.Add(s)
		if completed != nil {
			completed.Add(1)
		}
	}
	return c.Summary()
}

// send performs one request and reports what happened. Failures are part of
// the measurement, so they travel inside the Sample instead of as an error.
func send(ctx context.Context, client *http.Client, cfg Config) stats.Sample {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	req, err := newRequest(ctx, cfg)
	if err != nil {
		return stats.Sample{Err: err}
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

// newRequest builds a fresh request from cfg. A request is not safe to share
// between goroutines, so every call gets its own header map and body reader;
// cfg itself is only read.
func newRequest(ctx context.Context, cfg Config) (*http.Request, error) {
	var body io.Reader
	if len(cfg.Body) > 0 {
		body = bytes.NewReader(cfg.Body)
	}
	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if cfg.Header != nil {
		req.Header = cfg.Header.Clone()
	}
	if host := req.Header.Get("Host"); host != "" {
		req.Host = host
	}
	return req, nil
}
