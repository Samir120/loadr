package runner

import (
	"context"
	"io"
	"net/http"
	"slices"
	"testing"
)

// seen is what the test server observed about one request.
type seen struct {
	method        string
	host          string
	header        http.Header
	body          string
	contentLength int64
}

func TestRunSendsConfiguredRequest(t *testing.T) {
	const requests = 3

	tests := []struct {
		name       string
		method     string
		header     http.Header
		body       string
		wantMethod string
		wantHost   string // "" means the host from the URL
	}{
		{name: "empty method means GET", wantMethod: http.MethodGet},
		{name: "DELETE without body", method: http.MethodDelete, wantMethod: http.MethodDelete},
		{
			name:       "POST with body and headers",
			method:     http.MethodPost,
			header:     http.Header{"Content-Type": {"application/json"}, "X-Multi": {"a", "b"}},
			body:       `{"k":1}`,
			wantMethod: http.MethodPost,
		},
		{
			name:       "Host header sets the request host",
			header:     http.Header{"Host": {"api.example.test"}},
			wantMethod: http.MethodGet,
			wantHost:   "api.example.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make(chan seen, requests)
			srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading request body: %v", err)
				}
				got <- seen{
					method:        r.Method,
					host:          r.Host,
					header:        r.Header.Clone(),
					body:          string(body),
					contentLength: r.ContentLength,
				}
			})

			cfg := testConfig(srv.URL, requests, 2)
			cfg.Method = tt.method
			cfg.Header = tt.header
			if tt.body != "" {
				cfg.Body = []byte(tt.body)
			}
			res, err := Run(context.Background(), cfg)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if res.Requests != requests || res.Errors != 0 {
				t.Fatalf("Result has %d requests and %d errors, want %d and 0", res.Requests, res.Errors, requests)
			}

			wantHost := tt.wantHost
			if wantHost == "" {
				wantHost = srv.Listener.Addr().String()
			}
			for i := range requests {
				s := <-got
				if s.method != tt.wantMethod {
					t.Errorf("request %d: method = %q, want %q", i, s.method, tt.wantMethod)
				}
				if s.host != wantHost {
					t.Errorf("request %d: host = %q, want %q", i, s.host, wantHost)
				}
				if s.body != tt.body {
					t.Errorf("request %d: body = %q, want %q", i, s.body, tt.body)
				}
				if s.contentLength != int64(len(tt.body)) {
					t.Errorf("request %d: Content-Length = %d, want %d", i, s.contentLength, len(tt.body))
				}
				for name, want := range tt.header {
					if name == "Host" {
						continue // the server moves it to r.Host
					}
					if v := s.header.Values(name); !slices.Equal(v, want) {
						t.Errorf("request %d: header %s = %q, want %q", i, name, v, want)
					}
				}
			}
		})
	}
}
