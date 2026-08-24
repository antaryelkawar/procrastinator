// Package config provides configuration loading for the procrastinator backend.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration.
type Config struct {
	DatabaseURL    string
	LLMBaseURL     string
	LLMAPIKey      string
	LLMModel       string
	StorageDir     string
	HTTPAddr       string
	MaxUploadBytes int64
	LLMTimeout     time.Duration
}

// Load reads configuration from the provided source map. If src is nil, values are read from the real environment (os.Getenv). If src is non-nil, values are read from the map (missing key = unset).
func Load(src map[string]string) (*Config, error) {
	var cfg Config

	getValue := func(key string) string {
		if src == nil {
			return os.Getenv(key)
		}
		val, exists := src[key]
		if !exists {
			return ""
		}
		return val
	}

	// Required vars (in order)
	requiredVars := []struct {
		key  string
		dest *string
	}{
		{"LM_DATABASE_URL", &cfg.DatabaseURL},
		{"LM_LLM_BASE_URL", &cfg.LLMBaseURL},
		{"LM_LLM_API_KEY", &cfg.LLMAPIKey},
		{"LM_LLM_MODEL", &cfg.LLMModel},
		{"LM_STORAGE_DIR", &cfg.StorageDir},
	}

	for _, req := range requiredVars {
		val := getValue(req.key)
		if val == "" {
			return nil, fmt.Errorf("missing required environment variable: %s", req.key)
		}
		*req.dest = val
	}

	// Optional: LM_HTTP_ADDR
	cfg.HTTPAddr = getValue("LM_HTTP_ADDR")
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}

	// Optional: LM_MAX_UPLOAD_BYTES
	maxUploadStr := getValue("LM_MAX_UPLOAD_BYTES")
	if maxUploadStr == "" {
		cfg.MaxUploadBytes = 20971520
	} else {
		val, err := strconv.ParseInt(maxUploadStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value for LM_MAX_UPLOAD_BYTES: %w", err)
		}
		cfg.MaxUploadBytes = val
	}

	// Optional: LM_LLM_TIMEOUT
	timeoutStr := getValue("LM_LLM_TIMEOUT")
	if timeoutStr == "" {
		cfg.LLMTimeout = 60 * time.Second
	} else {
		val, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid value for LM_LLM_TIMEOUT: %w", err)
		}
		cfg.LLMTimeout = val
	}

	return &cfg, nil
}
