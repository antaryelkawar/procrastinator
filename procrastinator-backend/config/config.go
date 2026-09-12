package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
	// MaxStatementBytes is the maximum size in bytes of a bank statement file
	// accepted for ingestion.
	MaxStatementBytes int64
	// MaxStatementLines is the maximum number of lines of a bank statement
	// file accepted for ingestion.
	MaxStatementLines int
	LLMTimeout        time.Duration
	IngestReviewThreshold float64

	// LLMWorkers are the configured extraction workers. When
	// PROCRASTINATOR_LLM_WORKERS is unset the default is two workers on LLMModel
	// with the extract/verify strategies (dual consensus, D2).
	LLMWorkers []Worker
	// AssetDeleteRetentionDays is the soft-delete retention window in days
	// (restore is allowed within this window).
	AssetDeleteRetentionDays int
	// LookupCandidateLimit bounds the candidate set returned by each
	// identity-lookup stage.
	LookupCandidateLimit int
	// ProcessTimeout is the end-to-end synchronous processing budget.
	ProcessTimeout time.Duration
	// BrandLexicon is the brand list loaded from the PROCRASTINATOR_BRAND_LEXICON
	// JSON path (nil when unset: the embedded canonical list is used).
	BrandLexicon []string
	// BasicAuthUsers holds the configured Basic Auth credential pairs,
	// parsed from PROCRASTINATOR_BASIC_AUTH_USERS (required).
	BasicAuthUsers []Credential
}

// Credential holds a Basic Auth username and password pair.
type Credential struct {
	User string
	Pass string
}

