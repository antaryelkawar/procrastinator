package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/identity"
)

// ErrTooLarge is returned when the upload exceeds the configured size limit.
var ErrTooLarge = errors.New("file too large")

// ErrNoTextStorage is returned when Process is called with Text input but no
// text storage has been configured (see Service.SetTextStorage).
var ErrNoTextStorage = errors.New("text storage not configured")

// ErrExtraction is returned when LLM extraction or parsing fails. Kept for
// parity with core/ingest; in the multi-worker pipeline per-worker failures
// are folded into OutcomeFailed rather than surfaced as this error.
var ErrExtraction = errors.New("extraction failed")

// Input is the single argument to Service.Process: the uploaded document and
// the scope it belongs to.
type Input struct {
	// Filename is the client-supplied filename (accepted for the API
	// contract; the storage derives the Source from the bytes).
	Filename string
	// Payload is the raw document bytes.
	Payload []byte
	// ContentType is the MIME type of the document (e.g. "application/pdf",
	// "image/jpeg").
	ContentType string
	// OwnerHouseholdID fences identity matching and stamps the source +
	// document to a household scope; nil means personal (no household).
	OwnerHouseholdID *string
	// Text is true for pasted free-text input; the service stores it via the
	// text storage (as a text/plain source) instead of the document storage.
	Text bool
	// Directive is the user's free-text note for this upload (design D7). It
	// is persisted as documents.payload.data.user_directive and appended to the
	// extraction prompt. Empty for note-less uploads.
	Directive string
}

// Service composes the standalone processing pipeline: dedupe, storage,
// multi-worker extraction with consensus, and the commit/hold decision. It is
// user-scoped: Process resolves the bound user from the context first and
// fails closed when it is absent.
type Service struct {
	factory        *repo.Factory
	extractor      *Extractor
	storage        repo.FileStorage
	textStorage    repo.FileStorage
	maxBytes       int64
	threshold      float64
	reviewer       repo.Reviewer
	retentionDays  int
	candidateLimit int
	processTimeout time.Duration
}

// New constructs a Service. extractor is the already-built *Extractor that
// encapsulates the extraction workers, chatter, per-worker timeout, and
// system prompt. maxBytes is the maximum accepted upload size in bytes
// (uploads strictly larger are rejected with ErrTooLarge); threshold is the
// minimum consensus confidence required for auto-commit; retentionDays is the
// source retention window; candidateLimit bounds the identity candidate set
// per stage; processTimeout is the end-to-end budget for the LLM extraction +
// decision work. Defaults are the caller's (config) responsibility.
func New(factory *repo.Factory, extractor *Extractor, storage repo.FileStorage, maxBytes int64, threshold float64, reviewer repo.Reviewer, retentionDays int, candidateLimit int, processTimeout time.Duration) *Service {
	return &Service{
		factory:        factory,
		extractor:      extractor,
		storage:        storage,
		maxBytes:       maxBytes,
		threshold:      threshold,
		reviewer:       reviewer,
		retentionDays:  retentionDays,
		candidateLimit: candidateLimit,
		processTimeout: processTimeout,
	}
}

// SetTextStorage sets the storage used for pasted-text inputs (Input.Text).
// When nil, a text input fails with ErrNoTextStorage.
func (s *Service) SetTextStorage(ts repo.FileStorage) { s.textStorage = ts }

// StoreSource stores the uploaded bytes via the configured FileStorage and
// persists the Source row. It is used by the add handler for duplicate uploads
// where the full pipeline is not run. Returns the persisted Source.
func (s *Service) StoreSource(ctx context.Context, in Input) (entity.Source, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Source{}, err
	}
	var source entity.Source
	if in.Text {
		if s.textStorage == nil {
			return entity.Source{}, ErrNoTextStorage
		}
		source, err = s.textStorage.Put(ctx, in.Payload)
	} else {
		source, err = s.storage.Put(ctx, in.Payload)
	}
	if err != nil {
		return entity.Source{}, err
	}
	source.OwnerHouseholdID = in.OwnerHouseholdID
	source, err = s.factory.Sources.Create(ctx, source, repo.Owner(tid))
	return source, err
}

