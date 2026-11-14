package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartProgress(t *testing.T) {
	tests := []struct {
		name  string
		ticks int
		want  string
	}{
		{name: "stopped before any tick", want: "\r" + eraseLine},
		{name: "redraws on every tick", ticks: 2, want: "\r7 requests\r7 requests\r" + eraseLine},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				buf       bytes.Buffer
				completed atomic.Int64
			)
			completed.Store(7)
			tick := make(chan time.Time)

			stop := startProgress(&buf, &completed, tick)
			for range tt.ticks {
				tick <- time.Time{}
			}
			stop()

			if got := buf.String(); got != tt.want {
				t.Errorf("progress output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("creating file: %v", err)
	}
	t.Cleanup(func() { file.Close() })

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { devNull.Close() })

	tests := []struct {
		name string
		w    io.Writer
		want bool
	}{
		{name: "buffer", w: &bytes.Buffer{}},
		{name: "regular file", w: file},
		{name: "character device", w: devNull, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTerminal(tt.w); got != tt.want {
				t.Errorf("isTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunWritesNoProgressWhenStderrIsNotATerminal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)

	var stdout, stderr bytes.Buffer
	args := []string{"-n", "5", "-q", "1000", srv.URL}
	if err := run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if strings.Contains(stderr.String(), "\r") {
		t.Errorf("stderr = %q, want no progress line", stderr.String())
	}
}
