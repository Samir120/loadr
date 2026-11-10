package runner

import (
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	valid := Config{
		URL:         "http://example.com",
		Requests:    10,
		Concurrency: 2,
		Timeout:     time.Second,
	}

	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "duration instead of requests", mutate: func(c *Config) { c.Requests = 0; c.Duration = time.Second }},
		{name: "empty url", mutate: func(c *Config) { c.URL = "" }, wantErr: true},
		{name: "no scheme", mutate: func(c *Config) { c.URL = "example.com" }, wantErr: true},
		{name: "ftp scheme", mutate: func(c *Config) { c.URL = "ftp://example.com" }, wantErr: true},
		{name: "no host", mutate: func(c *Config) { c.URL = "http://" }, wantErr: true},
		{name: "zero concurrency", mutate: func(c *Config) { c.Concurrency = 0 }, wantErr: true},
		{name: "negative duration", mutate: func(c *Config) { c.Duration = -time.Second }, wantErr: true},
		{name: "no requests and no duration", mutate: func(c *Config) { c.Requests = 0 }, wantErr: true},
		{name: "zero timeout", mutate: func(c *Config) { c.Timeout = 0 }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