// Process runs the processing pipeline for one upload and returns a
// discriminated Outcome:
//
//  1. resolve the bound user (fail closed);
//  2. reject oversize payloads with ErrTooLarge;
//  3. dedupe by content hash (pre-storage, no LLM) → OutcomeDuplicate;
//  4. store the bytes and persist the Source row (pre-tx, survives later
//     failures);
//  5. run the multi-worker extractor under an end-to-end budget and compute
//     consensus;
//  6. on budget overrun → hold with the partial provenance;
//  7. when every worker failed → OutcomeFailed (the source row is retained);
//  8. when the classification is "statement" → OutcomeStatement;
//  9. normalize warranty end and infer the asset category;
//  10. commit (resolve identity + create the document in one transaction) when
//     the consensus confidence meets the threshold, else hold (including the
//     ambiguous / soft-deleted cases that Resolve reports).
func (s *Service) Process(ctx context.Context, in Input) (Outcome, error) {
	// 1. Resolve the bound user; fail closed when absent.
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return Outcome{}, err
	}

	// 2. Size check.
	if int64(len(in.Payload)) > s.maxBytes {
		return Outcome{}, ErrTooLarge
	}

	// 3. Dedupe (pre-storage, no LLM).
	sum := sha256.Sum256(in.Payload)
	hash := hex.EncodeToString(sum[:])
	dup, ok, err := DedupeSource(ctx, s.factory.Sources, s.factory.Documents, s.factory.Assets, hash)
	if err != nil {
		return Outcome{}, err
	}
	if ok {
		return dup, nil
	}

	// 4. Storage + source row (pre-tx, survives later failures). Pasted text
	// goes to the text storage (text/plain source); documents to the document
	// storage.
	var source entity.Source
	if in.Text {
		if s.textStorage == nil {
			return Outcome{}, ErrNoTextStorage
		}
		source, err = s.textStorage.Put(ctx, in.Payload)
	} else {
		source, err = s.storage.Put(ctx, in.Payload)
	}
	if err != nil {
		return Outcome{}, err
	}
	source.OwnerHouseholdID = in.OwnerHouseholdID
	source, err = s.factory.Sources.Create(ctx, source, repo.Owner(tid))
	if err != nil {
		return Outcome{}, err
	}

	// 5. End-to-end budget for the LLM extraction + decision work.
	pctx, cancel := context.WithTimeout(ctx, s.processTimeout)
	defer cancel()

	// 6. Extraction + consensus. The user's directive (if any) is appended to
	// the extraction prompt so the model can honor the note.
	results := s.extractor.Run(pctx, in.ContentType, in.Payload, in.Directive)
	ext, conf, unresolved := Consensus(results)
	provenance := buildProvenance(results, unresolved, nil)

	// 7. Budget overrun (checked before the all-failed check): hold with the
	// partial provenance, using the original ctx since pctx is already done.
	if errors.Is(pctx.Err(), context.DeadlineExceeded) {
		rev, err := s.reviewer.Hold(ctx, repo.HoldInput{
			Extraction:       ext,
			SourceID:         source.ID,
			OwnerHouseholdID: in.OwnerHouseholdID,
			Provenance:       provenance,
		})
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{
			Kind:       OutcomeHeldForReview,
			Review:     &rev,
			Confidence: ptrConf(conf),
			Provenance: provenance,
		}, nil
	}

	// 8. All workers failed (no overrun): the source row from step 4 is
	// intentionally retained.
	if allFailed(results) {
		return Outcome{
			Kind:       OutcomeFailed,
			Reason:     "all extraction workers failed",
			Provenance: provenance,
		}, nil
	}

	// 9. Statement classification: route to the ledger import pipeline.
	if ext.Classification == "statement" {
		return Outcome{
			Kind:       OutcomeStatement,
			Statement:  &source,
			Confidence: ptrConf(conf),
			Provenance: provenance,
		}, nil
	}

	// 10. Warranty: derive the end date from purchase + duration (explicit end
	// wins), or keep the existing value.
	if end, ok := commons.AddWarrantyEnd(ext.PurchaseDate, ext.WarrantyDuration, ext.WarrantyEnd); ok {
		ext.WarrantyEnd = end
	}

	// 11. Category: infer the asset category + confidence.
	cat, catConf := InferCategory(derefStr(ext.Name), derefStr(ext.Brand), "", ext.AssetCategory)
	ext.AssetCategory = &cat
	ext.CategoryConfidence = &catConf

	// 12. Identity lookup + commit/hold decision.
	confMet := ext.Confidence != nil && *ext.Confidence >= s.threshold
	if confMet {
		// Commit path: resolve identity + create the document in one transaction.
		var asset entity.Asset
		err = s.factory.InTx(ctx, func(ctx context.Context, repos *repo.Repos) error {
			a, _, err := identity.Resolve(ctx, repos.Assets, ext, in.OwnerHouseholdID, s.candidateLimit)
			if err != nil {
				return err
			}
			doc := entity.Document{
				SourceID:         source.ID,
				AssetID:          a.ID,
				DocType:          ext.Classification,
				ExtractedFields:  extractionFields(ext),
				RawExtraction:    ext.RawPayload,
				OwnerHouseholdID: in.OwnerHouseholdID,
				Confidence:       ext.Confidence,
				UserDirective:    in.Directive,
			}
			if _, err = repos.Documents.Create(ctx, doc, repo.Owner(tid)); err != nil {
				return err
			}
			asset = a
			return nil
		})
		switch {
		case err == nil:
			return Outcome{
				Kind:       OutcomeCommitted,
				Asset:      &asset,
				Confidence: ext.Confidence,
				Provenance: provenance,
			}, nil
		case errors.Is(err, identity.ErrAmbiguous), errors.Is(err, identity.ErrSoftDeleted):
			// Ambiguous / soft-deleted are held, not errors; fall through to the
			// hold path below.
		default:
			// Any other error rolled the transaction back; surface it.
			return Outcome{}, err
		}
	}

	// Hold path (reached when confMet is false, or the commit path hit
	// ErrAmbiguous / ErrSoftDeleted): obtain the candidate set read-only.
	m, merr := identity.Match(ctx, s.factory.Assets, ext, in.OwnerHouseholdID, s.candidateLimit)
	if merr != nil {
		return Outcome{}, merr
	}
	provenance = buildProvenance(results, unresolved, m.Candidates)
	rev, err := s.reviewer.Hold(ctx, repo.HoldInput{
		Extraction:       ext,
		SourceID:         source.ID,
		OwnerHouseholdID: in.OwnerHouseholdID,
		Provenance:       provenance,
	})
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Kind:       OutcomeHeldForReview,
		Review:     &rev,
		Confidence: ext.Confidence,
		Provenance: provenance,
	}, nil
}

