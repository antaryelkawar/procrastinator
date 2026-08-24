package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	allRequired := map[string]string{
		"LM_DATABASE_URL": "postgres://user:pass@host:port/db",
		"LM_LLM_BASE_URL": "http://localhost:8081",
		"LM_LLM_API_KEY":  "some-api-key",
		"LM_LLM_MODEL":    "llama2",
		"LM_STORAGE_DIR":  "/tmp/storage",
	}

	tests := []struct {
		name             string
		src              map[string]string
		want             Config
		wantErrSubstring string
	}{
		{
			name: "all vars set",
			src: map[string]string{
				"LM_DATABASE_URL":     "postgres://user:pass@host:port/db",
				"LM_LLM_BASE_URL":     "http://localhost:8081",
				"LM_LLM_API_KEY":      "some-api-key",
				"LM_LLM_MODEL":        "llama2",
				"LM_STORAGE_DIR":      "/tmp/storage",
				"LM_HTTP_ADDR":        ":9090",
				"LM_MAX_UPLOAD_BYTES": "104857600",
				"LM_LLM_TIMEOUT":      "120s",
			},
			want: Config{
				DatabaseURL:    "postgres://user:pass@host:port/db",
				LLMBaseURL:     "http://localhost:8081",
				LLMAPIKey:      "some-api-key",
				LLMModel:       "llama2",
				StorageDir:     "/tmp/storage",
				HTTPAddr:       ":9090",
				MaxUploadBytes: 104857600,
				LLMTimeout:     120 * time.Second,
			},
		},
		{
			name: "optional vars default",
			src:  allRequired,
			want: Config{
				DatabaseURL:    "postgres://user:pass@host:port/db",
				LLMBaseURL:     "http://localhost:8081",
				LLMAPIKey:      "some-api-key",
				LLMModel:       "llama2",
				StorageDir:     "/tmp/storage",
				HTTPAddr:       ":8080",
				MaxUploadBytes: 20971520,
				LLMTimeout:     60 * time.Second,
			},
		},
		{
			name:             "missing LM_DATABASE_URL",
			src:              withoutKey(allRequired, "LM_DATABASE_URL"),
			wantErrSubstring: "LM_DATABASE_URL",
		},
		{
			name:             "missing LM_LLM_BASE_URL",
			src:              withoutKey(allRequired, "LM_LLM_BASE_URL"),
			wantErrSubstring: "LM_LLM_BASE_URL",
		},
		{
			name:             "missing LM_LLM_API_KEY",
			src:              withoutKey(allRequired, "LM_LLM_API_KEY"),
			wantErrSubstring: "LM_LLM_API_KEY",
		},
		{
			name:             "missing LM_LLM_MODEL",
			src:              withoutKey(allRequired, "LM_LLM_MODEL"),
			wantErrSubstring: "LM_LLM_MODEL",
		},
		{
			name:             "missing LM_STORAGE_DIR",
			src:              withoutKey(allRequired, "LM_STORAGE_DIR"),
			wantErrSubstring: "LM_STORAGE_DIR",
		},
		{
			name:             "empty LM_DATABASE_URL",
			src:              withEmptyKey(allRequired, "LM_DATABASE_URL"),
			wantErrSubstring: "LM_DATABASE_URL",
		},
		{
			name:             "empty LM_LLM_BASE_URL",
			src:              withEmptyKey(allRequired, "LM_LLM_BASE_URL"),
			wantErrSubstring: "LM_LLM_BASE_URL",
		},
		{
			name:             "empty LM_LLM_API_KEY",
			src:              withEmptyKey(allRequired, "LM_LLM_API_KEY"),
			wantErrSubstring: "LM_LLM_API_KEY",
		},
		{
			name:             "empty LM_LLM_MODEL",
			src:              withEmptyKey(allRequired, "LM_LLM_MODEL"),
			wantErrSubstring: "LM_LLM_MODEL",
		},
		{
			name:             "empty LM_STORAGE_DIR",
			src:              withEmptyKey(allRequired, "LM_STORAGE_DIR"),
			wantErrSubstring: "LM_STORAGE_DIR",
		},
		{
			name: "invalid max upload bytes",
			src: map[string]string{
				"LM_DATABASE_URL":     "postgres://user:pass@host:port/db",
				"LM_LLM_BASE_URL":     "http://localhost:8081",
				"LM_LLM_API_KEY":      "some-api-key",
				"LM_LLM_MODEL":        "llama2",
				"LM_STORAGE_DIR":      "/tmp/storage",
				"LM_MAX_UPLOAD_BYTES": "not-a-number",
			},
			wantErrSubstring: "LM_MAX_UPLOAD_BYTES",
		},
		{
			name: "invalid llm timeout",
			src: map[string]string{
				"LM_DATABASE_URL": "postgres://user:pass@host:port/db",
				"LM_LLM_BASE_URL": "http://localhost:8081",
				"LM_LLM_API_KEY":  "some-api-key",
				"LM_LLM_MODEL":    "llama2",
				"LM_STORAGE_DIR":  "/tmp/storage",
				"LM_LLM_TIMEOUT":  "not-a-duration",
			},
			wantErrSubstring: "LM_LLM_TIMEOUT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Load(tc.src)
			if tc.wantErrSubstring != "" {
				if err == nil {
					t.Fatalf("Load() succeeded, expected error containing %q", tc.wantErrSubstring)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstring) {
					t.Errorf("Load() error = %q, want substring %q", err.Error(), tc.wantErrSubstring)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}

			if *got != tc.want {
				t.Errorf("Load() got %+v, want %+v", *got, tc.want)
			}
		})
	}
}

// withoutKey returns a copy of src without the specified key.
func withoutKey(src map[string]string, key string) map[string]string {
	result := make(map[string]string)
	for k, v := range src {
		if k != key {
			result[k] = v
		}
	}
	return result
}

// withEmptyKey returns a copy of src with the specified key set to empty string.
func withEmptyKey(src map[string]string, key string) map[string]string {
	result := make(map[string]string)
	for k, v := range src {
		result[k] = v
	}
	result[key] = ""
	return result
}
