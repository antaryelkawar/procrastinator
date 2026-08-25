package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the server configuration.
type Config struct {
	DatabaseURL    string
	HTTPAddr       string
	LLMBaseURL     string
	LLMAPIKey      string
	LLMModel       string
	StorageDir     string
	MaxUploadBytes int64
	LLMTimeout     time.Duration
}

// Load populates a Config from the provided source map.
// If src is nil, it loads from environment variables.
func Load(src map[string]string) (*Config, error) {
	if src == nil {
		src = make(map[string]string)
		vars := []string{
			"PROCRASTINATOR_DATABASE_URL",
			"PROCRASTINATOR_HTTP_ADDR",
			"PROCRASTINATOR_LLM_BASE_URL",
			"PROCRASTINATOR_LLM_API_KEY",
			"PROCRASTINATOR_LLM_MODEL",
			"PROCRASTINATOR_STORAGE_DIR",
			"PROCRASTINATOR_MAX_UPLOAD_BYTES",
			"PROCRASTINATOR_LLM_TIMEOUT",
		}
		for _, v := range vars {
			if val, ok := os.LookupEnv(v); ok {
				src[v] = val
			}
		}
	}

	// Required variables — checked in this order.
	required := []string{
		"PROCRASTINATOR_DATABASE_URL",
		"PROCRASTINATOR_LLM_API_KEY",
		"PROCRASTINATOR_LLM_MODEL",
	}
	for _, name := range required {
		if src[name] == "" {
			return nil, fmt.Errorf("config: missing required environment variable %s", name)
		}
	}

	cfg := &Config{
		DatabaseURL: src["PROCRASTINATOR_DATABASE_URL"],
		LLMAPIKey:   src["PROCRASTINATOR_LLM_API_KEY"],
		LLMModel:    src["PROCRASTINATOR_LLM_MODEL"],
	}

	// Optional variables with defaults
	cfg.HTTPAddr = getOrDefault(src, "PROCRASTINATOR_HTTP_ADDR", ":8080")
	cfg.LLMBaseURL = getOrDefault(src, "PROCRASTINATOR_LLM_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai/")
	cfg.StorageDir = getOrDefault(src, "PROCRASTINATOR_STORAGE_DIR", "./storage")

	maxUploadStr := getOrDefault(src, "PROCRASTINATOR_MAX_UPLOAD_BYTES", "20971520")
	maxUpload, err := strconv.ParseInt(maxUploadStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_MAX_UPLOAD_BYTES: %w", err)
	}
	cfg.MaxUploadBytes = maxUpload

	timeoutStr := getOrDefault(src, "PROCRASTINATOR_LLM_TIMEOUT", "60s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_LLM_TIMEOUT: %w", err)
	}
	cfg.LLMTimeout = timeout

	return cfg, nil
}

func getOrDefault(src map[string]string, name, def string) string {
	if val, ok := src[name]; ok && val != "" {
		return val
	}
	return def
}