// Reprocess re-runs the extraction pipeline for an existing source. It reads
// the source bytes back from storage, runs the extractor with the supplied
// directive, computes consensus, and returns a discriminated Outcome. It does
// NOT re-store the source or re-dedupe (the source row already exists); the
// caller (the documents reprocess handler) persists the document outcome.
//
// Unlike Process, Reprocess does not create a document row itself: on the
// commit path it only resolves identity and returns the target asset so the
// caller can point the existing document at it.
func (s *Service) Reprocess(ctx context.Context, sourceID string, directive string, ownerHH *string) (Outcome, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return Outcome{}, err
	}

	// Look up the source row to get the storage path and content type.
	sources, err := s.factory.Sources.List(ctx, repo.Owner(tid), repo.Where("id", "=", sourceID))
	if err != nil {
		return Outcome{}, err
	}
	if len(sources) == 0 {
		return Outcome{}, errors.New("source not found")
	}
	source := sources[0]

	// Read the stored bytes back from storage.
	payload, err := s.storage.Get(ctx, source.Path)
	if err != nil {
		return Outcome{}, err
	}

	// End-to-end budget for the LLM extraction + decision work.
	pctx, cancel := context.WithTimeout(ctx, s.processTimeout)
	defer cancel()

	// Extraction + consensus with the (possibly new) directive.
	results := s.extractor.Run(pctx, source.ContentType, payload, directive)
	ext, conf, unresolved := Consensus(results)
	provenance := buildProvenance(results, unresolved, nil)

	// Budget overrun: hold with the partial provenance (original ctx, pctx done).
	if errors.Is(pctx.Err(), context.DeadlineExceeded) {
		rev, err := s.reviewer.Hold(ctx, repo.HoldInput{
			Extraction:       ext,
			SourceID:         source.ID,
			OwnerHouseholdID: ownerHH,
			Provenance:       provenance,
		})
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{Kind: OutcomeHeldForReview, Review: &rev, Confidence: ptrConf(conf), Provenance: provenance}, nil
	}

	// All workers failed.
	if allFailed(results) {
		return Outcome{Kind: OutcomeFailed, Reason: "all extraction workers failed", Provenance: provenance}, nil
	}

	// Statement classification.
	if ext.Classification == "statement" {
		return Outcome{Kind: OutcomeStatement, Statement: &source, Confidence: ptrConf(conf), Provenance: provenance}, nil
	}

	// Warranty end + category inference.
	if end, ok := commons.AddWarrantyEnd(ext.PurchaseDate, ext.WarrantyDuration, ext.WarrantyEnd); ok {
		ext.WarrantyEnd = end
	}
	cat, catConf := InferCategory(derefStr(ext.Name), derefStr(ext.Brand), "", ext.AssetCategory)
	ext.AssetCategory = &cat
	ext.CategoryConfidence = &catConf

	// Identity resolution + commit/hold decision.
	confMet := ext.Confidence != nil && *ext.Confidence >= s.threshold
	if confMet {
		// Commit path: resolve identity (merge into or create the target asset).
		// No document is created here — the caller owns the existing row.
		var asset entity.Asset
		err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
			a, _, err := identity.Resolve(ctx, r.Assets, ext, ownerHH, s.candidateLimit)
			if err != nil {
				return err
			}
			asset = a
			return nil
		})
		switch {
		case err == nil:
			return Outcome{Kind: OutcomeCommitted, Asset: &asset, Confidence: ext.Confidence, Provenance: provenance}, nil
		case errors.Is(err, identity.ErrAmbiguous), errors.Is(err, identity.ErrSoftDeleted):
			// Fall through to the hold path.
		default:
			return Outcome{}, err
		}
	}

	// Hold path (below threshold, or commit path hit ErrAmbiguous / ErrSoftDeleted).
	m, merr := identity.Match(ctx, s.factory.Assets, ext, ownerHH, s.candidateLimit)
	if merr != nil {
		return Outcome{}, merr
	}
	provenance = buildProvenance(results, unresolved, m.Candidates)
	rev, err := s.reviewer.Hold(ctx, repo.HoldInput{
		Extraction:       ext,
		SourceID:         source.ID,
		OwnerHouseholdID: ownerHH,
		Provenance:       provenance,
	})
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Kind: OutcomeHeldForReview, Review: &rev, Confidence: ext.Confidence, Provenance: provenance}, nil
}

