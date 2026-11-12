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

	"github.com/Samir120/loadr/internal/runner"
)

var (
	// errUsage marks errors caused by invalid flags or arguments. main maps
	// it to exit code 2.
	errUsage = errors.New("usage error")

	// errInterrupted reports that the run was cut short by Ctrl-C after the
	// partial result was printed. main maps it to exit code 130.
	errInterrupted = errors.New("interrupted")
)

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

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	res, err := runner.Run(ctx, cfg)
	if err != nil {
		return err
	}
	interrupted := ctx.Err() != nil

	if _, err := fmt.Fprintf(stdout, "requests %d  errors %d  elapsed %s\n", res.Requests, res.Errors, res.Elapsed); err != nil {
		return err
	}
	if interrupted {
		return errInterrupted
	}
	return nil
}

func parseFlags(args []string, stderr io.Writer) (runner.Config, error) {
	fs := flag.NewFlagSet("loadr", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: loadr [flags] URL")
		fs.PrintDefaults()
	}

	var cfg runner.Config
	fs.IntVar(&cfg.Requests, "n", 200, "total requests; ignored when -d is set")
	fs.DurationVar(&cfg.Duration, "d", 0, "run for this long instead of a fixed count")
	fs.IntVar(&cfg.Concurrency, "c", 10, "concurrent workers")
	fs.DurationVar(&cfg.Timeout, "t", 10*time.Second, "per-request timeout")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return runner.Config{}, err
		}
		return runner.Config{}, fmt.Errorf("%w: %v", errUsage, err)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return runner.Config{}, fmt.Errorf("%w: expected one URL, got %d", errUsage, fs.NArg())
	}
	cfg.URL = fs.Arg(0)

	if err := cfg.Validate(); err != nil {
		return runner.Config{}, fmt.Errorf("%w: %v", errUsage, err)
	}
	return cfg, nil
}
