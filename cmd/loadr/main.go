// Command loadr is a small HTTP load tester.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/Samir120/loadr/internal/report"
	"github.com/Samir120/loadr/internal/runner"
	"github.com/Samir120/loadr/internal/stats"
)

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
	)
	fs.IntVar(&cfg.Requests, "n", 200, "total requests; ignored when -d is set")
	fs.DurationVar(&cfg.Duration, "d", 0, "run for this long instead of a fixed count")
	fs.IntVar(&cfg.Concurrency, "c", 10, "concurrent workers")
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
