package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	base := map[string]string{
		"PROCRASTINATOR_DATABASE_URL":     "postgres://user:pass@localhost:5432/db",
		"PROCRASTINATOR_LLM_API_KEY":      "api-key-123",
		"PROCRASTINATOR_LLM_MODEL":        "gemma-4-26b-a4b-it",
		"PROCRASTINATOR_HTTP_ADDR":        ":9090",
		"PROCRASTINATOR_LLM_BASE_URL":     "https://example.invalid/v1",
		"PROCRASTINATOR_STORAGE_DIR":      "/tmp/alt-storage",
		"PROCRASTINATOR_MAX_UPLOAD_BYTES": "1024",
		"PROCRASTINATOR_LLM_TIMEOUT":      "45s",
	}

	without := func(src map[string]string, keys ...string) map[string]string {
		out := make(map[string]string, len(src))
		for k, v := range src {
			out[k] = v
		}
		for _, k := range keys {
			delete(out, k)
		}
		return out
	}

	cases := []struct {
		name        string
		src         map[string]string
		wantErr     bool
		errContains string
		want        *Config
	}{
		{
			name:    "all vars set non-default",
			src:     base,
			wantErr: false,
			want: &Config{
				DatabaseURL:    "postgres://user:pass@localhost:5432/db",
				HTTPAddr:       ":9090",
				LLMBaseURL:     "https://example.invalid/v1",
				LLMAPIKey:      "api-key-123",
				LLMModel:       "gemma-4-26b-a4b-it",
				StorageDir:     "/tmp/alt-storage",
				MaxUploadBytes: 1024,
				LLMTimeout:     45 * time.Second,
			},
		},
		{
			name: "defaults applied for optional vars",
			src: map[string]string{
				"PROCRASTINATOR_DATABASE_URL": "postgres://user:pass@localhost:5432/db",
				"PROCRASTINATOR_LLM_API_KEY":  "api-key-123",
				"PROCRASTINATOR_LLM_MODEL":    "gemma-4-26b-a4b-it",
			},
			wantErr: false,
			want: &Config{
				DatabaseURL:    "postgres://user:pass@localhost:5432/db",
				HTTPAddr:       ":8080",
				LLMBaseURL:     "https://generativelanguage.googleapis.com/v1beta/openai/",
				LLMAPIKey:      "api-key-123",
				LLMModel:       "gemma-4-26b-a4b-it",
				StorageDir:     "./storage",
				MaxUploadBytes: 20971520,
				LLMTimeout:     60 * time.Second,
			},
		},
		{
			name:        "missing PROCRASTINATOR_DATABASE_URL",
			src:         without(base, "PROCRASTINATOR_DATABASE_URL"),
			wantErr:     true,
			errContains: "PROCRASTINATOR_DATABASE_URL",
			want:        nil,
		},
		{
			name:        "missing PROCRASTINATOR_LLM_API_KEY",
			src:         without(base, "PROCRASTINATOR_LLM_API_KEY"),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LLM_API_KEY",
			want:        nil,
		},
		{
			name:        "missing PROCRASTINATOR_LLM_MODEL",
			src:         without(base, "PROCRASTINATOR_LLM_MODEL"),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LLM_MODEL",
			want:        nil,
		},
		{
			name: "empty PROCRASTINATOR_DATABASE_URL",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_DATABASE_URL"] = ""
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_DATABASE_URL",
			want:        nil,
		},
		{
			name: "empty PROCRASTINATOR_LLM_API_KEY",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_LLM_API_KEY"] = ""
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LLM_API_KEY",
			want:        nil,
		},
		{
			name: "empty PROCRASTINATOR_LLM_MODEL",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_LLM_MODEL"] = ""
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LLM_MODEL",
			want:        nil,
		},
		{
			name:        "multiple missing names first in check order",
			src:         without(base, "PROCRASTINATOR_DATABASE_URL", "PROCRASTINATOR_LLM_MODEL"),
			wantErr:     true,
			errContains: "PROCRASTINATOR_DATABASE_URL",
			want:        nil,
		},
		{
			name: "invalid PROCRASTINATOR_MAX_UPLOAD_BYTES",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_MAX_UPLOAD_BYTES"] = "not-a-number"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_MAX_UPLOAD_BYTES",
			want:        nil,
		},
		{
			name: "invalid PROCRASTINATOR_LLM_TIMEOUT",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_LLM_TIMEOUT"] = "not-a-duration"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LLM_TIMEOUT",
			want:        nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(tc.src)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContains)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
				}
				if cfg != nil {
					t.Errorf("expected nil Config on error, got %+v", cfg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg == nil {
				t.Fatal("expected non-nil Config")
			}
			if tc.want == nil {
				t.Fatal("test case missing want Config")
			}

			if !reflect.DeepEqual(cfg, tc.want) {
				t.Errorf("Config mismatch\nwant: %+v\ngot:  %+v", tc.want, cfg)
			}
		})
	}
}
