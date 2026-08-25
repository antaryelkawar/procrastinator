package llm

import (
	"context"
	"os"
	"testing"
	"time"

	"procrastinator-backend/commons/parse"
)

// liveLLMConfig is the ONLY env access in this file. It gates the real LLM
// integration tests: they skip unless PROCRASTINATOR_TEST_LLM_INTEGRATION=1
// is set, then require live credentials.
func liveLLMConfig(t *testing.T) (baseURL, apiKey, model string, timeout time.Duration) {
	t.Helper()
	if os.Getenv("PROCRASTINATOR_TEST_LLM_INTEGRATION") != "1" {
		t.Skip("set PROCRASTINATOR_TEST_LLM_INTEGRATION=1 to run real LLM tests")
	}
	baseURL = os.Getenv("PROCRASTINATOR_LLM_BASE_URL")
	apiKey = os.Getenv("PROCRASTINATOR_LLM_API_KEY")
	model = os.Getenv("PROCRASTINATOR_LLM_MODEL")
	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("PROCRASTINATOR_LLM_BASE_URL, PROCRASTINATOR_LLM_API_KEY, PROCRASTINATOR_LLM_MODEL required")
	}
	timeout = 120 * time.Second // LLM calls can be slow
	return baseURL, apiKey, model, timeout
}

// TestRealLLM_LGInvoice runs a live LLM extraction against the LG/AMC invoice
// PDF and asserts the core identity fields come back.
func TestRealLLM_LGInvoice(t *testing.T) {
	baseURL, apiKey, model, timeout := liveLLMConfig(t)
	t.Parallel()

	pdfData, err := os.ReadFile("../../../sample-data/INVNAG2302754.pdf")
	if err != nil {
		t.Fatalf("read PDF: %v", err)
	}

	client := New(baseURL, apiKey, model, timeout)
	ext := NewExtractor(client)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	raw, err := ext.Extract(ctx, "application/pdf", pdfData)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	parsed, err := parse.ParseExtraction(raw)
	if err != nil {
		t.Fatalf("parse extraction: %v", err)
	}
	t.Logf("extraction: %+v", parsed)

	if parsed.Classification != "invoice" && parsed.Classification != "amc" {
		t.Errorf("classification = %q, want invoice or amc", parsed.Classification)
	}
	if parsed.SerialNumber == nil || *parsed.SerialNumber == "" {
		t.Errorf("serial number is nil or empty, want non-empty (e.g. 012PFPM00313)")
	}
	if parsed.Brand == nil || *parsed.Brand == "" {
		t.Errorf("brand is nil or empty, want non-empty (e.g. LG)")
	}
	if parsed.Price == nil || *parsed.Price == "" {
		t.Errorf("price is nil or empty, want non-empty")
	}
	if len(parsed.Metadata) == 0 {
		t.Errorf("metadata is empty, want at least one amc-related field")
	}
}

// TestRealLLM_IFBAMC runs a live LLM extraction against the IFB AMC PDF and
// asserts the core identity fields come back.
func TestRealLLM_IFBAMC(t *testing.T) {
	baseURL, apiKey, model, timeout := liveLLMConfig(t)
	t.Parallel()

	pdfData, err := os.ReadFile("../../../sample-data/Annual Maintenance Contract.pdf")
	if err != nil {
		t.Fatalf("read PDF: %v", err)
	}

	client := New(baseURL, apiKey, model, timeout)
	ext := NewExtractor(client)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	raw, err := ext.Extract(ctx, "application/pdf", pdfData)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	parsed, err := parse.ParseExtraction(raw)
	if err != nil {
		t.Fatalf("parse extraction: %v", err)
	}
	t.Logf("extraction: %+v", parsed)

	if parsed.Classification != "amc" && parsed.Classification != "warranty" {
		t.Errorf("classification = %q, want amc or warranty", parsed.Classification)
	}
	if parsed.SerialNumber == nil || *parsed.SerialNumber == "" {
		t.Errorf("serial number is nil or empty, want non-empty (e.g. 005220171015002361)")
	}
	if parsed.Brand == nil || *parsed.Brand == "" {
		t.Errorf("brand is nil or empty, want non-empty (e.g. IFB)")
	}
}
