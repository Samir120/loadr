# loadr — Design

A small HTTP load tester in Go. Fires N requests (or runs for a duration) at one URL with fixed concurrency and reports latency percentiles, throughput, status and error counts. Standard library only, single static binary.

This document is the v1 contract. If a feature isn't listed under "v1 scope", it isn't in v1.

## v1 scope

### CLI

```
loadr [flags] URL

  -n int        total requests (default 200). Ignored when -d is set.
  -d duration   run for this long instead of a fixed count, e.g. 30s, 2m.
  -c int        concurrent workers (default 10).
  -q int        max requests per second across all workers; 0 = unlimited.
  -m string     HTTP method (default GET).
  -H value      request header "Key: Value"; repeatable.
  -body string  request body; "@path" reads the body from a file.
  -t duration   per-request timeout (default 10s).
  -o string     output format: text | json (default text).
```

Validation errors go to stderr with exit code 2. Ctrl-C ends the run early and prints what was collected so far.

### Output

```
Summary
  Requests      10000        Duration    4.21s
  Rate          2374.3 req/s Bytes       1.2 MB

Latency
  min  0.8ms   mean  41.2ms   max  312.6ms
  p50  38.1ms  p90   67.4ms   p95  84.0ms   p99  142.7ms

Status codes
  200  9980
  503  12

Errors
  timeout  8
```

`-o json` emits the same data as a single JSON object with stable field names.

### Non-goals for v1

HTTP/2 configuration, TLS options, multiple targets, request scripting, live UI, distributed workers, histogram export. These are a parking lot, not a roadmap.

## Architecture

```
cmd/loadr/main.go        flags → runner.Config → runner.Run → report.Write
internal/runner          worker pool, collector, cancellation
internal/stats           latency aggregation, percentiles, histograms
internal/report          text and JSON renderers
```

Dependency direction is strictly downward: `cmd` → `runner` → `stats`; `cmd` → `report` → `stats`. `internal/` packages do not import `flag`, do not print, do not exit.

### Data flow

1. `main` parses flags into `runner.Config` and builds a `context.Context` from `signal.NotifyContext`. The signal is the only thing that cancels it.
2. `runner.Run(ctx, cfg)` wraps `ctx` in `context.WithTimeout` when `-d` is set, then starts `cfg.Concurrency` workers and one collector.
3. Each worker loops while `ctx` is not done: claim a request slot from a shared atomic counter (limit `-n`, unbounded under `-d`), optionally wait on the rate-limit ticker, build a fresh `*http.Request` with `context.WithTimeout(ctx, cfg.Timeout)`, send it through the shared `*http.Client`, drain the body to `io.Discard`, close it, send a `stats.Sample{Latency, Status, Bytes, Err}` on the results channel. A sample that failed after the run's `ctx` was done is dropped: the request was abandoned, not measured.
4. The collector feeds every sample to a `stats.Collector`. After the `WaitGroup` is done the channel is closed and drained, and the collector returns `Collector.Summary()`.
5. `Run` returns a `runner.Result`: the `stats.Summary` (embedded) plus wall-clock `Elapsed`. `report` renders it.

### Key decisions

- **One `http.Client`, tuned transport.** `MaxIdleConnsPerHost` and `MaxConnsPerHost` set to the concurrency. Clients are safe for concurrent use; the default transport's idle-connection limit would silently serialise work.
- **Fresh request per call.** `*http.Request` is not safe to reuse across goroutines. Bodies are wrapped in `bytes.NewReader` each time.
- **Timeout via context, not `Client.Timeout`.** One cancellation mechanism that composes with Ctrl-C and `-d`.
- **Channel to a single collector.** The collector is the only goroutine that touches the sample slice. No locks on the hot path, no data races by construction.
- **Nearest-rank percentiles on a sorted slice.** `p(k) = sorted[ceil(k/100 · n) − 1]`. Sorting once at the end is cheap at v1 sizes and easy to test by hand.
- **Latency is time to last byte.** The body is read fully. This matches `hey` and keeps keep-alive connections reusable.
- **Rate limiting with one ticker.** Workers block on a shared `<-ticker.C` before each request when `-q > 0`. Imprecise under heavy contention, documented as such.

### Exit codes

| Code | Meaning                                   |
|-----:|-------------------------------------------|
| 0    | run completed                             |
| 1    | run failed to start or runtime error      |
| 2    | invalid flags or URL                      |
| 130  | interrupted by Ctrl-C (partial report printed) |

Whether a completed run with request errors returns 0 is open (STATE.md Q1).

## Quality gates

- `make check` = `gofmt -l`, `go vet ./...`, `staticcheck ./...`, `go test -race ./...`. Green before every commit.
- Every package has tests. `runner` is tested only through `httptest` servers.
- CI runs `make check` on every push.

## Measurement notes (for the README)

- Latencies include DNS, connect and TLS for the first request on each connection; subsequent requests reuse connections.
- Failed requests (timeouts, connection errors, body read errors) count toward the request and error totals only; they are excluded from latency, status codes and bytes.
- The tool measures what the client observes; it is not a substitute for server-side metrics.
- Running on the same machine as the target distorts results.
