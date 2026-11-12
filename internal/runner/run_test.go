package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	t.Run("sends exactly n requests", func(t *testing.T) {
		tests := []struct {
			name        string
			requests    int
			concurrency int
		}{
			{name: "one worker", requests: 10, concurrency: 1},
			{name: "done criterion", requests: 1000, concurrency: 50},
			{name: "uneven split", requests: 101, concurrency: 7},
			{name: "more workers than requests", requests: 3, concurrency: 50},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var hits atomic.Int64
				srv := newServer(t, func(http.ResponseWriter, *http.Request) {
					hits.Add(1)
				})

				res, err := Run(context.Background(), testConfig(srv.URL, tt.requests, tt.concurrency))
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				if got := hits.Load(); got != int64(tt.requests) {
					t.Errorf("server saw %d requests, want %d", got, tt.requests)
				}
				if res.Requests != tt.requests {
					t.Errorf("Result.Requests = %d, want %d", res.Requests, tt.requests)
				}
				if res.Errors != 0 {
					t.Errorf("Result.Errors = %d, want 0", res.Errors)
				}
			})
		}
	})

	t.Run("runs workers concurrently", func(t *testing.T) {
		const workers = 5
		var arrived atomic.Int64
		release := make(chan struct{})
		srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
			if arrived.Add(1) == workers {
				close(release)
			}
			select {
			case <-release:
			case <-r.Context().Done():
			}
		})

		cfg := testConfig(srv.URL, workers, workers)
		cfg.Timeout = time.Second
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if res.Errors != 0 {
			t.Errorf("Result.Errors = %d, want 0: requests did not overlap", res.Errors)
		}
	})

	t.Run("counts timeouts as errors", func(t *testing.T) {
		srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})

		cfg := testConfig(srv.URL, 3, 2)
		cfg.Timeout = 10 * time.Millisecond
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if res.Requests != 3 || res.Errors != 3 {
			t.Errorf("Result = %+v, want 3 requests and 3 errors", res)
		}
	})

	t.Run("runs for the configured duration", func(t *testing.T) {
		srv := newServer(t, func(http.ResponseWriter, *http.Request) {})

		cfg := testConfig(srv.URL, 1, 4)
		cfg.Duration = 100 * time.Millisecond
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if res.Elapsed < cfg.Duration || res.Elapsed > cfg.Duration+time.Second {
			t.Errorf("Result.Elapsed = %v, want about %v", res.Elapsed, cfg.Duration)
		}
		if res.Requests <= cfg.Requests {
			t.Errorf("Result.Requests = %d, want more than %d: Requests must be ignored", res.Requests, cfg.Requests)
		}
		if res.Errors != 0 {
			t.Errorf("Result.Errors = %d, want 0: requests cut off by the deadline must not count", res.Errors)
		}
	})

	t.Run("stops when ctx is cancelled", func(t *testing.T) {
		tests := []struct {
			name     string
			cancelAt int64 // cancel once the server has seen this many requests
			stall    bool  // hold every request until the client gives up
		}{
			{name: "between requests", cancelAt: 20},
			{name: "mid-request", cancelAt: 1, stall: true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				var hits atomic.Int64
				srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
					if hits.Add(1) == tt.cancelAt {
						cancel()
					}
					if tt.stall {
						<-r.Context().Done()
					}
				})

				cfg := testConfig(srv.URL, 10_000, 4)
				res, err := Run(ctx, cfg)
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				if res.Requests >= cfg.Requests {
					t.Errorf("Result.Requests = %d, want fewer than %d: run did not stop", res.Requests, cfg.Requests)
				}
				if tt.stall && res.Requests != 0 {
					t.Errorf("Result.Requests = %d, want 0: abandoned requests must not count", res.Requests)
				}
				if res.Errors != 0 {
					t.Errorf("Result.Errors = %d, want 0: cancellation is not a request failure", res.Errors)
				}
			})
		}
	})

	t.Run("leaves no goroutines behind", func(t *testing.T) {
		tests := []struct {
			name  string
			stall bool // every request is still in flight when the run ends
		}{
			{name: "idle connections"},
			{name: "abandoned requests", stall: true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
					if tt.stall {
						<-r.Context().Done()
					}
				})
				before := runtime.NumGoroutine()

				cfg := testConfig(srv.URL, 1, 8)
				cfg.Duration = 50 * time.Millisecond
				if _, err := Run(context.Background(), cfg); err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				waitForGoroutines(t, before)
			})
		}
	})
}

// waitForGoroutines fails t unless the goroutine count drops back to want.
// Connection goroutines exit asynchronously after their sockets close and
// nothing signals when they are gone, so this polls against a deadline.
func waitForGoroutines(t *testing.T, want int) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		got := runtime.NumGoroutine()
		if got <= want {
			return
		}
		select {
		case <-tick.C:
		case <-timeout:
			buf := make([]byte, 64<<10)
			t.Fatalf("%d goroutines running, want <= %d\n%s", got, want, buf[:runtime.Stack(buf, true)])
		}
	}
}

func newServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func testConfig(url string, requests, concurrency int) Config {
	return Config{
		URL:         url,
		Requests:    requests,
		Concurrency: concurrency,
		Timeout:     5 * time.Second,
	}
}
