// Package report renders the result of a load test as text for people or
// as JSON for programs.
package report

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/Samir120/loadr/internal/stats"
)

// Text writes a human-readable report of s to w. elapsed is the run's
// wall-clock time.
//
// Sections with nothing to show are left out: Latency and Status codes when
// no request succeeded, Errors when none failed.
func Text(w io.Writer, s stats.Summary, elapsed time.Duration) error {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "Summary")
	fmt.Fprintf(tw, "  Requests\t%d\tDuration\t%.2fs\n", s.Requests, elapsed.Seconds())
	fmt.Fprintf(tw, "  Rate\t%.1f req/s\tBytes\t%s\n", rate(s.Requests, elapsed), formatBytes(s.Bytes))

	if len(s.Statuses) > 0 {
		fmt.Fprintln(tw)
		fmt.Fprintln(tw, "Latency")
		fmt.Fprintf(tw, "  min\t%.1fms\tmean\t%.1fms\tmax\t%.1fms\n",
			millis(s.Min), millis(s.Mean), millis(s.Max))
		fmt.Fprintf(tw, "  p50\t%.1fms\tp90\t%.1fms\tp95\t%.1fms\tp99\t%.1fms\n",
			millis(s.P50), millis(s.P90), millis(s.P95), millis(s.P99))

		fmt.Fprintln(tw)
		fmt.Fprintln(tw, "Status codes")
		for _, code := range slices.Sorted(maps.Keys(s.Statuses)) {
			fmt.Fprintf(tw, "  %d\t%d\n", code, s.Statuses[code])
		}
	}

	if s.Errors > 0 {
		fmt.Fprintln(tw)
		fmt.Fprintln(tw, "Errors")
		fmt.Fprintf(tw, "  total\t%d\n", s.Errors)
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("formatting text report: %w", err)
	}
	if _, err := buf.WriteTo(w); err != nil {
		return fmt.Errorf("writing text report: %w", err)
	}
	return nil
}

// rate returns requests per second over elapsed, or 0 if elapsed is not
// positive.
func rate(requests int, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(requests) / elapsed.Seconds()
}

// millis converts d to fractional milliseconds.
func millis(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// formatBytes renders n with a decimal (SI) unit, e.g. "1.2 MB".
func formatBytes(n int64) string {
	const prefixes = "kMGTPE"
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	v, i := float64(n)/1000, 0
	for v >= 999.95 && i < len(prefixes)-1 {
		v /= 1000
		i++
	}
	return fmt.Sprintf("%.1f %cB", v, prefixes[i])
}
