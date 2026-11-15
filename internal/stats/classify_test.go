package stats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"socket EMFILE", sendErr(dialErr(os.NewSyscallError("socket", syscall.EMFILE))), classFDLimit},
		{"socket ENFILE", sendErr(dialErr(os.NewSyscallError("socket", syscall.ENFILE))), classFDLimit},
		{"request deadline", sendErr(context.DeadlineExceeded), classTimeout},
		{"dial deadline", sendErr(dialErr(os.ErrDeadlineExceeded)), classTimeout},
		{"body read deadline", fmt.Errorf("reading body: %w", context.DeadlineExceeded), classTimeout},
		{"dns timeout", sendErr(dialErr(&net.DNSError{Err: "i/o timeout", Name: "slow.test", IsTimeout: true})), classTimeout},
		{"dns not found", sendErr(dialErr(&net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true})), classDNS},
		{"connection refused", sendErr(dialErr(os.NewSyscallError("connect", syscall.ECONNREFUSED))), classRefused},
		{"connection reset", sendErr(&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}), classClosed},
		{"broken pipe", sendErr(&net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.EPIPE)}), classClosed},
		{"EOF before response", sendErr(io.EOF), classClosed},
		{"body cut short", fmt.Errorf("reading body: %w", io.ErrUnexpectedEOF), classClosed},
		{"cancelled", sendErr(context.Canceled), classOther},
		{"unrecognised", fmt.Errorf("building request: %w", errors.New("net/http: invalid method")), classOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.err); got != tt.want {
				t.Errorf("classify(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// sendErr wraps err the way runner.send reports a failed client.Do.
func sendErr(err error) error {
	return fmt.Errorf("sending request: %w", &url.Error{Op: "Get", URL: "http://loadr.test", Err: err})
}

// dialErr wraps err the way net reports a failed dial.
func dialErr(err error) error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: err}
}
