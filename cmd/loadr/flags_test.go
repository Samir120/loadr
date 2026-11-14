package main

import (
	"cmp"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseFlagsRequestShaping(t *testing.T) {
	const target = "http://example.com"

	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.json")
	if err := os.WriteFile(bodyFile, []byte(`{"from":"file"}`), 0o600); err != nil {
		t.Fatalf("writing body file: %v", err)
	}
	emptyFile := filepath.Join(dir, "empty")
	if err := os.WriteFile(emptyFile, nil, 0o600); err != nil {
		t.Fatalf("writing empty file: %v", err)
	}

	tests := []struct {
		name       string
		args       []string
		wantErr    error
		wantMethod string      // "" means GET
		wantHeader http.Header // compared in full; nil means none
		wantBody   string
	}{
		{name: "defaults", args: []string{target}},
		{name: "method", args: []string{"-m", "PUT", target}, wantMethod: "PUT"},
		{
			name:       "repeated headers are canonicalised and trimmed",
			args:       []string{"-H", "X-A: 1", "-H", "x-a:2", "-H", "Accept:  text/plain ", target},
			wantHeader: http.Header{"X-A": {"1", "2"}, "Accept": {"text/plain"}},
		},
		{
			name:       "inline body gets default content type",
			args:       []string{"-m", "POST", "-body", "hello", target},
			wantMethod: "POST",
			wantHeader: http.Header{"Content-Type": {defaultContentType}},
			wantBody:   "hello",
		},
		{
			name:       "explicit content type wins",
			args:       []string{"-body", "{}", "-H", "content-type: application/json", target},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantBody:   "{}",
		},
		{
			name:       "body from file",
			args:       []string{"-body", "@" + bodyFile, target},
			wantHeader: http.Header{"Content-Type": {defaultContentType}},
			wantBody:   `{"from":"file"}`,
		},
		{name: "empty body file sends no content type", args: []string{"-body", "@" + emptyFile, target}},
		{name: "header without colon", args: []string{"-H", "X-A", target}, wantErr: errUsage},
		{name: "header name with space", args: []string{"-H", "X A: 1", target}, wantErr: errUsage},
		{name: "invalid method", args: []string{"-m", "GE T", target}, wantErr: errUsage},
		{name: "missing body file", args: []string{"-body", "@" + filepath.Join(dir, "missing"), target}, wantErr: errUsage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseFlags(tt.args, io.Discard)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseFlags() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			cfg := opts.cfg
			if want := cmp.Or(tt.wantMethod, http.MethodGet); cfg.Method != want {
				t.Errorf("Method = %q, want %q", cfg.Method, want)
			}
			if !maps.EqualFunc(cfg.Header, tt.wantHeader, slices.Equal[[]string]) {
				t.Errorf("Header = %v, want %v", cfg.Header, tt.wantHeader)
			}
			if string(cfg.Body) != tt.wantBody {
				t.Errorf("Body = %q, want %q", cfg.Body, tt.wantBody)
			}
		})
	}
}
