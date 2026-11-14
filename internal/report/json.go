package report

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Samir120/loadr/internal/stats"
)

// jsonReport is the JSON shape of a report. Its field names are part of
// loadr's output contract: add fields, never rename or remove them.
type jsonReport struct {
	Requests    int         `json:"requests"`
	Errors      int         `json:"errors"`
	DurationMs  float64     `json:"duration_ms"`
	RateRPS     float64     `json:"rate_rps"`
	Bytes       int64       `json:"bytes"`
	LatencyMs   jsonLatency `json:"latency_ms"`
	StatusCodes map[int]int `json:"status_codes"`
}

// jsonLatency holds latency statistics in fractional milliseconds.
type jsonLatency struct {
	Min  float64 `json:"min"`
	Mean float64 `json:"mean"`
	Max  float64 `json:"max"`
	P50  float64 `json:"p50"`
	P90  float64 `json:"p90"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
}

// JSON writes s to w as a single-line JSON object followed by a newline.
// elapsed is the run's wall-clock time.
//
// Durations are fractional milliseconds. Every field is always present:
// when no request succeeded the latency fields are 0 and status_codes is an
// empty object, never null.
func JSON(w io.Writer, s stats.Summary, elapsed time.Duration) error {
	statuses := s.Statuses
	if statuses == nil {
		statuses = map[int]int{}
	}

	r := jsonReport{
		Requests:   s.Requests,
		Errors:     s.Errors,
		DurationMs: millis(elapsed),
		RateRPS:    rate(s.Requests, elapsed),
		Bytes:      s.Bytes,
		LatencyMs: jsonLatency{
			Min:  millis(s.Min),
			Mean: millis(s.Mean),
			Max:  millis(s.Max),
			P50:  millis(s.P50),
			P90:  millis(s.P90),
			P95:  millis(s.P95),
			P99:  millis(s.P99),
		},
		StatusCodes: statuses,
	}

	if err := json.NewEncoder(w).Encode(r); err != nil {
		return fmt.Errorf("writing JSON report: %w", err)
	}
	return nil
}
