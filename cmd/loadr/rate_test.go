package main

import (
	"errors"
	"io"
	"testing"
)

func TestParseFlagsRate(t *testing.T) {
	const target = "http://example.com"

	tests := []struct {
		name     string
		args     []string
		wantRate int
		wantErr  error
	}{
		{name: "default is unlimited", args: []string{target}},
		{name: "explicit zero", args: []string{"-q", "0", target}},
		{name: "positive", args: []string{"-q", "50", target}, wantRate: 50},
		{name: "negative", args: []string{"-q", "-1", target}, wantErr: errUsage},
		{name: "not a number", args: []string{"-q", "fast", target}, wantErr: errUsage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseFlags(tt.args, io.Discard)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseFlags() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && opts.cfg.Rate != tt.wantRate {
				t.Errorf("Rate = %d, want %d", opts.cfg.Rate, tt.wantRate)
			}
		})
	}
}
