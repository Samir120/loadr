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

	// A server that is closed before use: every request fails to connect.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name       string
		ctx        context.Context
		args       []string
		wantErr    error
		wantStdout string
	}{
		{name: "text report", args: []string{"-n", "1", "-c", "1", srv.URL}, wantStdout: "Status codes\n  204  1\n"},
		{name: "json report", args: []string{"-o", "json", "-n", "3", "-c", "1", srv.URL}, wantStdout: `{"requests":3,"errors":0,`},
		{name: "duration", args: []string{"-o", "json", "-d", "50ms", "-c", "2", srv.URL}, wantStdout: `"errors":0,`},
		{name: "request errors still exit 0", args: []string{"-o", "json", "-n", "2", "-c", "1", deadURL}, wantStdout: `{"requests":2,"errors":2,`},
		{name: "interrupted", ctx: cancelled, args: []string{"-o", "json", "-n", "100", srv.URL}, wantErr: errInterrupted, wantStdout: `{"requests":0,"errors":0,`},
		{name: "help", args: []string{"-h"}, wantErr: flag.ErrHelp},
		{name: "missing url", args: []string{"-n", "1"}, wantErr: errUsage},
		{name: "two urls", args: []string{srv.URL, srv.URL}, wantErr: errUsage},
		{name: "unknown flag", args: []string{"-x", srv.URL}, wantErr: errUsage},
		{name: "invalid config", args: []string{"-c", "0", srv.URL}, wantErr: errUsage},
		{name: "bad url", args: []string{"example.com"}, wantErr: errUsage},
		{name: "bad format", args: []string{"-o", "xml", srv.URL}, wantErr: errUsage},
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
