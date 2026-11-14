// Command loadr is a small HTTP load tester.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Samir120/loadr/internal/report"
	"github.com/Samir120/loadr/internal/runner"
	"github.com/Samir120/loadr/internal/stats"
)

// defaultContentType labels a -body that no -H labels. RFC 9110 lets a
// server assume it for an unlabelled body, so it claims nothing about the
// bytes.
const defaultContentType = "application/octet-stream"

var (
	// errUsage marks errors caused by invalid flags or arguments. main maps
	// it to exit code 2.
	errUsage = errors.New("usage error")

	// errInterrupted reports that the run was cut short by Ctrl-C after the
	// partial result was printed. main maps it to exit code 130.
	errInterrupted = errors.New("interrupted")
)

// options is everything parsed from the command line.
type options struct {
	cfg runner.Config

	// render writes the report in the format chosen with -o.
	render func(w io.Writer, s stats.Summary, elapsed time.Duration) error
}

// headerFlag collects repeated -H "Key: Value" flags into an http.Header.
type headerFlag http.Header

// String implements flag.Value. flag also calls it on a zero headerFlag to
// decide whether to print a default, so it must cope with a nil map.
func (h headerFlag) String() string {
	return fmt.Sprint(http.Header(h))
}

// Set implements flag.Value. It only splits "Key: Value";
// runner.Config.Validate decides whether the name and value are legal.
func (h headerFlag) Set(s string) error {
	name, value, ok := strings.Cut(s, ":")
	if !ok {
		return errors.New(`want "Key: Value"`)
	}
	http.Header(h).Add(name, strings.TrimSpace(value))
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()

	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return
	case errors.Is(err, errInterrupted):
		fmt.Fprintln(os.Stderr, "loadr:", err)
		os.Exit(130)
	case errors.Is(err, errUsage):
		fmt.Fprintln(os.Stderr, "loadr:", err)
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "loadr:", err)
		os.Exit(1)
	}
}

// run parses args, performs the load test and writes the report to stdout.
//
// Failed requests are part of the report, not an error: a run that
// completes returns nil however many of its requests failed.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	res, err := runner.Run(ctx, opts.cfg)
	if err != nil {
		return err
	}
	interrupted := ctx.Err() != nil

	if err := opts.render(stdout, res.Summary, res.Elapsed); err != nil {
		return err
	}
	if interrupted {
		return errInterrupted
	}
	return nil
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	fs := flag.NewFlagSet("loadr", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: loadr [flags] URL")
		fs.PrintDefaults()
	}

	var (
		cfg    runner.Config
		format string
		body   string
	)
	cfg.Header = make(http.Header)
	fs.IntVar(&cfg.Requests, "n", 200, "total requests; ignored when -d is set")
	fs.DurationVar(&cfg.Duration, "d", 0, "run for this long instead of a fixed count")
	fs.IntVar(&cfg.Concurrency, "c", 10, "concurrent workers")
	fs.StringVar(&cfg.Method, "m", http.MethodGet, "HTTP method")
	fs.Var(headerFlag(cfg.Header), "H", `request header "Key: Value"; repeatable`)
	fs.StringVar(&body, "body", "", `request body; "@path" reads it from a file`)
	fs.DurationVar(&cfg.Timeout, "t", 10*time.Second, "per-request timeout")
	fs.StringVar(&format, "o", "text", "output format: text | json")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, err
		}
		return options{}, fmt.Errorf("%w: %v", errUsage, err)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return options{}, fmt.Errorf("%w: expected one URL, got %d", errUsage, fs.NArg())
	}
	cfg.URL = fs.Arg(0)

	if err := cfg.Validate(); err != nil {
		return options{}, fmt.Errorf("%w: %v", errUsage, err)
	}

	b, err := readBody(body)
	if err != nil {
		return options{}, fmt.Errorf("%w: %v", errUsage, err)
	}
	cfg.Body = b
	if len(cfg.Body) > 0 && cfg.Header.Get("Content-Type") == "" {
		cfg.Header.Set("Content-Type", defaultContentType)
	}

	opts := options{cfg: cfg}
	switch format {
	case "text":
		opts.render = report.Text
	case "json":
		opts.render = report.JSON
	default:
		return options{}, fmt.Errorf("%w: output format %q: must be text or json", errUsage, format)
	}
	return opts, nil
}

// readBody resolves the -body argument: "@path" is the contents of the file
// at path, anything else is the body itself.
func readBody(arg string) ([]byte, error) {
	path, ok := strings.CutPrefix(arg, "@")
	if !ok {
		return []byte(arg), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return b, nil
}
