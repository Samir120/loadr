package runner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	t.Run("returns the status code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		defer srv.Close()

		res, err := Run(context.Background(), Config{
			URL:         srv.URL,
			Requests:    1,
			Concurrency: 1,
			Timeout:     time.Second,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if res.Status != http.StatusTeapot {
			t.Errorf("Status = %d, want %d", res.Status, http.StatusTeapot)
		}
	})

	t.Run("times out", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		_, err := Run(context.Background(), Config{
			URL:         srv.URL,
			Requests:    1,
			Concurrency: 1,
			Timeout:     10 * time.Millisecond,
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
		}
	})
}
