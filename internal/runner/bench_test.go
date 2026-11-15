package runner

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkRun measures the cost of one request through Run against a local
// server. It exists to profile the hot path:
//
//	go test -run='^$' -bench='Run/c=50' -benchtime=5s -cpuprofile cpu.prof ./internal/runner
//
// The server runs in the same process, so its work is part of the profile.
func BenchmarkRun(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(body)
	}))
	b.Cleanup(srv.Close)

	tests := []struct {
		name        string
		concurrency int
	}{
		{name: "c=1", concurrency: 1},
		{name: "c=10", concurrency: 10},
		{name: "c=50", concurrency: 50},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			cfg := testConfig(srv.URL, b.N, tt.concurrency)
			b.ReportAllocs()
			b.ResetTimer()
			res, err := Run(context.Background(), cfg)
			b.StopTimer()
			if err != nil {
				b.Fatalf("Run() error = %v", err)
			}
			if res.Requests != b.N || res.Errors != 0 {
				b.Fatalf("Result = %d requests, %d errors; want %d, 0", res.Requests, res.Errors, b.N)
			}
		})
	}
}
