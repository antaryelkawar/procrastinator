package statement

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// Error sentinels returned by the Service methods. They map to HTTP status
// codes in the API layer (design D11):
//
//	ErrTooLarge         -> 413 Request Entity Too Large
//	ErrUnsupportedType  -> 415 Unsupported Media Type
//	ErrNoLines          -> 422 Unprocessable Entity
//	ErrTooManyLines     -> 422 Unprocessable Entity
//	ErrConflict         -> 409 Conflict
var (
	// ErrTooLarge is returned when the uploaded data exceeds the configured
	// size limit. The store is never called for such uploads.
	ErrTooLarge = errors.New("statement upload too large")

	// ErrUnsupportedType is returned when the statement store rejects the
	// upload content (the store is the sniffer: its rejection means the
	// content is neither CSV nor PDF) or returns a Source whose ContentType
	// is neither text/csv nor application/pdf.
	ErrUnsupportedType = errors.New("unsupported statement type")

	// ErrNoLines is returned when the statement contains no parseable lines.
	// The Source is already retained (file + Source row).
	ErrNoLines = errors.New("statement has no lines")

	// ErrTooManyLines is returned when the statement contains more lines
	// than the configured limit. The Source is already retained (file +
	// Source row).
	ErrTooManyLines = errors.New("statement has too many lines")

	// ErrConflict is returned when a batch state transition is not allowed
	// by the state machine (e.g. committing a discarded batch, discarding a
	// committed batch, or operating on an unrecognized state).
	ErrConflict = errors.New("batch state conflict")
)

// CommitSummary is the outcome of a Commit call. Created is the number of
// movements created (or already created on an idempotent re-commit); Skipped
// is the number of lines that produced no movement (duplicate,
// possible-duplicate, or error lines).
type CommitSummary struct {
	Created int
	Skipped int
}

// StatementSourceStore stores an uploaded statement and returns the resulting
// Source record. The concrete implementation (infra/filestorage) sniffs the
// content type: CSV or PDF is accepted, anything else is an error.
type StatementSourceStore interface {
	Store(ctx context.Context, originalName string, data []byte) (entity.Source, error)
}

// MovementsForAccountLister lists existing ledger movements for one account
// (user-scoped by the caller via opts), used for duplicate detection during
// Upload.
type MovementsForAccountLister interface {
	MovementsForAccount(ctx context.Context, accountID string, opts ...repo.Option) ([]entity.MoneyMovement, error)
}

// LinkCandidateLister lists documents that could be auto-linked to a movement
// by exact amount and currency (user-scoped by the caller via opts). A
// document already linked to any movement is not a candidate.
type LinkCandidateLister interface {
	LinkCandidates(ctx context.Context, amount, currency string, opts ...repo.Option) ([]entity.Document, error)
}

// Service implements the import batch lifecycle (design D6/D8/D10/D11):
// Upload (size check, store, account check, parse, classify, atomic
// batch+lines persistence in preview state), Commit (atomic all-or-none
// movement creation for valid lines + batch transition + idempotent
// auto-link), Discard, and read/list operations.
//
// User identity is fail-closed: the user ID is resolved once from the context
// per call (user.ErrNoUser before any query when absent) and repo.Owner(tid)
// is applied to every repository call.
type Service struct {
	factory   *repo.Factory
	src       StatementSourceStore
	movs      MovementsForAccountLister
	linkCands LinkCandidateLister
	pdf       PDFTextExtractor
	maxBytes  int64
	maxLines  int
}

// New constructs a Service. maxBytes is the maximum accepted upload size in
// bytes (uploads strictly larger are rejected with ErrTooLarge); maxLines is
// the maximum number of statement lines (strictly more are rejected with
// ErrTooManyLines).
func New(factory *repo.Factory, src StatementSourceStore, movs MovementsForAccountLister, linkCands LinkCandidateLister, pdf PDFTextExtractor, maxBytes int64, maxLines int) *Service {
	return &Service{
		factory:   factory,
		src:       src,
		movs:      movs,
		linkCands: linkCands,
		pdf:       pdf,
		maxBytes:  maxBytes,
		maxLines:  maxLines,
	}
}

