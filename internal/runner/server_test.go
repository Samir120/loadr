package runner

import (
	"context"
	"maps"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// behaviour is how a test server answers the n-th request it receives,
// counting from 1 in arrival order.
type behaviour func(t *testing.T, w http.ResponseWriter, r *http.Request, n int64)

func TestRunServerBehaviours(t *testing.T) {
	const latency = 20 * time.Millisecond

	tests := []struct {
		name           string
		serve          behaviour
		requests       int
		concurrency    int
		timeout        time.Duration // 0 keeps testConfig's default
		wantErrors     int
		wantStatuses   map[int]int
		wantBytes      int64
		wantMinLatency time.Duration // lower bound on Result.Min
	}{
		{
			name:           "fixed latency",
			serve:          delay(latency),
			requests:       20,
			concurrency:    4,
			wantStatuses:   map[int]int{http.StatusOK: 20},
			wantMinLatency: latency,
		},
		{
			name:         "every fifth request fails with 500",
			serve:        failEvery(5),
			requests:     100,
			concurrency:  10,
			wantStatuses: map[int]int{http.StatusOK: 80, http.StatusInternalServerError: 20},
		},
		{
			name:        "connection reset",
			serve:       resetConn,
			requests:    10,
			concurrency: 2,
			wantErrors:  10,
		},
		{
			name:           "slow body within timeout",
			serve:          slowBody(5, 10*time.Millisecond),
			requests:       8,
			concurrency:    4,
			wantStatuses:   map[int]int{http.StatusOK: 8},
			wantBytes:      40,
			wantMinLatency: 50 * time.Millisecond,
		},
		{
			name:        "slow body past timeout",
			serve:       slowBody(5, 50*time.Millisecond),
			requests:    4,
			concurrency: 2,
			timeout:     100 * time.Millisecond,
			wantErrors:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int64
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				tt.serve(t, w, r, hits.Add(1))
			})

			cfg := testConfig(srv.URL, tt.requests, tt.concurrency)
			if tt.timeout > 0 {
				cfg.Timeout = tt.timeout
			}
			res, err := Run(context.Background(), cfg)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if got := hits.Load(); got != int64(tt.requests) {
				t.Errorf("server saw %d requests, want %d", got, tt.requests)
			}
			if res.Requests != tt.requests || res.Errors != tt.wantErrors {
				t.Errorf("Result has %d requests and %d errors, want %d and %d",
					res.Requests, res.Errors, tt.requests, tt.wantErrors)
			}
			if !maps.Equal(res.Statuses, tt.wantStatuses) {
				t.Errorf("Result.Statuses = %v, want %v", res.Statuses, tt.wantStatuses)
			}
			if res.Bytes != tt.wantBytes {
				t.Errorf("Result.Bytes = %d, want %d", res.Bytes, tt.wantBytes)
			}
			if res.Min < tt.wantMinLatency {
				t.Errorf("Result.Min = %v, want at least %v: latency must cover the whole response",
					res.Min, tt.wantMinLatency)
			}
		})
	}

	t.Run("duration mode with fixed latency", func(t *testing.T) {
		srv := newServer(t, func(http.ResponseWriter, *http.Request) {
			time.Sleep(latency)
		})

		cfg := testConfig(srv.URL, 1, 4)
		cfg.Duration = 200 * time.Millisecond
		res, err := Run(context.Background(), cfg)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		// No worker can finish more than Duration/latency requests in a row.
		maxRequests := cfg.Concurrency * int(cfg.Duration/latency)
		if res.Requests == 0 || res.Requests > maxRequests {
			t.Errorf("Result.Requests = %d, want between 1 and %d", res.Requests, maxRequests)
		}
		if res.Errors != 0 {
			t.Errorf("Result.Errors = %d, want 0: requests in flight at the deadline must be dropped", res.Errors)
		}
		if res.Elapsed < cfg.Duration || res.Elapsed > cfg.Duration+time.Second {
			t.Errorf("Result.Elapsed = %v, want about %v", res.Elapsed, cfg.Duration)
		}
	})
}

// delay answers every request with an empty 200 after d.
func delay(d time.Duration) behaviour {
	return func(*testing.T, http.ResponseWriter, *http.Request, int64) {
		time.Sleep(d)
	}
}

// failEvery answers every k-th request with a 500 and the rest with a 200.
// Keying on arrival order makes the split exact under any interleaving.
func failEvery(k int64) behaviour {
	return func(_ *testing.T, w http.ResponseWriter, _ *http.Request, n int64) {
		if n%k == 0 {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// resetConn drops the connection without a response. A zero linger makes
// Close send a TCP RST instead of a FIN, so the client sees a reset rather
// than a clean EOF.
func resetConn(t *testing.T, w http.ResponseWriter, _ *http.Request, _ int64) {
	conn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		t.Errorf("hijacking connection: %v", err)
		return
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		if err := tc.SetLinger(0); err != nil {
			t.Errorf("setting linger: %v", err)
		}
	}
	if err := conn.Close(); err != nil {
		t.Errorf("closing connection: %v", err)
	}
}

// slowBody sends the headers at once, then a body of chunks bytes, one byte
// per gap, so the response takes at least chunks·gap to finish.
func slowBody(chunks int, gap time.Duration) behaviour {
	return func(_ *testing.T, w http.ResponseWriter, r *http.Request, _ int64) {
		rc := http.NewResponseController(w)
		for range chunks {
			if _, err := w.Write([]byte("x")); err != nil {
				return // client gave up
			}
			if err := rc.Flush(); err != nil {
				return
			}
			select {
			case <-time.After(gap):
			case <-r.Context().Done():
				return
			}
		}
	}
}
