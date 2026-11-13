package stats

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCollectorSummary(t *testing.T) {
	errFailed := errors.New("failed")

	tests := []struct {
		name    string
		samples []Sample
		want    Summary
	}{
		{
			name: "no samples",
			want: Summary{},
		},
		{
			name:    "one sample",
			samples: []Sample{success(7, 200, 100)},
			want: Summary{
				Requests: 1, Bytes: 100,
				Min: ms(7), Mean: ms(7), Max: ms(7),
				P50: ms(7), P90: ms(7), P95: ms(7), P99: ms(7),
				Statuses: map[int]int{200: 1},
			},
		},
		{
			// Added out of order so the test fails if Summary doesn't sort.
			// n=10, rank = ceil(k·n/100): p50→5, p90→9, p95→10, p99→10.
			// Mean = 55ms / 10.
			name: "ten unsorted samples",
			samples: []Sample{
				success(7, 200, 10), success(3, 200, 10), success(10, 503, 10),
				success(1, 200, 10), success(9, 200, 10), success(2, 200, 10),
				success(8, 503, 10), success(5, 200, 10), success(4, 200, 10),
				success(6, 200, 10),
			},
			want: Summary{
				Requests: 10, Bytes: 100,
				Min: ms(1), Mean: 5500 * time.Microsecond, Max: ms(10),
				P50: ms(5), P90: ms(9), P95: ms(10), P99: ms(10),
				Statuses: map[int]int{200: 8, 503: 2},
			},
		},
		{
			// n=20 puts every percentile on a distinct rank: 10, 18, 19, 20.
			// Mean = 210ms / 20.
			name:    "twenty samples",
			samples: ascending(20),
			want: Summary{
				Requests: 20, Bytes: 20,
				Min: ms(1), Mean: 10500 * time.Microsecond, Max: ms(20),
				P50: ms(10), P90: ms(18), P95: ms(19), P99: ms(20),
				Statuses: map[int]int{200: 20},
			},
		},
		{
			// Failures count toward Requests and Errors only: the 10s timeout
			// must not reach Max, and the body-read failure's 200 and bytes
			// must not reach Statuses or Bytes.
			// Successes n=2: p50→rank 1, p90/p95/p99→rank 2.
			name: "failures excluded from distribution",
			samples: []Sample{
				success(2, 200, 50),
				{Latency: 10 * time.Second, Err: errFailed},
				{Latency: ms(30), Status: 200, Bytes: 999, Err: errFailed},
				success(4, 200, 50),
			},
			want: Summary{
				Requests: 4, Errors: 2, Bytes: 100,
				Min: ms(2), Mean: ms(3), Max: ms(4),
				P50: ms(2), P90: ms(4), P95: ms(4), P99: ms(4),
				Statuses: map[int]int{200: 2},
			},
		},
		{
			name: "only failures",
			samples: []Sample{
				{Latency: ms(5), Err: errFailed},
				{Latency: ms(6), Err: errFailed},
			},
			want: Summary{Requests: 2, Errors: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Collector
			for _, s := range tt.samples {
				c.Add(s)
			}
			if got := c.Summary(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Summary() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestSummaryDoesNotAliasCollector(t *testing.T) {
	var c Collector
	c.Add(success(5, 200, 0))

	first := c.Summary()
	first.Statuses[200] = 99

	// Adding after Summary has sorted in place must still work.
	c.Add(success(1, 404, 0))
	got := c.Summary()

	if want := map[int]int{200: 1, 404: 1}; !reflect.DeepEqual(got.Statuses, want) {
		t.Errorf("Statuses = %v, want %v", got.Statuses, want)
	}
	if got.Min != ms(1) {
		t.Errorf("Min = %v, want %v", got.Min, ms(1))
	}
}

// ms returns n milliseconds.
func ms(n int) time.Duration {
	return time.Duration(n) * time.Millisecond
}

// success returns a successful sample.
func success(latencyMS, status int, bytes int64) Sample {
	return Sample{Latency: ms(latencyMS), Status: status, Bytes: bytes}
}

// ascending returns n successful 200 samples of 1..n ms, one byte each.
func ascending(n int) []Sample {
	s := make([]Sample, n)
	for i := range n {
		s[i] = success(i+1, 200, 1)
	}
	return s
}
