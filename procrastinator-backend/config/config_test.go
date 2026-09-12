package config

import (
	"os"
	"path/filepath"
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
		"PROCRASTINATOR_BASIC_AUTH_USERS": `[{"user":"admin","pass":"s3cret-pw"}]`,
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
			name: "all vars set non-default",
			src: map[string]string{
				"PROCRASTINATOR_DATABASE_URL":            "postgres://user:pass@localhost:5432/db",
				"PROCRASTINATOR_LLM_API_KEY":             "api-key-123",
				"PROCRASTINATOR_LLM_MODEL":               "gemma-4-26b-a4b-it",
				"PROCRASTINATOR_HTTP_ADDR":               ":9090",
				"PROCRASTINATOR_LLM_BASE_URL":            "https://example.invalid/v1",
				"PROCRASTINATOR_STORAGE_DIR":             "/tmp/alt-storage",
				"PROCRASTINATOR_MAX_UPLOAD_BYTES":        "1024",
				"PROCRASTINATOR_MAX_STATEMENT_BYTES":     "10485760",
				"PROCRASTINATOR_MAX_STATEMENT_LINES":     "2500",
				"PROCRASTINATOR_LLM_TIMEOUT":             "45s",
				"PROCRASTINATOR_INGEST_REVIEW_THRESHOLD": "0.92",
				"PROCRASTINATOR_BASIC_AUTH_USERS":        `[{"user":"admin","pass":"s3cret-pw"}]`,
			},
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     10485760,
				MaxStatementLines:     2500,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 0.92,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BrandLexicon:             nil,
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "defaults applied for optional vars",
			src: map[string]string{
				"PROCRASTINATOR_DATABASE_URL":     "postgres://user:pass@localhost:5432/db",
				"PROCRASTINATOR_LLM_API_KEY":      "api-key-123",
				"PROCRASTINATOR_LLM_MODEL":        "gemma-4-26b-a4b-it",
				"PROCRASTINATOR_BASIC_AUTH_USERS": `[{"user":"admin","pass":"s3cret-pw"}]`,
			},
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":8080",
				LLMBaseURL:            "https://generativelanguage.googleapis.com/v1beta/openai/",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "./storage",
				MaxUploadBytes:        20971520,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            60 * time.Second,
				IngestReviewThreshold: 0.7,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BrandLexicon:             nil,
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "valid threshold 0.0",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_INGEST_REVIEW_THRESHOLD"] = "0"
				return m
			}(),
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 0.0,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BrandLexicon:             nil,
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "valid threshold 1.0",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_INGEST_REVIEW_THRESHOLD"] = "1"
				return m
			}(),
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 1.0,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BrandLexicon:             nil,
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "valid threshold 0.9",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_INGEST_REVIEW_THRESHOLD"] = "0.9"
				return m
			}(),
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 0.9,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BrandLexicon:             nil,
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "invalid threshold abc",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_INGEST_REVIEW_THRESHOLD"] = "abc"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_INGEST_REVIEW_THRESHOLD",
			want:        nil,
		},
		{
			name: "invalid threshold 1.5 out of range",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_INGEST_REVIEW_THRESHOLD"] = "1.5"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_INGEST_REVIEW_THRESHOLD",
			want:        nil,
		},
		{
			name: "invalid threshold -0.1 out of range",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_INGEST_REVIEW_THRESHOLD"] = "-0.1"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_INGEST_REVIEW_THRESHOLD",
			want:        nil,
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
			name: "invalid PROCRASTINATOR_MAX_STATEMENT_BYTES",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_MAX_STATEMENT_BYTES"] = "not-a-number"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_MAX_STATEMENT_BYTES",
			want:        nil,
		},
		{
			name: "invalid PROCRASTINATOR_MAX_STATEMENT_LINES",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_MAX_STATEMENT_LINES"] = "also-bad"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_MAX_STATEMENT_LINES",
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
		{
			name: "explicit LLM workers",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_LLM_WORKERS"] = "model-a@https://a.invalid/v1, model-b"
				return m
			}(),
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 0.7,
				LLMWorkers: []Worker{
					{Model: "model-a", BaseURL: "https://a.invalid/v1", Strategy: "extract"},
					{Model: "model-b", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "explicit retention candidate timeout",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+3)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS"] = "45"
				m["PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT"] = "25"
				m["PROCRASTINATOR_PROCESS_TIMEOUT"] = "45s"
				return m
			}(),
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 0.7,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 45,
				LookupCandidateLimit:     25,
				ProcessTimeout:           45 * time.Second,
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "invalid PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS"] = "abc"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS",
			want:        nil,
		},
		{
			name: "invalid PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT"] = "xyz"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT",
			want:        nil,
		},
		{
			name: "invalid PROCRASTINATOR_PROCESS_TIMEOUT",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_PROCESS_TIMEOUT"] = "nope"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_PROCESS_TIMEOUT",
			want:        nil,
		},
		{
			name: "invalid PROCRASTINATOR_LLM_WORKERS empty entry",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_LLM_WORKERS"] = "model-a,"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LLM_WORKERS",
			want:        nil,
		},
		{
			name: "brand lexicon json path",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				p := filepath.Join(t.TempDir(), "brands.json")
				if err := os.WriteFile(p, []byte(`["Alpha","Beta"]`), 0o644); err != nil {
					t.Fatalf("write brands.json: %v", err)
				}
				m["PROCRASTINATOR_BRAND_LEXICON"] = p
				return m
			}(),
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 0.7,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BrandLexicon:             []string{"Alpha", "Beta"},
				BasicAuthUsers:           []Credential{{User: "admin", Pass: "s3cret-pw"}},
			},
		},
		{
			name: "PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS must be positive",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS"] = "0"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT must be positive",
			src: func() map[string]string {
				m := make(map[string]string, len(base)+1)
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT"] = "-3"
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT",
			want:        nil,
		},
		{
			name:        "missing PROCRASTINATOR_BASIC_AUTH_USERS",
			src:         without(base, "PROCRASTINATOR_BASIC_AUTH_USERS"),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS empty array",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[]`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS malformed JSON",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `not-json`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS not an array",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `{"user":"a","pass":"password1"}`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS extra unknown key",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[{"user":"admin","pass":"s3cret-pw","extra":"x"}]`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS user empty (0 chars)",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[{"user":"","pass":"s3cret-pw"}]`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS user 65 chars",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[{"user":"` + strings.Repeat("a", 65) + `","pass":"s3cret-pw"}]`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS pass 7 chars",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[{"user":"admin","pass":"1234567"}]`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS pass 129 chars",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[{"user":"admin","pass":"` + strings.Repeat("p", 129) + `"}]`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS 17 pairs",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[` + strings.Repeat(`{"user":"u","pass":"password1"},`, 16) + `{"user":"u","pass":"password1"}` + `]`
				return m
			}(),
			wantErr:     true,
			errContains: "PROCRASTINATOR_BASIC_AUTH_USERS",
			want:        nil,
		},
		{
			name: "PROCRASTINATOR_BASIC_AUTH_USERS valid 2 pairs",
			src: func() map[string]string {
				m := make(map[string]string, len(base))
				for k, v := range base {
					m[k] = v
				}
				m["PROCRASTINATOR_BASIC_AUTH_USERS"] = `[{"user":"admin","pass":"s3cret-pw"},{"user":"deploy","pass":"deploy-pw"}]`
				return m
			}(),
			wantErr: false,
			want: &Config{
				DatabaseURL:           "postgres://user:pass@localhost:5432/db",
				HTTPAddr:              ":9090",
				LLMBaseURL:            "https://example.invalid/v1",
				LLMAPIKey:             "api-key-123",
				LLMModel:              "gemma-4-26b-a4b-it",
				StorageDir:            "/tmp/alt-storage",
				MaxUploadBytes:        1024,
				MaxStatementBytes:     52428800,
				MaxStatementLines:     100000,
				LLMTimeout:            45 * time.Second,
				IngestReviewThreshold: 0.7,
				LLMWorkers: []Worker{
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "extract"},
					{Model: "gemma-4-26b-a4b-it", BaseURL: "https://example.invalid/v1", Strategy: "verify"},
				},
				AssetDeleteRetentionDays: 30,
				LookupCandidateLimit:     10,
				ProcessTimeout:           30 * time.Second,
				BrandLexicon:             nil,
				BasicAuthUsers: []Credential{
					{User: "admin", Pass: "s3cret-pw"},
					{User: "deploy", Pass: "deploy-pw"},
				},
			},
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
