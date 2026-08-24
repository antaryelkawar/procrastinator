// Package llm — opt-in real-Gemini integration test for model gemma-4-26b-a4b-it.
//
// TestRealLLMIntegration exercises the real Client from client.go against the
// real generative endpoint, aligned with the API pinned by client_test.go. It
// is excluded from default runs (skips unless the gate env vars are set); see
// liveLLMConfig for the full gate.
//
// The fixture internal/llm/testdata/invoice-fixture.png is a rendered invoice
// image with legible text (no longer a redacted placeholder).
//
// The test proves the full chain:
//
//	HTTP → real endpoint → raw response (possibly containing <thought> blocks)
//	→ extraction.ParseExtraction stripping → valid Extraction
//	  (classification "invoice" + usable identity)
//
// Extraction.RawPayload in the parsed struct preserves the raw response
// verbatim (thought blocks intact if the model emitted any) — that is the
// stripping evidence.
package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"procrastinator-backend/internal/config"
	"procrastinator-backend/internal/llm/extraction"
)

// liveLLMConfig is the single point of env access for the live LLM tests.
// It skips the caller unless LM_TEST_LLM_INTEGRATION=1 AND LM_LLM_BASE_URL,
// LM_LLM_API_KEY, and LM_LLM_MODEL are all set. LM_LLM_TIMEOUT (optional) is
// parsed as a time.Duration; default is 60s; parse failure is a Fatalf.
// os.Getenv may not appear anywhere else in this file.
func liveLLMConfig(t *testing.T) config.Config {
	t.Helper()

	if os.Getenv("LM_TEST_LLM_INTEGRATION") != "1" {
		t.Skip("skipping: set LM_TEST_LLM_INTEGRATION=1 plus LM_LLM_BASE_URL/LM_LLM_API_KEY/LM_LLM_MODEL to enable")
	}

	baseURL := os.Getenv("LM_LLM_BASE_URL")
	apiKey := os.Getenv("LM_LLM_API_KEY")
	model := os.Getenv("LM_LLM_MODEL")
	if baseURL == "" {
		t.Skip("skipping: LM_LLM_BASE_URL is empty")
	}
	if apiKey == "" {
		t.Skip("skipping: LM_LLM_API_KEY is empty")
	}
	if model == "" {
		t.Skip("skipping: LM_LLM_MODEL is empty")
	}

	var timeout time.Duration
	if raw := os.Getenv("LM_LLM_TIMEOUT"); raw == "" {
		timeout = 60 * time.Second
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			t.Fatalf("failed to parse LM_LLM_TIMEOUT %q: %v", raw, err)
		}
		timeout = d
	}

	return config.Config{
		LLMBaseURL: baseURL,
		LLMAPIKey:  apiKey,
		LLMModel:   model,
		LLMTimeout: timeout,
	}
}

func TestRealLLMIntegration(t *testing.T) {
	cfg := liveLLMConfig(t)
	t.Parallel()
	client := New(cfg)

	fixture, err := os.ReadFile(filepath.Join("testdata", "invoice-fixture.png"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	raw, err := client.Extract(context.Background(), "image/png", fixture)
	if err != nil {
		t.Fatalf("extraction request failed: %v", err)
	}

	ex, err := extraction.ParseExtraction(raw)
	if err != nil {
		t.Fatalf("ParseExtraction failed: %v, raw: %s", err, string(raw))
	}

	if ex.Classification != "invoice" {
		t.Fatalf("expected classification 'invoice', got %q, raw: %s", ex.Classification, ex.RawPayload)
	}

	usable := ex.SerialNumber != "" || (ex.Brand != "" && ex.Model != "")
	if !usable {
		t.Fatalf("extraction yielded no usable identity: brand=%q, model=%q, serial=%q, raw: %s",
			ex.Brand, ex.Model, ex.SerialNumber, ex.RawPayload)
	}

	t.Logf("parsed Extraction: %+v", ex)
}
