package runner

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// Config describes one load-test run. The caller builds it and must call
// Validate before passing it to Run.
type Config struct {
	// URL is the target of every request.
	URL string

	// Requests is the total number of requests to send. Ignored when
	// Duration is set.
	Requests int

	// Duration, when non-zero, bounds the run by time instead of by count.
	Duration time.Duration

	// Concurrency is the number of workers sending requests in parallel.
	Concurrency int

	// Timeout bounds each individual request.
	Timeout time.Duration
}

// Validate reports the first problem with the configuration, or nil.
func (c Config) Validate() error {
	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("parsing url %q: %w", c.URL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url %q: scheme must be http or https", c.URL)
	}
	if u.Host == "" {
		return fmt.Errorf("url %q: missing host", c.URL)
	}
	if c.Concurrency <= 0 {
		return errors.New("concurrency must be > 0")
	}
	if c.Duration < 0 {
		return errors.New("duration must be >= 0")
	}
	if c.Duration == 0 && c.Requests <= 0 {
		return errors.New("requests must be > 0 when no duration is set")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be > 0")
	}
	return nil
}
