package processing

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/parse"
)

// ContentPart is one piece of user content for a chat request: text or an
// image URL reference. It mirrors the wire format without coupling
// core/processing to infra/llm (the adapter converts between the two).
type ContentPart struct {
	Kind     string // "text" or "image_url"
	Text     string
	ImageURL string
}

// Chatter issues a single chat-completion request and returns the raw
// assistant content. The port is implemented by an adapter over
// infra/llm.Client at the composition root, so core/processing never
// imports infra (ARC-001).
type Chatter interface {
	Chat(ctx context.Context, systemPrompt string, content []ContentPart) (string, error)
}

// Strategy selects a prompting variant for an extraction worker.
type Strategy struct {
	// Name identifies the strategy, e.g. "extract" or "verify".
	// Unknown names use the default extraction prompt.
	Name string
}

// Worker is one extraction endpoint: a model at a base URL, run with a
// strategy. The worker list is a constructor parameter (config parsing is
// a composition concern, not a processing concern).
type Worker struct {
	Model    string
	BaseURL  string
	Strategy Strategy
}

// WorkerResult is the outcome of one worker: Extraction when Err == nil,
// otherwise Err records the transport or parse failure.
type WorkerResult struct {
	Worker     Worker
	Extraction entity.Extraction
	Err        error
}

// verifyNote is appended to the base extraction prompt for verify-strategy
// workers, instructing the model to independently re-derive identity fields
// and emit null rather than guess where uncertain.
const verifyNote = "Independently re-verify this document. Cross-check the identity fields (serial_number, brand, model) character by character against what is legible. Where uncertain, emit null rather than guess."

// Extractor runs a fixed set of extraction workers in parallel against a
// Chatter. Each worker issues exactly ONE chat-completion request with its
// own timeout; a worker's transport or parse failure is recorded in its
// WorkerResult, not returned as an error (per-worker failure is not fatal).
type Extractor struct {
	chatter      Chatter
	workers      []Worker
	timeout      time.Duration
	systemPrompt string
}

// NewExtractor creates an Extractor. timeout is the per-worker deadline;
// systemPrompt is the base extraction prompt (strategy variants are derived
// from it).
func NewExtractor(chatter Chatter, workers []Worker, timeout time.Duration, systemPrompt string) *Extractor {
	return &Extractor{
		chatter:      chatter,
		workers:      workers,
		timeout:      timeout,
		systemPrompt: systemPrompt,
	}
}

// Run executes all workers concurrently and returns one WorkerResult per
// worker, in the same order as the configured workers. It never returns an
// error: failures are recorded per worker.
func (e *Extractor) Run(ctx context.Context, contentType string, docData []byte) []WorkerResult {
	results := make([]WorkerResult, len(e.workers))

	// Plain Group (no derived context): each g.Go closure returns nil so one
	// worker's failure never cancels its siblings (per-worker failure is not
	// fatal); per-worker deadlines come from runWorker's own timeout context.
	var eg errgroup.Group

	for i := range e.workers {
		i := i
		w := e.workers[i]

		eg.Go(func() error {
			results[i] = e.runWorker(ctx, w, contentType, docData)
			return nil
		})
	}

	_ = eg.Wait()
	return results
}

// runWorker issues exactly one chat request for w, parses the response, and
// records the outcome in a WorkerResult.
func (e *Extractor) runWorker(ctx context.Context, w Worker, contentType string, docData []byte) WorkerResult {
	wctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	raw, err := e.chatter.Chat(wctx, e.systemPromptFor(w.Strategy), docContentParts(contentType, docData))
	if err != nil {
		return WorkerResult{
			Worker: w,
			Err:    fmt.Errorf("processing: worker %s: %w", w.Model, err),
		}
	}

	ext, err := parse.ParseExtraction(raw)
	if err != nil {
		return WorkerResult{
			Worker: w,
			Err:    fmt.Errorf("processing: worker %s: parse: %w", w.Model, err),
		}
	}

	return WorkerResult{
		Worker:     w,
		Extraction: ext,
	}
}

// systemPromptFor derives the prompt for s: the verify strategy appends
// verifyNote to the base prompt, everything else uses the base prompt.
func (e *Extractor) systemPromptFor(s Strategy) string {
	if s.Name == "verify" {
		return e.systemPrompt + "\n\n" + verifyNote
	}
	return e.systemPrompt
}

// docContentParts mirrors the wire format used by infra/llm.Extractor.Extract:
// a text instruction plus an image_url data URI carrying the document bytes.
func docContentParts(contentType string, docData []byte) []ContentPart {
	b64 := base64.StdEncoding.EncodeToString(docData)
	dataURI := fmt.Sprintf("data:%s;base64,%s", contentType, b64)

	return []ContentPart{
		{Kind: "text", Text: "Extract all fields from this document."},
		{Kind: "image_url", ImageURL: dataURI},
	}
}
