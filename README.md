# loadr

A small HTTP load tester. loadr sends a fixed number of requests, or keeps
sending for a fixed time, to one URL from a fixed number of concurrent
workers, and reports latency percentiles, throughput, status codes and
errors. It uses only the Go standard library and builds to a single static
binary.

## Install

Needs Go 1.27 or later.

```sh
CGO_ENABLED=0 go install github.com/Samir120/loadr/cmd/loadr@v0.1.0
```

`CGO_ENABLED=0` gives a static binary. Without it, `go install` links the
`net` package against the host's libc, and the binary may not run on another
Linux distribution.

From source:

```sh
git clone https://github.com/Samir120/loadr
cd loadr
make build    # static binary at ./loadr
```

## Usage

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

```sh
# 1000 requests, 50 at a time
loadr -n 1000 -c 50 http://localhost:8080/

# 30 seconds, at most 200 requests per second
loadr -d 30s -c 20 -q 200 http://localhost:8080/health

# POST a JSON file
loadr -n 500 -m POST -H "Content-Type: application/json" -body @order.json http://localhost:8080/orders

# Hit a reverse proxy directly and pick the virtual host
loadr -d 10s -c 50 -H "Host: staging.example.com" http://10.43.0.10/

# Fail a CI step if any request failed
loadr -n 1000 -o json http://localhost:8080/ | jq -e '.errors == 0'
```

A `-body` sent without a `Content-Type` header is labelled
`application/octet-stream`. `-H "Host: ..."` sets the request's Host. A literal
body that starts with `@` cannot be given inline; put it in a file.

Ctrl-C ends the run early and prints what was collected so far.

| Exit code | Meaning |
|----------:|---------|
| 0   | run completed, even if some requests failed |
| 1   | run failed to start, or runtime error |
| 2   | invalid flags or URL |
| 130 | interrupted by Ctrl-C (partial report printed) |

Failed requests are measurements, not run errors: they show up in the report,
not in the exit code. To gate on them, read `errors` or `error_classes` from
`-o json`.

## Output

Ten seconds at 50 workers against a staging site on a single-node k3s,
with loadr on the same machine:

```
Summary
  Requests  11724         Duration  10.00s
  Rate      1172.1 req/s  Bytes     46.7 MB

Latency
  min  0.1ms  mean  42.4ms   max  601.1ms
  p50  3.3ms  p90   101.9ms  p95  159.6ms  p99  292.6ms

Status codes
  200  11724
```

Latency and Status codes are left out when no request succeeded, and Errors
when none failed. Errors gives the total, then one line per class, most common
first:

| Class      | Meaning |
|------------|---------|
| `fd_limit` | loadr ran out of file descriptors; the request never left the machine |
| `timeout`  | `-t` expired, at any stage: DNS, connect, TLS, headers or body |
| `dns`      | the host name did not resolve |
| `refused`  | the connection was refused; nothing was listening |
| `closed`   | the server closed or reset the connection before the response was complete |
| `other`    | anything else |

An HTTP 4xx or 5xx is not an error: the server answered, and the answer is
counted under Status codes.

`-o json` prints the same data as one line. Durations are fractional
milliseconds, every field is always present, and field names never change:

```json
{"requests":10000,"errors":8,"duration_ms":4210,"rate_rps":2375.3,"bytes":1234567,
 "latency_ms":{"min":0.812,"mean":41.2,"max":312.6,"p50":38.1,"p90":67.4,"p95":84,"p99":142.7},
 "status_codes":{"200":9980,"503":12},"error_classes":{"closed":3,"refused":2,"timeout":3}}
```

## How loadr measures

- **Latency** runs from sending the request to reading the last byte of the
  body. The body is read in full and discarded, so connections stay reusable.
- **Connections are reused.** The connection pool matches `-c`. The first
  request on each connection also pays for DNS, connect and TLS, so at high
  `-c` the burst of new connections shows up in the tail. In one run against
  a single-node k3s, `-c 500` put p99 at 3.0 s.
- **Redirects are followed**, up to 10, and latency covers the whole chain.
  A plain `http://` URL that redirects to `https://` can send the whole test
  somewhere else, for example through a CDN to the public site. Point loadr at
  the final URL.
- **Failed requests** count toward Requests, Errors and Rate, never toward
  latency, status codes or bytes, so a 10 s timeout cannot drag p99 up.
- **Rate** is completed requests, failed ones included, divided by
  wall-clock time. Failures on the client's own side return instantly: when
  loadr runs out of file descriptors, workers spin, and a `-d` run reports a
  huge Requests and Rate. One such run showed 2,560,531 errors and
  "259,633 req/s". Read Status codes and Errors before you trust Rate.
- **`-q` is a ceiling, not a guarantee.** All workers share one ticker, so
  the run never goes faster than `-q`, but it falls short when the target or
  the client cannot keep up.
- **File descriptors.** Each connection needs one. Go raises the soft
  `RLIMIT_NOFILE` to the hard limit at startup, so `ulimit -n` is not loadr's
  limit. To impose one, use `prlimit --nofile=N:N loadr ...`.
- **The end of a run.** Requests still in flight when `-d` expires or Ctrl-C
  arrives are abandoned and not counted.
- **Progress.** While the test runs, loadr redraws a count of completed
  requests on stderr, and erases it before the report. It does this only when
  stderr is a terminal, so piping or redirecting stderr to a file hides it.
  `/dev/null` counts as a terminal, which is harmless.
- **What it sees.** loadr measures what the client observes; it is no
  substitute for server-side metrics. Running it on the same machine as the
  target, as in the example above, means both compete for CPU.

## Development

```sh
make check    # gofmt, go vet, staticcheck, go test -race
make build    # static binary at ./loadr
```

Standard library only. CI runs `make check` on every push.
