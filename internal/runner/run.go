package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Result summarises a completed run.
type Result struct {
	// Status is the HTTP status code of the response.
	Status int

	// Elapsed is the wall-clock time taken by the run.
	Elapsed time.Duration
}

// Run performs the load test described by cfg.
//
// This is the D1 placeholder: it sends exactly one request. The worker
// pool arrives on D2 and replaces this body.
func Run(ctx context.Context, cfg Config) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("building request: %w", err)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return Result{}, fmt.Errorf("reading body: %w", err)
	}

	return Result{Status: resp.StatusCode, Elapsed: time.Since(start)}, nil
}
