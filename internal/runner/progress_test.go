package runner

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunCompleted(t *testing.T) {
	tests := []struct {
		name  string
		stall bool // every request times out
	}{
		{name: "successes"},
		{name: "failures count too", stall: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
				if tt.stall {
					<-r.Context().Done()
				}
			})

			cfg := testConfig(srv.URL, 20, 4)
			if tt.stall {
				cfg.Timeout = 10 * time.Millisecond
			}
			var completed atomic.Int64
			cfg.Completed = &completed

			res, err := Run(context.Background(), cfg)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := completed.Load(); got != int64(res.Requests) || got != int64(cfg.Requests) {
				t.Errorf("Completed = %d, Result.Requests = %d; want both %d", got, res.Requests, cfg.Requests)
			}
		})
	}
}
