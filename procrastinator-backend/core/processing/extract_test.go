package processing

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// validExtractionJSON is a valid extraction payload that parses cleanly.
const validExtractionJSON = `{"brand":"LG","model":"FHP10","serial_number":"SN123","name":"Washing Machine","confidence":0.9}`

// chatterCall records one Chat invocation for call assertions.
type chatterCall struct {
	ctx          context.Context
	systemPrompt string
	content      []ContentPart
}

// fakeChatter is an in-memory Chatter for tests. It records every call and
// returns whatever its behavior function (fn) yields, guarded by a mutex so
// it is safe for parallel use.
type fakeChatter struct {
	mu    sync.Mutex
	calls []chatterCall
	fn    func(ctx context.Context, systemPrompt string, content []ContentPart) (string, error)
}

// Compile-time guard: fakeChatter must satisfy Chatter.
var _ Chatter = (*fakeChatter)(nil)

// Chat records the call and delegates to the behavior function.
func (f *fakeChatter) Chat(ctx context.Context, systemPrompt string, content []ContentPart) (string, error) {
	f.mu.Lock()
	cp := make([]ContentPart, len(content))
	copy(cp, content)
	f.calls = append(f.calls, chatterCall{
		ctx:          ctx,
		systemPrompt: systemPrompt,
		content:      cp,
	})
	f.mu.Unlock()

	if f.fn == nil {
		return validExtractionJSON, nil
	}
	return f.fn(ctx, systemPrompt, content)
}

// count returns the number of recorded Chat calls.
func (f *fakeChatter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// call returns the call recorded at position i (0 = first).
func (f *fakeChatter) call(i int) chatterCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[i]
}

// contentParts is the content the extractor should present per worker: a text
// instruction plus one image_url data URI.
func contentParts(contentType string, docData []byte) []ContentPart {
	return []ContentPart{
		{Kind: "text", Text: "Extract all fields from this document."},
		{Kind: "image_url", ImageURL: "data:" + contentType + ";base64,"},
	}
}

func TestExtractRun_AllWorkersSucceed(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
		return validExtractionJSON, nil
	}}
	workers := []Worker{
		{Model: "model-a", BaseURL: "http://a.example", Strategy: Strategy{Name: "extract"}},
		{Model: "model-b", BaseURL: "http://b.example", Strategy: Strategy{Name: "verify"}},
	}
	ext := NewExtractor(fake, workers, time.Second, "BASE PROMPT")

	results := ext.Run(context.Background(), "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	if len(results) != len(workers) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(workers))
	}
	// Results must be in worker order.
	for i := range workers {
		if results[i].Worker.Model != workers[i].Model {
			t.Fatalf("results[%d].Worker.Model = %q, want %q", i, results[i].Worker.Model, workers[i].Model)
		}
	}
	if fake.count() != 2 {
		t.Fatalf("fake chat count = %d, want 2", fake.count())
	}
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("results[%d].Err = %v, want nil", i, r.Err)
		}
		if r.Extraction.Brand == nil || *r.Extraction.Brand != "LG" {
			t.Errorf("results[%d].Extraction.Brand = %v, want LG", i, r.Extraction.Brand)
		}
		if r.Extraction.Model == nil || *r.Extraction.Model != "FHP10" {
			t.Errorf("results[%d].Extraction.Model = %v, want FHP10", i, r.Extraction.Model)
		}
		if r.Extraction.SerialNumber == nil || *r.Extraction.SerialNumber != "SN123" {
			t.Errorf("results[%d].Extraction.SerialNumber = %v, want SN123", i, r.Extraction.SerialNumber)
		}
	}
}

func TestExtractRun_OneWorkerFails(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(_ context.Context, systemPrompt string, _ []ContentPart) (string, error) {
		if strings.Contains(systemPrompt, verifyNote) {
			return "", errors.New("model error on verify")
		}
		return validExtractionJSON, nil
	}}
	workers := []Worker{
		{Model: "model-a", BaseURL: "http://a.example", Strategy: Strategy{Name: "extract"}},
		{Model: "model-b", BaseURL: "http://b.example", Strategy: Strategy{Name: "verify"}},
	}
	ext := NewExtractor(fake, workers, time.Second, "BASE PROMPT")

	results := ext.Run(context.Background(), "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	// The verify worker (index 1) fails; the extract worker (index 0) succeeds.
	if results[0].Err != nil {
		t.Fatalf("results[0].Err = %v, want nil (extract worker)", results[0].Err)
	}
	if results[1].Err == nil {
		t.Fatalf("results[1].Err = nil, want non-nil (verify worker)")
	}
	// Run has no error return; the verify failure is recorded per-worker only.
	if results[1].Err == nil || !strings.Contains(results[1].Err.Error(), "model error on verify") {
		t.Fatalf("results[1].Err = %v, want it wrap the transport error", results[1].Err)
	}
}

