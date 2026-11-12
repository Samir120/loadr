package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name       string
		ctx        context.Context
		args       []string
		wantErr    error
		wantStdout string
	}{
		{name: "happy path", args: []string{"-n", "1", "-c", "1", srv.URL}, wantStdout: "requests 1  errors 0"},
		{name: "duration", args: []string{"-d", "50ms", "-c", "2", srv.URL}, wantStdout: "errors 0"},
		{name: "interrupted", ctx: cancelled, args: []string{"-n", "100", srv.URL}, wantErr: errInterrupted, wantStdout: "requests 0  errors 0"},
		{name: "help", args: []string{"-h"}, wantErr: flag.ErrHelp},
		{name: "missing url", args: []string{"-n", "1"}, wantErr: errUsage},
		{name: "two urls", args: []string{srv.URL, srv.URL}, wantErr: errUsage},
		{name: "unknown flag", args: []string{"-x", srv.URL}, wantErr: errUsage},
		{name: "invalid config", args: []string{"-c", "0", srv.URL}, wantErr: errUsage},
		{name: "bad url", args: []string{"example.com"}, wantErr: errUsage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			var stdout, stderr bytes.Buffer
			err := run(ctx, tt.args, &stdout, &stderr)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("run() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}