// Upload runs the statement import pipeline for one upload:
//
//  1. Resolve the user from ctx (fail-closed: no query runs without it).
//  2. Reject data larger than maxBytes with ErrTooLarge (the store is never
//     called).
//  3. Store the bytes via the statement store, then persist the Source as a
//     row (pool-bound) before parsing. A store error means the content is
//     not CSV/PDF and is mapped to ErrUnsupportedType; a returned Source
//     whose ContentType is neither text/csv nor application/pdf is also
//     ErrUnsupportedType. The Source (file + row) is retained even on later
//     failure.
//  4. Load the account (repo.ErrNotFound propagates; the Source is retained).
//  5. Parse the statement into raw lines (CSV rows or trimmed non-empty PDF
//     text lines); zero lines is ErrNoLines.
//  6. Reject more than maxLines lines with ErrTooManyLines (Source retained).
//  7. Extract fields from each raw line (1-based line refs).
//  8. Load the existing movements for the account (user-scoped).
//  9. Classify the lines against the existing movements (pure, deterministic).
//  10. Atomically create the batch (state preview, per-status counts) and all
//     its lines in a single transaction.
//
// It returns the created batch (with repo-assigned ID) and its lines.
func (s *Service) Upload(ctx context.Context, accountID, filename string, data []byte) (entity.ImportBatch, []entity.ImportLine, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}

	// 2. Size check before the store: the store is never called.
	if int64(len(data)) > s.maxBytes {
		return entity.ImportBatch{}, nil, ErrTooLarge
	}

	// 3. Store + sniff. The store is the content sniffer: its rejection means
	// non-CSV/PDF content, so its error maps to ErrUnsupportedType.
	source, err := s.src.Store(ctx, filename, data)
	if err != nil {
		return entity.ImportBatch{}, nil, fmt.Errorf("%w: %v", ErrUnsupportedType, err)
	}
	if source.ContentType != "text/csv" && source.ContentType != "application/pdf" {
		return entity.ImportBatch{}, nil, fmt.Errorf("%w: %s", ErrUnsupportedType, source.ContentType)
	}

	// 3b. Persist the Source row (retained even on later failure). The repo
	// assigns the id via gen_random_uuid() (the store's client-side id is
	// dropped on insert), so capture the returned row: the batch FK must
	// reference the committed source id, not the store's provisional one.
	source, err = s.factory.Sources.Create(ctx, source, repo.Owner(tid))
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}

	// 4. Account check (ErrNotFound propagates; the Source is retained). The
	// account is loaded here (and again inside the commit transaction) so
	// that a missing account fails fast before any batch is persisted.
	if _, err := s.factory.Accounts.Get(ctx, accountID, repo.Owner(tid)); err != nil {
		return entity.ImportBatch{}, nil, err
	}

	// 5. Parse.
	var format StatementFormat
	var raws []string
	switch source.ContentType {
	case "text/csv":
		format = FormatCSV
		raws, err = ParseCSV(data)
		if err != nil {
			return entity.ImportBatch{}, nil, fmt.Errorf("upload statement: %w", err)
		}
	default: // application/pdf
		format = FormatPDF
		text, err := s.pdf.ExtractText(data)
		if err != nil {
			return entity.ImportBatch{}, nil, fmt.Errorf("upload statement: %w", err)
		}
		raws = pdfTextLines(text)
	}
	if len(raws) == 0 {
		return entity.ImportBatch{}, nil, ErrNoLines
	}

	// 6. Line bound (Source retained, no batch).
	if len(raws) > s.maxLines {
		return entity.ImportBatch{}, nil, ErrTooManyLines
	}

	// 7. Field extraction (1-based line refs).
	lines := make([]ParsedLine, len(raws))
	for i, raw := range raws {
		lines[i] = ExtractFields(i+1, raw)
	}

	// 8. Existing movements for the account (user-scoped).
	existing, err := s.movs.MovementsForAccount(ctx, accountID, repo.Owner(tid))
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}

	// 9. Deterministic classification.
	classified := Classify(lines, existing)

	// 10. Atomic batch + lines in preview state.
	var createdBatch entity.ImportBatch
	err = s.factory.InTx(ctx, func(ctx context.Context, repos *repo.Repos) error {
		batch := entity.ImportBatch{
			State:                entity.BatchStatePreview,
			AccountID:            accountID,
			SourceID:             source.ID,
			Filename:             filename,
			Format:               string(format),
			LineCountValid:       countStatus(classified, entity.LineStatusValid),
			LineCountDuplicate:   countStatus(classified, entity.LineStatusDuplicate),
			LineCountPossibleDup: countStatus(classified, entity.LineStatusPossibleDuplicate),
			LineCountError:       countStatus(classified, entity.LineStatusError),
		}
		created, err := repos.ImportBatches.Create(ctx, batch, repo.Owner(tid))
		if err != nil {
			return err
		}
		createdBatch = created

		importLines := make([]entity.ImportLine, len(classified))
		for i, l := range classified {
			importLines[i] = entity.ImportLine{
				BatchID:           created.ID,
				LineRef:           l.LineRef,
				RawLine:           l.RawLine,
				OccurredOn:        l.OccurredOn,
				Amount:            l.Amount,
				Direction:         &l.Direction,
				Description:       l.Description,
				NormDescription:   l.NormDescription,
				ExternalReference: l.ExternalReference,
				Status:            l.Status,
				ErrorReason:       l.ErrorReason,
			}
		}
		for i := range importLines {
			if _, err := repos.ImportLines.Create(ctx, importLines[i], repo.Owner(tid)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}

	// 11. Return the created (repo-assigned-ID) batch and lines.
	createdLines, err := s.factory.ImportLines.List(ctx, repo.Owner(tid), repo.Where("batch_id", "=", createdBatch.ID), repo.OrderBy("line_ref"))
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}
	return createdBatch, createdLines, nil
}

// Commit atomically commits a preview batch: every valid line (in line_ref
// order) becomes a ledger movement (expense for out / income for in, currency
// from the account), the batch transitions to committed, and idempotent
// auto-links are applied after the transaction. Any failure rolls back the
// whole transaction: no movements are created and the batch stays in preview.
//
// Committing an already-committed batch is idempotent: it returns the summary
// recomputed from the persisted movements and batch counts without creating
// anything (and re-applies the idempotent auto-links). Committing a
// discarded batch returns ErrConflict; any unrecognized state is treated
// defensively as ErrConflict.
func (s *Service) Commit(ctx context.Context, batchID string) (CommitSummary, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return CommitSummary{}, err
	}

	batch, err := s.factory.ImportBatches.Get(ctx, batchID, repo.Owner(tid))
	if err != nil {
		return CommitSummary{}, err
	}

	switch batch.State {
	case entity.BatchStateDiscarded:
		return CommitSummary{}, ErrConflict

	case entity.BatchStateCommitted:
		// Idempotent re-commit: summary recomputed from the persisted
		// movements and the batch's line counts.
		movs, err := s.factory.Movements.List(ctx, repo.Owner(tid), repo.Where("import_batch_id", "=", batchID))
		if err != nil {
			return CommitSummary{}, err
		}
		summary := CommitSummary{
			Created: len(movs),
			Skipped: batch.LineCountDuplicate + batch.LineCountPossibleDup + batch.LineCountError,
		}
		if err := s.applyAutoLinks(ctx, tid, batch); err != nil {
			return CommitSummary{}, err
		}
		return summary, nil

	case entity.BatchStatePreview:
		var validCount int
		err := s.factory.InTx(ctx, func(ctx context.Context, repos *repo.Repos) error {
			lines, err := repos.ImportLines.List(ctx, repo.Owner(tid), repo.Where("batch_id", "=", batchID), repo.OrderBy("line_ref"))
			if err != nil {
				return err
			}
			acct, err := repos.Accounts.Get(ctx, batch.AccountID, repo.Owner(tid))
			if err != nil {
				return err
			}

			// Create a movement for every valid line, in line_ref order.
			for _, line := range lines {
				if line.Status != entity.LineStatusValid {
					continue
				}
				if line.Amount == nil || line.OccurredOn == nil || line.Description == nil || line.NormDescription == nil {
					// Defensive: a valid line always has these fields set by
					// ExtractFields; treat a violation as a failed commit.
					return fmt.Errorf("commit batch %s: valid line %d has missing required fields", batchID, line.LineRef)
				}
				kind := entity.KindIncome
				if line.Direction != nil && *line.Direction == DirectionOut {
					kind = entity.KindExpense
				}
				mv := entity.MoneyMovement{
					Kind:              kind,
					Amount:            *line.Amount,
					Currency:          acct.Currency,
					OccurredOn:        *line.OccurredOn,
					Description:       *line.Description,
					NormDescription:   *line.NormDescription,
					Origin:            entity.OriginImport,
					ImportBatchID:     &batch.ID,
					ImportLine:        &line.LineRef,
					ExternalReference: line.ExternalReference,
				}
				if kind == entity.KindExpense {
					mv.SourceAccountID = &acct.ID
				} else {
					mv.DestinationAccountID = &acct.ID
				}
				if _, err := repos.Movements.Create(ctx, mv, repo.Owner(tid)); err != nil {
					return err
				}
				validCount++
			}

			// Transition the batch to committed with recomputed counts.
			committed := batch
			committed.State = entity.BatchStateCommitted
			committed.LineCountValid = countImportStatus(lines, entity.LineStatusValid)
			committed.LineCountDuplicate = countImportStatus(lines, entity.LineStatusDuplicate)
			committed.LineCountPossibleDup = countImportStatus(lines, entity.LineStatusPossibleDuplicate)
			committed.LineCountError = countImportStatus(lines, entity.LineStatusError)
			if _, err := repos.ImportBatches.Update(ctx, committed, repo.Owner(tid)); err != nil {
				return err
			}
			batch = committed
			return nil
		})
		if err != nil {
			return CommitSummary{}, err
		}

		// Idempotent auto-links, pool-bound after the transaction.
		if err := s.applyAutoLinks(ctx, tid, batch); err != nil {
			return CommitSummary{}, err
		}

		skipped := batch.LineCountDuplicate + batch.LineCountPossibleDup + batch.LineCountError
		return CommitSummary{Created: validCount, Skipped: skipped}, nil

	default:
		// Defensive: no other state is recognized by the state machine.
		return CommitSummary{}, ErrConflict
	}
}

