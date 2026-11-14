package report

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/Samir120/loadr/internal/stats"
)

func TestJSON(t *testing.T) {
	tests := []struct {
		name    string
		summary stats.Summary
		elapsed time.Duration
		want    string
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
			elapsed: 4 * time.Second,
			want: `{"requests":10000,"errors":8,"duration_ms":4000,"rate_rps":2500,"bytes":1234567,` +
				`"latency_ms":{"min":0.812,"mean":41.2,"max":312.6,"p50":38.1,"p90":67.4,"p95":84,"p99":142.7},` +
				`"status_codes":{"200":9980,"503":12}}` + "\n",
		},
		{
			name:    "all_failed",
			summary: stats.Summary{Requests: 5, Errors: 5},
			elapsed: time.Second,
			want: `{"requests":5,"errors":5,"duration_ms":1000,"rate_rps":5,"bytes":0,` +
				`"latency_ms":{"min":0,"mean":0,"max":0,"p50":0,"p90":0,"p95":0,"p99":0},` +
				`"status_codes":{}}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := JSON(&buf, tt.summary, tt.elapsed); err != nil {
				t.Fatalf("JSON() error = %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("JSON() output differs\ngot:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestJSONWriteError(t *testing.T) {
	want := errors.New("disk full")
	err := JSON(failingWriter{err: want}, stats.Summary{}, 0)
	if !errors.Is(err, want) {
		t.Fatalf("JSON() error = %v, want it to wrap %v", err, want)
	}
}
