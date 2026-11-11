package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
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