// Discard transitions a preview batch to the discarded (terminal) state.
// Discarding an already-discarded batch is idempotent and returns the batch
// unchanged. Discarding a committed batch returns ErrConflict.
func (s *Service) Discard(ctx context.Context, batchID string) (entity.ImportBatch, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.ImportBatch{}, err
	}

	batch, err := s.factory.ImportBatches.Get(ctx, batchID, repo.Owner(tid))
	if err != nil {
		return entity.ImportBatch{}, err
	}

	switch batch.State {
	case entity.BatchStatePreview:
		// Pool-bound (no tx): a single state transition.
		batch.State = entity.BatchStateDiscarded
		updated, err := s.factory.ImportBatches.Update(ctx, batch, repo.Owner(tid))
		if err != nil {
			return entity.ImportBatch{}, err
		}
		return updated, nil
	case entity.BatchStateDiscarded:
		return batch, nil
	default:
		return entity.ImportBatch{}, ErrConflict
	}
}

// GetBatch returns one batch with all of its lines ordered by line_ref.
// The returned line slice is non-nil (empty) when the batch has no lines.
func (s *Service) GetBatch(ctx context.Context, batchID string) (entity.ImportBatch, []entity.ImportLine, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}

	batch, err := s.factory.ImportBatches.Get(ctx, batchID, repo.Owner(tid))
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}

	lines, err := s.factory.ImportLines.List(ctx, repo.Owner(tid), repo.Where("batch_id", "=", batchID), repo.OrderBy("line_ref"))
	if err != nil {
		return entity.ImportBatch{}, nil, err
	}
	return batch, lines, nil
}

