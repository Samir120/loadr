package runner

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config describes one load-test run. The caller builds it and must call
// Validate before passing it to Run.
type Config struct {
	// URL is the target of every request.
	URL string

	// Method is the HTTP method of every request. Empty means GET, as in
	// http.Request.
	Method string

	// Header is sent with every request. A Host entry sets the request's
	// Host, which net/http would otherwise drop from the header.
	Header http.Header

	// Body, when non-empty, is sent with every request. Run never modifies
	// it.
	Body []byte

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
	if c.Method != "" && !isToken(c.Method) {
		return fmt.Errorf("method %q: not a valid token", c.Method)
	}
	for name, values := range c.Header {
		if !isToken(name) {
			return fmt.Errorf("header name %q: not a valid token", name)
		}
		for _, v := range values {
			if !isFieldValue(v) {
				return fmt.Errorf("header %s: value %q contains a control character", name, v)
			}
		}
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

// isToken reports whether s is an RFC 9110 token, the syntax shared by
// methods and header field names.
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		alnum := 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
		if !alnum && strings.IndexByte("!#$%&'*+-.^_`|~", c) < 0 {
			return false
		}
	}
	return true
}

// isFieldValue reports whether s is a valid header field value: no control
// characters other than horizontal tab, so "a\r\nX-Evil: 1" cannot smuggle
// in a second header.
func isFieldValue(s string) bool {
	for i := range len(s) {
		if c := s[i]; (c < ' ' && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}