func TestExtractRun_AllWorkersFail(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
		return "", errors.New("always down")
	}}
	workers := []Worker{
		{Model: "model-a", BaseURL: "http://a.example"},
		{Model: "model-b", BaseURL: "http://b.example"},
	}
	ext := NewExtractor(fake, workers, time.Second, "BASE PROMPT")

	results := ext.Run(context.Background(), "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, r := range results {
		if r.Err == nil {
			t.Fatalf("results[%d].Err = nil, want non-nil", i)
		}
		if r.Extraction.Brand != nil || r.Extraction.Model != nil || r.Extraction.SerialNumber != nil || r.Extraction.Classification != "" || len(r.Extraction.Metadata) != 0 {
			t.Errorf("results[%d].Extraction = %+v, want zero-valued", i, r.Extraction)
		}
	}
}

func TestExtractRun_ParseFailureRecorded(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
		return "no json here", nil
	}}
	workers := []Worker{
		{Model: "model-a", BaseURL: "http://a.example"},
	}
	ext := NewExtractor(fake, workers, time.Second, "BASE PROMPT")

	results := ext.Run(context.Background(), "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Fatal("results[0].Err = nil, want non-nil (parse failure)")
	}
	if !strings.Contains(results[0].Err.Error(), "parse") {
		t.Fatalf("results[0].Err = %v, want it wrapped with 'parse'", results[0].Err)
	}
}

func TestExtractRun_PerWorkerTimeout(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(ctx context.Context, _ string, _ []ContentPart) (string, error) {
		select {
		case <-time.After(300 * time.Millisecond):
			return validExtractionJSON, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}}
	workers := []Worker{
		{Model: "model-a", BaseURL: "http://a.example"},
		{Model: "model-b", BaseURL: "http://b.example"},
	}
	ext := NewExtractor(fake, workers, 50*time.Millisecond, "BASE PROMPT")

	start := time.Now()
	results := ext.Run(context.Background(), "image/png", []byte{0x89, 0x50, 0x4E, 0x47})
	elapsed := time.Since(start)

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, r := range results {
		if r.Err == nil {
			t.Fatalf("results[%d].Err = nil, want deadline exceeded", i)
		}
		if !errors.Is(r.Err, context.DeadlineExceeded) {
			t.Errorf("results[%d].Err = %v, want it wrap context.DeadlineExceeded", i, r.Err)
		}
	}
	_ = elapsed
}

func TestExtractRun_StrategyPrompts(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
		return validExtractionJSON, nil
	}}
	workers := []Worker{
		{Model: "model-a", BaseURL: "http://a.example", Strategy: Strategy{Name: "extract"}},
		{Model: "model-b", BaseURL: "http://b.example", Strategy: Strategy{Name: "verify"}},
	}
	ext := NewExtractor(fake, workers, time.Second, "BASE PROMPT")

	results := ext.Run(context.Background(), "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("result Err = %v, want nil", r.Err)
		}
	}

	// Worker order is not deterministic across parallel calls, so locate the
	// prompts by content rather than by recorded position.
	var baseSeen, verifySeen bool
	for i := 0; i < fake.count(); i++ {
		p := fake.call(i).systemPrompt
		if p == "BASE PROMPT" {
			baseSeen = true
		}
		if strings.HasPrefix(p, "BASE PROMPT") && strings.Contains(p, verifyNote) {
			verifySeen = true
		}
	}
	if !baseSeen {
		t.Errorf("no worker received exactly %q", "BASE PROMPT")
	}
	if !verifySeen {
		t.Errorf("no worker received a verify-prompt derived from %q", "BASE PROMPT")
	}
}

func TestExtractRun_OneRequestPerWorker(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
		return validExtractionJSON, nil
	}}
	workers := []Worker{
		{Model: "model-a", BaseURL: "http://a.example"},
		{Model: "model-b", BaseURL: "http://b.example"},
		{Model: "model-c", BaseURL: "http://c.example"},
	}
	ext := NewExtractor(fake, workers, time.Second, "BASE PROMPT")

	results := ext.Run(context.Background(), "image/png", []byte{0x89, 0x50, 0x4E, 0x47})

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if fake.count() != 3 {
		t.Fatalf("fake chat count = %d, want exactly 3 (one per worker)", fake.count())
	}
}

func TestExtractRun_ContentParts(t *testing.T) {
	t.Parallel()

	fake := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
		return validExtractionJSON, nil
	}}
	workers := []Worker{{Model: "model-a", BaseURL: "http://a.example"}}
	ext := NewExtractor(fake, workers, time.Second, "BASE PROMPT")

	docData := []byte{0x89, 0x50, 0x4E, 0x47}
	_ = ext.Run(context.Background(), "image/png", docData)

	if fake.count() != 1 {
		t.Fatalf("fake chat count = %d, want 1", fake.count())
	}
	call := fake.call(0)
	if len(call.content) != 2 {
		t.Fatalf("content len = %d, want 2", len(call.content))
	}
	if call.content[0].Kind != "text" || call.content[0].Text != "Extract all fields from this document." {
		t.Errorf("content[0] = %+v, want text part with extraction instruction", call.content[0])
	}
	if call.content[1].Kind != "image_url" {
		t.Fatalf("content[1].Kind = %q, want image_url", call.content[1].Kind)
	}
	if !strings.HasPrefix(call.content[1].ImageURL, "data:image/png;base64,") {
		t.Errorf("content[1].ImageURL = %q, want it to start with %q", call.content[1].ImageURL, "data:image/png;base64,")
	}
}