// ListBatches returns all batches of the user ordered by created_at
// ascending with ties broken by id ascending. The returned slice is non-nil
// (empty) when the user has no batches.
func (s *Service) ListBatches(ctx context.Context) ([]entity.ImportBatch, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return nil, err
	}
	batches, err := s.factory.ImportBatches.List(ctx, repo.Owner(tid), repo.OrderBy("created_at, id"))
	if err != nil {
		return nil, err
	}
	return batches, nil
}

// applyAutoLinks idempotently auto-links the batch's unlinked import
// movements to documents: for each movement with Origin import and no linked
// document, exactly one candidate (by exact amount and currency) is linked
// with LinkCreator auto; zero or more than one candidate leaves the movement
// unlinked. Already-linked documents are not candidates, so two movements in
// the same batch can never both take the same document.
func (s *Service) applyAutoLinks(ctx context.Context, tid string, batch entity.ImportBatch) error {
	movs, err := s.factory.Movements.List(ctx, repo.Owner(tid), repo.Where("import_batch_id", "=", batch.ID))
	if err != nil {
		return fmt.Errorf("apply auto links for batch %s: list movements: %w", batch.ID, err)
	}
	for _, mv := range movs {
		if mv.Origin != entity.OriginImport || mv.LinkedDocumentID != nil {
			continue
		}
		cands, err := s.linkCands.LinkCandidates(ctx, mv.Amount, mv.Currency, repo.Owner(tid))
		if err != nil {
			return fmt.Errorf("apply auto links for batch %s: list candidates for movement %s: %w", batch.ID, mv.ID, err)
		}
		if len(cands) != 1 {
			continue // zero or >1 candidates: no link
		}
		docID := cands[0].ID
		auto := entity.LinkCreatorAuto
		mv.LinkedDocumentID = &docID
		mv.LinkCreator = &auto
		if _, err := s.factory.Movements.Update(ctx, mv, repo.Owner(tid)); err != nil {
			return fmt.Errorf("apply auto links for batch %s: link movement %s: %w", batch.ID, mv.ID, err)
		}
	}
	return nil
}

// pdfTextLines splits extracted PDF text on newlines, trims each element, and
// drops empty strings, producing one raw line per non-empty line.
func pdfTextLines(text string) []string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// countStatus counts the lines with the given status among parsed lines.
func countStatus(lines []ParsedLine, status string) int {
	n := 0
	for _, l := range lines {
		if l.Status == status {
			n++
		}
	}
	return n
}

// countImportStatus counts the lines with the given status among import
// lines.
func countImportStatus(lines []entity.ImportLine, status string) int {
	n := 0
	for _, l := range lines {
		if l.Status == status {
			n++
		}
	}
	return n
}
