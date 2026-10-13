// Package config loads the settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

// Config holds the settings of the application.
type Config struct {
	HostPort               string
	PageFetchTimeout       time.Duration
	SingleLinkCheckTimeout time.Duration
	TotalLinkCheckTimeout  time.Duration
	LinkCheckWorkers       int
	PageSizeLimitMB        int
	EnableProfiling        bool
	LogLevel               zerolog.Level
}

// Load reads environment variables loads values those to variables
// if a variable is not set or empty, a default value is assigned
func Load() (Config, error) {
	var c Config
	var err error

	c.HostPort = envString("HOST_PORT", ":8080")

	c.PageFetchTimeout, err = envDuration("PAGE_FETCH_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	c.SingleLinkCheckTimeout, err = envDuration("SINGLE_LINK_CHECK_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	c.TotalLinkCheckTimeout, err = envDuration("TOTAL_LINK_CHECK_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}

	c.LinkCheckWorkers, err = envInt("LINK_CHECK_WORKERS", 20)
	if err != nil {
		return Config{}, err
	}
	c.PageSizeLimitMB, err = envInt("PAGE_SIZE_LIMIT_MB", 5)
	if err != nil {
		return Config{}, err
	}

	c.EnableProfiling, err = envBool("ENABLE_PPROF", true)
	if err != nil {
		return Config{}, err
	}

	c.LogLevel, err = envLogLevel("LOG_LEVEL", zerolog.InfoLevel)
	if err != nil {
		return Config{}, err
	}

	// validations
	if c.PageFetchTimeout <= 0 || c.SingleLinkCheckTimeout <= 0 || c.TotalLinkCheckTimeout <= 0 {
		return Config{}, errors.New("timeouts must be greater than zero")
	}
	if c.LinkCheckWorkers < 1 {
		return Config{}, errors.New("LINK_CHECK_WORKERS must be at least 1")
	}
	if c.PageSizeLimitMB < 1 {
		return Config{}, errors.New("PAGE_SIZE_LIMIT_MB must be at least 1")
	}
	return c, nil
}

func envString(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a duration like 10s or 500ms", name, value)
	}
	return d, nil
}

func envInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a whole number", name, value)
	}
	return n, nil
}

func envBool(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s=%q is not true or false", name, value)
	}
	return b, nil
}

func envLogLevel(name string, fallback zerolog.Level) (zerolog.Level, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	level, err := zerolog.ParseLevel(value)
	if err != nil {
		return fallback, fmt.Errorf("provided %s=%q is not a supported log level [debug, info, warn, error]", name, value)
	}
	return level, nil
}
