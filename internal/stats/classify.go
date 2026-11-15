package stats

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
)

// Error classes, as they appear in Summary.ErrorClasses. The names are part
// of loadr's output contract: add classes, never rename them.
const (
	classFDLimit = "fd_limit" // the client ran out of file descriptors
	classTimeout = "timeout"  // the per-request timeout expired
	classDNS     = "dns"      // the host name did not resolve
	classRefused = "refused"  // nothing accepted the connection
	classClosed  = "closed"   // the server closed or reset the connection
	classOther   = "other"    // anything else
)

// classify maps a failed request's error to an error class. The checks run
// in order and the first match wins: running out of file descriptors is
// reported as such even though it surfaces as a dial error, and a DNS lookup
// that timed out counts as a timeout.
func classify(err error) string {
	var (
		netErr net.Error
		dnsErr *net.DNSError
	)
	switch {
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return classFDLimit
	case errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		return classTimeout
	case errors.As(err, &dnsErr):
		return classDNS
	case errors.Is(err, syscall.ECONNREFUSED):
		return classRefused
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return classClosed
	default:
		return classOther
	}
}
