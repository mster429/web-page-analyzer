package config

import (
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

var variables = []string{
	"HOST_PORT", "PAGE_FETCH_TIMEOUT", "SINGLE_LINK_CHECK_TIMEOUT", "TOTAL_LINK_CHECK_TIMEOUT",
	"LINK_CHECK_WORKERS", "PAGE_SIZE_LIMIT_MB", "ENABLE_PPROF", "LOG_LEVEL",
}

// clearEnv makes sure a setting from the developer's own shell cannot change the result.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range variables {
		t.Setenv(name, "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}

	want := Config{
		HostPort:               ":8080",
		PageFetchTimeout:       10 * time.Second,
		SingleLinkCheckTimeout: 5 * time.Second,
		TotalLinkCheckTimeout:  20 * time.Second,
		LinkCheckWorkers:       20,
		PageSizeLimitMB:        5,
		EnableProfiling:        true,
		LogLevel:               zerolog.InfoLevel,
	}
	if got != want {
		t.Errorf("Load() = %+v\nwant     %+v", got, want)
	}
}

func TestLoad_FromEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOST_PORT", "127.0.0.1:9090")
	t.Setenv("PAGE_FETCH_TIMEOUT", "3s")
	t.Setenv("SINGLE_LINK_CHECK_TIMEOUT", "500ms")
	t.Setenv("TOTAL_LINK_CHECK_TIMEOUT", "1m")
	t.Setenv("LINK_CHECK_WORKERS", "8")
	t.Setenv("PAGE_SIZE_LIMIT_MB", "2")
	t.Setenv("ENABLE_PPROF", "false")
	t.Setenv("LOG_LEVEL", "debug")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}

	// every value differs from its default, so a variable that is not read shows up here
	want := Config{
		HostPort:               "127.0.0.1:9090",
		PageFetchTimeout:       3 * time.Second,
		SingleLinkCheckTimeout: 500 * time.Millisecond,
		TotalLinkCheckTimeout:  time.Minute,
		LinkCheckWorkers:       8,
		PageSizeLimitMB:        2,
		EnableProfiling:        false,
		LogLevel:               zerolog.DebugLevel,
	}
	if got != want {
		t.Errorf("Load() = %+v\nwant     %+v", got, want)
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantInErr string
	}{
		{"PAGE_FETCH_TIMEOUT", "ten seconds", "PAGE_FETCH_TIMEOUT"},
		{"PAGE_FETCH_TIMEOUT", "10", "PAGE_FETCH_TIMEOUT"}, // a number without a unit
		{"SINGLE_LINK_CHECK_TIMEOUT", "soon", "SINGLE_LINK_CHECK_TIMEOUT"},
		{"TOTAL_LINK_CHECK_TIMEOUT", "later", "TOTAL_LINK_CHECK_TIMEOUT"},
		{"LINK_CHECK_WORKERS", "many", "LINK_CHECK_WORKERS"},
		{"LINK_CHECK_WORKERS", "2.5", "LINK_CHECK_WORKERS"},
		{"PAGE_SIZE_LIMIT_MB", "big", "PAGE_SIZE_LIMIT_MB"},
		{"ENABLE_PPROF", "maybe", "ENABLE_PPROF"},
		{"LOG_LEVEL", "loud", "LOG_LEVEL"},

		// readable, but not allowed
		{"PAGE_FETCH_TIMEOUT", "0s", "timeouts"},
		{"SINGLE_LINK_CHECK_TIMEOUT", "-1s", "timeouts"},
		{"TOTAL_LINK_CHECK_TIMEOUT", "0s", "timeouts"},
		{"LINK_CHECK_WORKERS", "0", "LINK_CHECK_WORKERS"},
		{"PAGE_SIZE_LIMIT_MB", "0", "PAGE_SIZE_LIMIT_MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"="+tt.value, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tt.name, tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load accepted the value")
			}
			// the message has to say which setting is wrong
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantInErr)
			}
		})
	}
}
