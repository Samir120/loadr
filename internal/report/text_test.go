package report

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Samir120/loadr/internal/stats"
)

func TestText(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") != ""

	tests := []struct {
		name    string
		summary stats.Summary
		elapsed time.Duration
	}{
		{
			name: "full",
			summary: stats.Summary{
				Requests: 10000,
				Errors:   8,
				Bytes:    1_234_567,
				Min:      812 * time.Microsecond,
				Mean:     41_200 * time.Microsecond,
				Max:      312_600 * time.Microsecond,
				P50:      38_100 * time.Microsecond,
				P90:      67_400 * time.Microsecond,
				P95:      84 * time.Millisecond,
				P99:      142_700 * time.Microsecond,
				Statuses: map[int]int{503: 12, 200: 9980},
			},
			elapsed: 4210 * time.Millisecond,
		},
		{
			name:    "all_failed",
			summary: stats.Summary{Requests: 5, Errors: 5},
			elapsed: time.Second,
		},
		{
			name: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Text(&buf, tt.summary, tt.elapsed); err != nil {
				t.Fatalf("Text() error = %v", err)
			}

			path := filepath.Join("testdata", tt.name+".golden")
			if update {
				writeGolden(t, path, buf.Bytes())
			}
			want := readGolden(t, path)
			if got := buf.String(); got != want {
				t.Errorf("Text() output differs from %s\ngot:\n%s\nwant:\n%s", path, got, want)
			}
		})
	}
}

func TestTextWriteError(t *testing.T) {
	want := errors.New("disk full")
	err := Text(failingWriter{err: want}, stats.Summary{}, 0)
	if !errors.Is(err, want) {
		t.Fatalf("Text() error = %v, want it to wrap %v", err, want)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1000, "1.0 kB"},
		{999_949, "999.9 kB"},
		{999_999, "1.0 MB"},
		{1_234_567, "1.2 MB"},
		{math.MaxInt64, "9.2 EB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatBytes(tt.n); got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// failingWriter is an io.Writer whose every Write fails with err.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func writeGolden(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating golden dir: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("writing golden file: %v", err)
	}
}

func readGolden(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file (run with UPDATE_GOLDEN=1 to create it): %v", err)
	}
	return string(b)
}