// ptrConf returns a pointer to c.
func ptrConf(c float64) *float64 { return &c }

// derefStr dereferences p to a string; nil yields "".
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// allFailed reports whether no worker succeeded: true when results is empty or
// every WorkerResult carries a non-nil Err.
func allFailed(results []WorkerResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, r := range results {
		if r.Err == nil {
			return false
		}
	}
	return true
}

// buildProvenance assembles the Provenance map for an outcome: per-worker
// results (in order), the unresolved identity fields when any, and the
// candidate set when non-empty.
func buildProvenance(results []WorkerResult, unresolved []string, candidates []entity.Asset) Provenance {
	workers := make([]any, 0, len(results))
	for _, wr := range results {
		entry := map[string]any{"worker": wr.Worker.Model}
		if wr.Err != nil {
			entry["error"] = wr.Err.Error()
		} else {
			entry["extraction"] = wr.Extraction
		}
		workers = append(workers, entry)
	}

	prov := Provenance{"workers": workers}
	if len(unresolved) > 0 {
		prov["unresolved"] = unresolved
	}
	if len(candidates) > 0 {
		prov["candidates"] = candidates
	}
	return prov
}

// extractionFields flattens the extraction into a non-nil map containing a
// key only for each present value. Mirrors ingest.extractionFields so the
// stored document shape is identical across the standalone and legacy
// pipelines.
func extractionFields(ext entity.Extraction) map[string]any {
	fields := make(map[string]any, 9)
	if ext.Classification != "" {
		fields["classification"] = ext.Classification
	}
	if ext.Brand != nil {
		fields["brand"] = *ext.Brand
	}
	if ext.Model != nil {
		fields["model"] = *ext.Model
	}
	if ext.SerialNumber != nil {
		fields["serial_number"] = *ext.SerialNumber
	}
	if ext.Price != nil {
		fields["price"] = *ext.Price
	}
	if ext.Currency != nil {
		fields["currency"] = *ext.Currency
	}
	if ext.PurchaseDate != nil {
		fields["purchase_date"] = ext.PurchaseDate.Format(time.DateOnly)
	}
	if ext.WarrantyEnd != nil {
		fields["warranty_end"] = ext.WarrantyEnd.Format(time.DateOnly)
	}
	if ext.Metadata != nil {
		fields["metadata"] = ext.Metadata
	}
	return fields
}
