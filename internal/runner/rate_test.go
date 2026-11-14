package runner

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigValidateRate(t *testing.T) {
	tests := []struct {
		name    string
		rate    int
		wantErr bool
	}{
		{name: "zero means unlimited", rate: 0},
		{name: "one per second", rate: 1},
		{name: "one per nanosecond", rate: maxRate},
		{name: "negative", rate: -1, wantErr: true},
		{name: "interval would be zero", rate: maxRate + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig("http://example.com", 10, 2)
			cfg.Rate = tt.rate
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunRate(t *testing.T) {
	t.Run("takes at least n/rate", func(t *testing.T) {
		tests := []struct {
			name        string
			requests    int
			concurrency int
			rate        int
		}{
			{name: "one worker", requests: 10, concurrency: 1, rate: 50},
			{name: "workers share one limit", requests: 10, concurrency: 5, rate: 50},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var hits atomic.Int64
				srv := newServer(t, func(http.ResponseWriter, *http.Request) {
					hits.Add(1)
				})

				cfg := testConfig(srv.URL, tt.requests, tt.concurrency)
				cfg.Rate = tt.rate
				res, err := Run(context.Background(), cfg)
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				want := time.Duration(tt.requests) * time.Second / time.Duration(tt.rate)
				if res.Elapsed < want || res.Elapsed > want+time.Second {
					t.Errorf("Result.Elapsed = %v, want at least %v and under %v", res.Elapsed, want, want+time.Second)
				}
				if got := hits.Load(); got != int64(tt.requests) {
					t.Errorf("server saw %d requests, want %d", got, tt.requests)
				}
				if res.Requests != tt.requests || res.Errors != 0 {
					t.Errorf("Result = %d requests, %d errors; want %d, 0", res.Requests, res.Errors, tt.requests)
				}
			})
		}
	})

	t.Run("does not wait for a tick once the run ends", func(t *testing.T) {
		tests := []struct {
			name        string
			duration    time.Duration // the run's own -d
			cancelAfter time.Duration // deadline on the caller's ctx, standing in for Ctrl-C
		}{
			{name: "duration elapses", duration: 100 * time.Millisecond},
			{name: "ctx cancelled", cancelAfter: 100 * time.Millisecond},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var hits atomic.Int64
				srv := newServer(t, func(http.ResponseWriter, *http.Request) {
					hits.Add(1)
				})

				ctx := context.Background()
				if tt.cancelAfter > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tt.cancelAfter)
					defer cancel()
				}

				cfg := testConfig(srv.URL, 5, 2)
				cfg.Duration = tt.duration
				cfg.Rate = 1 // first tick after 1 s, long after the run has ended
				res, err := Run(ctx, cfg)
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				if res.Elapsed >= 500*time.Millisecond {
					t.Errorf("Result.Elapsed = %v, want well under the 1s tick: workers waited past the end of the run", res.Elapsed)
				}
				if got := hits.Load(); got != 0 || res.Requests != 0 {
					t.Errorf("server saw %d requests, Result.Requests = %d; want 0, 0", got, res.Requests)
				}
			})
		}
	})
}
