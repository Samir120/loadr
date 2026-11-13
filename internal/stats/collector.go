package stats

import (
	"maps"
	"slices"
	"time"
)

// Collector accumulates samples into summary statistics. The zero value is
// ready to use. A Collector is not safe for concurrent use; feed it from a
// single goroutine.
type Collector struct {
	latencies []time.Duration
	total     time.Duration
	statuses  map[int]int
	errors    int
	bytes     int64
}

// Summary is the aggregate of every sample added to a Collector.
//
// Failed samples count toward Requests and Errors only. Latency fields, Bytes
// and Statuses describe successful samples; the latency fields are zero and
// Statuses is nil when there were none.
type Summary struct {
	Requests int
	Errors   int
	Bytes    int64

	Min, Mean, Max     time.Duration
	P50, P90, P95, P99 time.Duration

	// Statuses maps HTTP status code to the number of responses with it.
	Statuses map[int]int
}

// Add records one sample.
func (c *Collector) Add(s Sample) {
	if s.Err != nil {
		c.errors++
		return
	}
	c.latencies = append(c.latencies, s.Latency)
	c.total += s.Latency
	c.bytes += s.Bytes
	if c.statuses == nil {
		c.statuses = make(map[int]int)
	}
	c.statuses[s.Status]++
}

// Summary computes statistics over every sample added so far. It sorts the
// collector's latencies in place, so calling it again is cheap and further
// calls to Add remain valid.
func (c *Collector) Summary() Summary {
	sum := Summary{
		Requests: len(c.latencies) + c.errors,
		Errors:   c.errors,
		Bytes:    c.bytes,
		Statuses: maps.Clone(c.statuses),
	}
	n := len(c.latencies)
	if n == 0 {
		return sum
	}

	slices.Sort(c.latencies)
	sum.Min = c.latencies[0]
	sum.Max = c.latencies[n-1]
	sum.Mean = c.total / time.Duration(n)
	sum.P50 = percentile(c.latencies, 50)
	sum.P90 = percentile(c.latencies, 90)
	sum.P95 = percentile(c.latencies, 95)
	sum.P99 = percentile(c.latencies, 99)
	return sum
}

// percentile returns the k-th percentile of sorted using the nearest-rank
// method: the element at 1-based rank ceil(k/100 · n). sorted must be
// non-empty and in ascending order, and k must be in [1, 100].
func percentile(sorted []time.Duration, k int) time.Duration {
	rank := (k*len(sorted) + 99) / 100
	return sorted[rank-1]
}
