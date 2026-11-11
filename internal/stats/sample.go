// Package stats turns per-request samples into summary statistics.
package stats

import "time"

// Sample is the outcome of a single request.
type Sample struct {
	// Latency is the time from sending the request to reading the last
	// byte of the body. Zero if the request could not be built.
	Latency time.Duration

	// Status is the HTTP status code, or 0 if no response arrived.
	Status int

	// Bytes is the number of body bytes read.
	Bytes int64

	// Err is non-nil if the request failed at any stage.
	Err error
}