// Worker describes one configured extraction worker (a model at a base URL
// with a prompting strategy). It is a config-native mirror of
// core/processing.Worker so the config package does not import core.
type Worker struct {
	Model    string
	BaseURL  string
	Strategy string
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
			"PROCRASTINATOR_MAX_STATEMENT_BYTES",
			"PROCRASTINATOR_MAX_STATEMENT_LINES",
			"PROCRASTINATOR_LLM_TIMEOUT",
			"PROCRASTINATOR_INGEST_REVIEW_THRESHOLD",
			"PROCRASTINATOR_BASIC_AUTH_USERS",
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
		"PROCRASTINATOR_BASIC_AUTH_USERS",
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

	maxStatementBytesStr := getOrDefault(src, "PROCRASTINATOR_MAX_STATEMENT_BYTES", "52428800")
	maxStatementBytes, err := strconv.ParseInt(maxStatementBytesStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_MAX_STATEMENT_BYTES: %w", err)
	}
	cfg.MaxStatementBytes = maxStatementBytes

	maxStatementLinesStr := getOrDefault(src, "PROCRASTINATOR_MAX_STATEMENT_LINES", "100000")
	maxStatementLines, err := strconv.ParseInt(maxStatementLinesStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_MAX_STATEMENT_LINES: %w", err)
	}
	cfg.MaxStatementLines = int(maxStatementLines)

	timeoutStr := getOrDefault(src, "PROCRASTINATOR_LLM_TIMEOUT", "60s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_LLM_TIMEOUT: %w", err)
	}
	cfg.LLMTimeout = timeout

	thresholdStr := getOrDefault(src, "PROCRASTINATOR_INGEST_REVIEW_THRESHOLD", "0.7")
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_INGEST_REVIEW_THRESHOLD: %w", err)
	}
	if threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("config: PROCRASTINATOR_INGEST_REVIEW_THRESHOLD must be in [0,1], got %v", threshold)
	}
	cfg.IngestReviewThreshold = threshold

	retentionStr := getOrDefault(src, "PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS", "30")
	retentionDays, err := strconv.Atoi(retentionStr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS: %w", err)
	}
	if retentionDays <= 0 {
		return nil, fmt.Errorf("config: PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS must be > 0, got %d", retentionDays)
	}
	cfg.AssetDeleteRetentionDays = retentionDays

	candStr := getOrDefault(src, "PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT", "10")
	candLimit, err := strconv.Atoi(candStr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT: %w", err)
	}
	if candLimit <= 0 {
		return nil, fmt.Errorf("config: PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT must be > 0, got %d", candLimit)
	}
	cfg.LookupCandidateLimit = candLimit

	procStr := getOrDefault(src, "PROCRASTINATOR_PROCESS_TIMEOUT", "30s")
	procTimeout, err := time.ParseDuration(procStr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid PROCRASTINATOR_PROCESS_TIMEOUT: %w", err)
	}
	cfg.ProcessTimeout = procTimeout

	if path := src["PROCRASTINATOR_BRAND_LEXICON"]; path != "" {
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("config: reading PROCRASTINATOR_BRAND_LEXICON %q: %w", path, rerr)
		}
		var brands []string
		if jerr := json.Unmarshal(data, &brands); jerr != nil {
			return nil, fmt.Errorf("config: PROCRASTINATOR_BRAND_LEXICON %q must be a JSON array of strings: %w", path, jerr)
		}
		cfg.BrandLexicon = brands
	}

	if workersRaw := src["PROCRASTINATOR_LLM_WORKERS"]; workersRaw == "" {
		cfg.LLMWorkers = []Worker{
			{Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL, Strategy: "extract"},
			{Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL, Strategy: "verify"},
		}
	} else {
		workers, werr := parseLLMWorkers(workersRaw, cfg.LLMBaseURL)
		if werr != nil {
			return nil, werr
		}
		cfg.LLMWorkers = workers
	}

	raw := src["PROCRASTINATOR_BASIC_AUTH_USERS"]
	var entries []struct {
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("config: PROCRASTINATOR_BASIC_AUTH_USERS must be a JSON array of {\"user\",\"pass\"} objects: %w", err)
	}
	if len(entries) < 1 || len(entries) > 16 {
		return nil, fmt.Errorf("config: PROCRASTINATOR_BASIC_AUTH_USERS must contain 1-16 credential pairs, got %d", len(entries))
	}
	creds := make([]Credential, 0, len(entries))
	for i, e := range entries {
		ulen := utf8.RuneCountInString(e.User)
		if ulen < 1 || ulen > 64 {
			return nil, fmt.Errorf("config: PROCRASTINATOR_BASIC_AUTH_USERS pair %d: user must be 1-64 chars, got %d", i+1, ulen)
		}
		plen := utf8.RuneCountInString(e.Pass)
		if plen < 8 || plen > 128 {
			return nil, fmt.Errorf("config: PROCRASTINATOR_BASIC_AUTH_USERS pair %d: pass must be 8-128 chars, got %d", i+1, plen)
		}
		creds = append(creds, Credential{User: e.User, Pass: e.Pass})
	}
	cfg.BasicAuthUsers = creds

	return cfg, nil
}

func getOrDefault(src map[string]string, name, def string) string {
	if val, ok := src[name]; ok && val != "" {
		return val
	}
	return def
}

// parseLLMWorkers parses the PROCRASTINATOR_LLM_WORKERS value: a comma list of
// model[@baseURL] entries. Each entry becomes one worker with the extract
// strategy; a missing base URL falls back to defaultBaseURL.
func parseLLMWorkers(raw, defaultBaseURL string) ([]Worker, error) {
	parts := strings.Split(raw, ",")
	workers := make([]Worker, 0, len(parts))
	for _, p := range parts {
		entry := strings.TrimSpace(p)
		if entry == "" {
			return nil, fmt.Errorf("config: PROCRASTINATOR_LLM_WORKERS has an empty entry: %q", raw)
		}
		model, baseURL := entry, defaultBaseURL
		if i := strings.Index(entry, "@"); i >= 0 {
			model, baseURL = entry[:i], entry[i+1:]
		}
		if model == "" || baseURL == "" {
			return nil, fmt.Errorf("config: invalid PROCRASTINATOR_LLM_WORKERS entry %q (want model[@baseURL])", entry)
		}
		workers = append(workers, Worker{Model: model, BaseURL: baseURL, Strategy: "extract"})
	}
	return workers, nil
}
