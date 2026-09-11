package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ---------------------------------------------------------------------------
// In-memory fakes for the Service pipeline
// ---------------------------------------------------------------------------

// fakeStorage is an in-memory repo.FileStorage for tests. It returns a
// freshly-generated Source row for every Put and counts how many times Put was
// called so tests can assert that no new source is stored (e.g. on the
// duplicate short-circuit).
type fakeStorage struct {
	mu       sync.Mutex
	putCount int
}

var _ repo.FileStorage = (*fakeStorage)(nil)

func (f *fakeStorage) Put(ctx context.Context, data []byte) (entity.Source, error) {
	f.mu.Lock()
	f.putCount++
	n := f.putCount
	f.mu.Unlock()

	tid, _ := user.UserFrom(ctx)
	sum := sha256.Sum256(data)
	return entity.Source{
		ID:          "src-" + itoa(n),
		OwnerID:     tid,
		ContentType: "application/pdf",
		Size:        int64(len(data)),
		Path:        "x",
		SHA256:      hex.EncodeToString(sum[:]),
		UploadedAt:  time.Now(),
	}, nil
}

// Get satisfies the repo.FileStorage interface. The fake never needs to read
// back bytes (Reprocess is exercised against real file storage in the api
// integration tests), so this is a no-op stub.
func (f *fakeStorage) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// putCalls returns the number of Put invocations so far.
func (f *fakeStorage) putCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putCount
}

// itoa is a tiny int-to-string helper (strconv-free) for unique IDs.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// fakeReviewer is an in-memory repo.Reviewer for tests. It records the last
// HoldInput and counts Hold invocations so tests can assert on the provenance
// that was passed and on how many times a hold happened.
type fakeReviewer struct {
	mu        sync.Mutex
	lastInput repo.HoldInput
	holdCount int
}

var _ repo.Reviewer = (*fakeReviewer)(nil)

func (f *fakeReviewer) Hold(_ context.Context, input repo.HoldInput) (entity.IngestReview, error) {
	f.mu.Lock()
	f.lastInput = input
	f.holdCount++
	f.mu.Unlock()
	return entity.IngestReview{
		ID:                 "review-1",
		SourceID:           input.SourceID,
		State:              entity.ReviewStatePending,
		Provenance:         input.Provenance,
		OwnerHouseholdID:   input.OwnerHouseholdID,
	}, nil
}

// holdCalls returns the number of Hold invocations so far.
func (f *fakeReviewer) holdCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holdCount
}

// identityAssetRepo is an in-memory repo.Repository[entity.Asset] that, unlike
// the existing fakeAssetRepo, actually honors the identity stage filters
// (norm_serial / norm_brand / norm_model / norm_name "=" comparisons and the
// owner_household_id scope fence, plus owner scoping and limit). This lets the
// 3-stage identity lookup in core/identity run end-to-end in tests.
type identityAssetRepo struct {
	mu          sync.Mutex
	rows        []entity.Asset
	createCount int
}

var _ repo.Repository[entity.Asset] = (*identityAssetRepo)(nil)

func (f *identityAssetRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	for _, a := range f.rows {
		if a.ID == id && (o.OwnerID == "" || a.OwnerID == o.OwnerID) {
			return a, nil
		}
	}
	return entity.Asset{}, repo.ErrNotFound
}

// normVal dereferences a normalized column pointer, returning "" for nil.
func normVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// hhVal dereferences an owner_household_id pointer, returning "" for nil.
func hhVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// matchFilters reports whether asset a satisfies every filter in flts.
func matchFilters(a entity.Asset, flts []repo.Filter) bool {
	for _, flt := range flts {
		if flt.Op != "=" && flt.Op != "IS NULL" {
			continue // identity only issues these two ops
		}
		switch flt.Field {
		case "norm_serial":
			if flt.Op == "=" && normVal(a.NormSerial) != fltStr(flt) {
				return false
			}
		case "norm_brand":
			if flt.Op == "=" && normVal(a.NormBrand) != fltStr(flt) {
				return false
			}
		case "norm_model":
			if flt.Op == "=" && normVal(a.NormModel) != fltStr(flt) {
				return false
			}
		case "norm_name":
			if flt.Op == "=" && normVal(a.NormName) != fltStr(flt) {
				return false
			}
		case "owner_household_id":
			if flt.Op == "IS NULL" {
				if a.OwnerHouseholdID != nil {
					return false
				}
			} else if flt.Op == "=" {
				if a.OwnerHouseholdID == nil || *a.OwnerHouseholdID != fltStr(flt) {
					return false
				}
			}
		}
	}
	return true
}

// fltStr coerces a filter value to a string (identity filters carry strings).
func fltStr(flt repo.Filter) string {
	if s, ok := flt.Value.(string); ok {
		return s
	}
	return ""
}

func (f *identityAssetRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	var out []entity.Asset
	for _, a := range f.rows {
		if o.OwnerID != "" && a.OwnerID != o.OwnerID {
			continue
		}
		if matchFilters(a, o.Filters) {
			out = append(out, a)
		}
	}
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (f *identityAssetRepo) Create(_ context.Context, e entity.Asset, _ ...repo.Option) (entity.Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCount++
	f.rows = append(f.rows, e)
	return e, nil
}

func (f *identityAssetRepo) Update(_ context.Context, e entity.Asset, _ ...repo.Option) (entity.Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == e.ID {
			f.rows[i] = e
			return e, nil
		}
	}
	return e, repo.ErrNotFound
}

func (f *identityAssetRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return repo.ErrNotFound
}

// newFakeFactory assembles a *repo.Factory wiring the given repos together and
// an InTx that simply invokes the callback against the same repos (no real
// transactional semantics are needed for these tests). At least Assets and
// Documents are set — that is all the commit path uses.
func newFakeFactory(assets *identityAssetRepo, sources *fakeSourceRepo, docs *fakeDocumentRepo) *repo.Factory {
	return &repo.Factory{
		Assets:    assets,
		Sources:   sources,
		Documents: docs,
		InTx: func(ctx context.Context, fn func(context.Context, *repo.Repos) error) error {
			return fn(ctx, &repo.Repos{Assets: assets, Documents: docs, Sources: sources})
		},
	}
}

// ---------------------------------------------------------------------------
// TestProcess
// ---------------------------------------------------------------------------

// TestProcess exercises the full Service.Process pipeline end-to-end through
// the public API (fakes for storage, repos, reviewer, chatter). It covers the
// five outcome kinds plus the oversize rejection and the budget-overrun hold.
func TestProcess(t *testing.T) {
	payload := []byte("%PDF-1.4 test")

	t.Run("committed", func(t *testing.T) {
		t.Parallel()

		sources := &fakeSourceRepo{}
		docs := &fakeDocumentRepo{}
		assets := &identityAssetRepo{}
		storage := &fakeStorage{}
		reviewer := &fakeReviewer{}

		// Both workers agree on the same identity → consensus conf 0.9.
		const agreeJSON = `{"serial_number":"SN-C","brand":"LG","model":"X1","name":"Washer","confidence":0.9}`
		chatter := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
			return agreeJSON, nil
		}}
		workers := []Worker{{Model: "m1"}, {Model: "m2"}}
		extractor := NewExtractor(chatter, workers, 5*time.Second, "sp")
		factory := newFakeFactory(assets, sources, docs)

		svc := New(factory, extractor, storage, 1<<20, 0.7, reviewer, 30, 10, 5*time.Second)

		ctx := user.WithUser(context.Background(), testUser)
		out, err := svc.Process(ctx, Input{Filename: "d.pdf", Payload: payload, ContentType: "application/pdf"})

		if err != nil {
			t.Fatalf("Process: unexpected error: %v", err)
		}
		if out.Kind != OutcomeCommitted {
			t.Fatalf("out.Kind = %q, want %q", out.Kind, OutcomeCommitted)
		}
		if out.Asset == nil {
			t.Fatal("out.Asset is nil, want non-nil")
		}
		if out.Review != nil {
			t.Errorf("out.Review = %+v, want nil for committed", out.Review)
		}
		if out.Asset.AssetCategory == nil {
			t.Errorf("out.Asset.AssetCategory = nil, want a non-nil inferred category")
		}
		if reviewer.holdCalls() != 0 {
			t.Errorf("reviewer.holdCount = %d, want 0", reviewer.holdCalls())
		}
		if assets.createCount != 1 {
			t.Errorf("assets.createCount = %d, want 1 (a new asset was created)", assets.createCount)
		}
		if sources.createCount != 1 {
			t.Fatalf("sources.createCount = %d, want 1", sources.createCount)
		}
		srcID := sources.rows[0].ID
		if docs.createCount != 1 {
			t.Fatalf("docs.createCount = %d, want 1", docs.createCount)
		}
		if docs.rows[0].AssetID != out.Asset.ID {
			t.Errorf("doc.AssetID = %q, want %q (out.Asset.ID)", docs.rows[0].AssetID, out.Asset.ID)
		}
		if docs.rows[0].SourceID != srcID {
			t.Errorf("doc.SourceID = %q, want %q (the single source row)", docs.rows[0].SourceID, srcID)
		}
	})

	t.Run("held", func(t *testing.T) {
		t.Parallel()

		sources := &fakeSourceRepo{}
		docs := &fakeDocumentRepo{}
		assets := &identityAssetRepo{}
		storage := &fakeStorage{}
		reviewer := &fakeReviewer{}

		// Seed one asset whose normalized serial matches the extraction, so
		// identity finds exactly one candidate.
		ns := commons.NormalizeSerial("SN-H")
		assets.rows = append(assets.rows, entity.Asset{
			ID:           "seed-h",
			OwnerID:      testUser,
			OwnerHouseholdID: nil,
			NormSerial:   &ns,
		})

		// Single worker → consensus conf 0.6 < threshold 0.7 → held.
		const heldJSON = `{"serial_number":"SN-H","brand":"LG","model":"X2","confidence":0.9}`
		chatter := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
			return heldJSON, nil
		}}
		workers := []Worker{{Model: "m1"}}
		extractor := NewExtractor(chatter, workers, 5*time.Second, "sp")
		factory := newFakeFactory(assets, sources, docs)

		svc := New(factory, extractor, storage, 1<<20, 0.7, reviewer, 30, 10, 5*time.Second)

		ctx := user.WithUser(context.Background(), testUser)
		out, err := svc.Process(ctx, Input{Filename: "d.pdf", Payload: payload, ContentType: "application/pdf"})

		if err != nil {
			t.Fatalf("Process: unexpected error: %v", err)
		}
		if out.Kind != OutcomeHeldForReview {
			t.Fatalf("out.Kind = %q, want %q", out.Kind, OutcomeHeldForReview)
		}
		if out.Review == nil {
			t.Fatal("out.Review is nil, want non-nil")
		}
		if reviewer.holdCalls() != 1 {
			t.Errorf("reviewer.holdCount = %d, want 1", reviewer.holdCalls())
		}
		if assets.createCount != 0 {
			t.Errorf("assets.createCount = %d, want 0 (no new asset)", assets.createCount)
		}
		if docs.createCount != 0 {
			t.Errorf("docs.createCount = %d, want 0", docs.createCount)
		}
		workersAny, ok := out.Review.Provenance["workers"].([]any)
		if !ok || len(workersAny) != 1 {
			t.Fatalf("Provenance[\"workers\"] = %v, want a []any of len 1", out.Review.Provenance["workers"])
		}
		cands, ok := out.Review.Provenance["candidates"].([]entity.Asset)
		if !ok || len(cands) != 1 {
			t.Fatalf("Provenance[\"candidates\"] = %v, want []entity.Asset of len 1", out.Review.Provenance["candidates"])
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		sources := &fakeSourceRepo{}
		docs := &fakeDocumentRepo{}
		assets := &identityAssetRepo{}
		storage := &fakeStorage{}
		reviewer := &fakeReviewer{}

		sum := sha256.Sum256(payload)
		hash := hex.EncodeToString(sum[:])
		sources.rows = append(sources.rows, entity.Source{ID: "src-dup", OwnerID: testUser, SHA256: hash})
		docs.rows = append(docs.rows, entity.Document{ID: "doc-dup", SourceID: "src-dup", AssetID: "asset-dup", OwnerID: testUser})
		assets.rows = append(assets.rows, entity.Asset{ID: "asset-dup", OwnerID: testUser})

		// Should never be reached (dedupe short-circuits before extraction).
		chatter := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
			return validExtractionJSON, nil
		}}
		extractor := NewExtractor(chatter, []Worker{{Model: "m1"}}, 5*time.Second, "sp")
		factory := newFakeFactory(assets, sources, docs)

		svc := New(factory, extractor, storage, 1<<20, 0.7, reviewer, 30, 10, 5*time.Second)

		ctx := user.WithUser(context.Background(), testUser)
		out, err := svc.Process(ctx, Input{Filename: "d.pdf", Payload: payload, ContentType: "application/pdf"})

		if err != nil {
			t.Fatalf("Process: unexpected error: %v", err)
		}
		if out.Kind != OutcomeDuplicate {
			t.Fatalf("out.Kind = %q, want %q", out.Kind, OutcomeDuplicate)
		}
		if out.Duplicate == nil {
			t.Fatal("out.Duplicate is nil, want non-nil")
		}
		if out.Duplicate.SourceID != "src-dup" {
			t.Errorf("Duplicate.SourceID = %q, want src-dup", out.Duplicate.SourceID)
		}
		if out.Duplicate.DocumentID != "doc-dup" {
			t.Errorf("Duplicate.DocumentID = %q, want doc-dup", out.Duplicate.DocumentID)
		}
		if out.Duplicate.AssetID != "asset-dup" {
			t.Errorf("Duplicate.AssetID = %q, want asset-dup", out.Duplicate.AssetID)
		}
		if out.Duplicate.AssetDeleted {
			t.Error("Duplicate.AssetDeleted = true, want false")
		}
		if storage.putCalls() != 0 {
			t.Errorf("storage Put calls = %d, want 0 (no new source)", storage.putCalls())
		}
		if sources.createCount != 0 {
			t.Errorf("sources.createCount = %d, want 0", sources.createCount)
		}
		if reviewer.holdCalls() != 0 {
			t.Errorf("reviewer.holdCount = %d, want 0", reviewer.holdCalls())
		}
		if docs.createCount != 0 {
			t.Errorf("docs.createCount = %d, want 0", docs.createCount)
		}
		if chatter.count() != 0 {
			t.Errorf("chatter.count() = %d, want 0 (no LLM call)", chatter.count())
		}
	})

	t.Run("failed", func(t *testing.T) {
		t.Parallel()

		sources := &fakeSourceRepo{}
		docs := &fakeDocumentRepo{}
		assets := &identityAssetRepo{}
		storage := &fakeStorage{}
		reviewer := &fakeReviewer{}

		chatter := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
			return "", errors.New("boom")
		}}
		workers := []Worker{{Model: "m1"}}
		extractor := NewExtractor(chatter, workers, 5*time.Second, "sp")
		factory := newFakeFactory(assets, sources, docs)

		svc := New(factory, extractor, storage, 1<<20, 0.7, reviewer, 30, 10, 5*time.Second)

		ctx := user.WithUser(context.Background(), testUser)
		out, err := svc.Process(ctx, Input{Filename: "d.pdf", Payload: payload, ContentType: "application/pdf"})

		// Failure is an Outcome, not a Go error.
		if err != nil {
			t.Fatalf("Process: unexpected error: %v", err)
		}
		if out.Kind != OutcomeFailed {
			t.Fatalf("out.Kind = %q, want %q", out.Kind, OutcomeFailed)
		}
		if out.Reason == "" {
			t.Errorf("out.Reason = %q, want non-empty", out.Reason)
		}
		if sources.createCount != 1 {
			t.Errorf("sources.createCount = %d, want 1 (source retained)", sources.createCount)
		}
		if assets.createCount != 0 {
			t.Errorf("assets.createCount = %d, want 0", assets.createCount)
		}
		if docs.createCount != 0 {
			t.Errorf("docs.createCount = %d, want 0", docs.createCount)
		}
	})

	t.Run("statement", func(t *testing.T) {
		t.Parallel()

		sources := &fakeSourceRepo{}
		docs := &fakeDocumentRepo{}
		assets := &identityAssetRepo{}
		storage := &fakeStorage{}
		reviewer := &fakeReviewer{}

		const statementJSON = `{"classification":"statement","serial_number":"SN-S","confidence":0.9}`
		chatter := &fakeChatter{fn: func(_ context.Context, _ string, _ []ContentPart) (string, error) {
			return statementJSON, nil
		}}
		workers := []Worker{{Model: "m1"}}
		extractor := NewExtractor(chatter, workers, 5*time.Second, "sp")
		factory := newFakeFactory(assets, sources, docs)

		svc := New(factory, extractor, storage, 1<<20, 0.7, reviewer, 30, 10, 5*time.Second)

		ctx := user.WithUser(context.Background(), testUser)
		out, err := svc.Process(ctx, Input{Filename: "d.pdf", Payload: payload, ContentType: "application/pdf"})

		sum := sha256.Sum256(payload)
		hash := hex.EncodeToString(sum[:])
		if err != nil {
			t.Fatalf("Process: unexpected error: %v", err)
		}
		if out.Kind != OutcomeStatement {
			t.Fatalf("out.Kind = %q, want %q", out.Kind, OutcomeStatement)
		}
		if out.Statement == nil {
			t.Fatal("out.Statement is nil, want the retained source")
		}
		if out.Statement.SHA256 != hash {
			t.Errorf("out.Statement.SHA256 = %q, want %q (hash of payload)", out.Statement.SHA256, hash)
		}
		if sources.createCount != 1 {
			t.Errorf("sources.createCount = %d, want 1 (source retained)", sources.createCount)
		}
		if assets.createCount != 0 {
			t.Errorf("assets.createCount = %d, want 0", assets.createCount)
		}
		if docs.createCount != 0 {
			t.Errorf("docs.createCount = %d, want 0", docs.createCount)
		}
		if reviewer.holdCalls() != 0 {
			t.Errorf("reviewer.holdCount = %d, want 0", reviewer.holdCalls())
		}
	})

	t.Run("oversize", func(t *testing.T) {
		t.Parallel()

		sources := &fakeSourceRepo{}
		docs := &fakeDocumentRepo{}
		assets := &identityAssetRepo{}
		storage := &fakeStorage{}
		reviewer := &fakeReviewer{}

		chatter := &fakeChatter{}
		extractor := NewExtractor(chatter, []Worker{{Model: "m1"}}, 5*time.Second, "sp")
		factory := newFakeFactory(assets, sources, docs)

		svc := New(factory, extractor, storage, 4, 0.7, reviewer, 30, 10, 5*time.Second)

		big := []byte("%PDF-1.4 longer") // len 15 > maxBytes 4
		ctx := user.WithUser(context.Background(), testUser)
		_, err := svc.Process(ctx, Input{Filename: "big.pdf", Payload: big, ContentType: "application/pdf"})

		if err == nil {
			t.Fatal("Process: expected an error for an oversize payload, got nil")
		}
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v, want ErrTooLarge", err)
		}
	})

	t.Run("budget_overrun", func(t *testing.T) {
		// Not parallel: timing-sensitive (the budget must fire before the worker).

		sources := &fakeSourceRepo{}
		docs := &fakeDocumentRepo{}
		assets := &identityAssetRepo{}
		storage := &fakeStorage{}
		reviewer := &fakeReviewer{}

		// The worker blocks until its (per-worker) context is cancelled. The
		// Service's processTimeout (50ms) is far smaller than the per-worker
		// timeout (5s), so the end-to-end budget fires first and the worker
		// returns ctx.Err() (deadline exceeded).
		chatter := &fakeChatter{fn: func(ctx context.Context, _ string, _ []ContentPart) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}}
		workers := []Worker{{Model: "m1"}}
		extractor := NewExtractor(chatter, workers, 5*time.Second, "sp")
		factory := newFakeFactory(assets, sources, docs)

		svc := New(factory, extractor, storage, 1<<20, 0.7, reviewer, 30, 10, 50*time.Millisecond)

		ctx := user.WithUser(context.Background(), testUser)
		out, err := svc.Process(ctx, Input{Filename: "d.pdf", Payload: payload, ContentType: "application/pdf"})

		if err != nil {
			t.Fatalf("Process: unexpected error: %v", err)
		}
		if out.Kind != OutcomeHeldForReview {
			t.Fatalf("out.Kind = %q, want %q", out.Kind, OutcomeHeldForReview)
		}
		if out.Review == nil {
			t.Fatal("out.Review is nil, want non-nil")
		}
		if reviewer.holdCalls() != 1 {
			t.Errorf("reviewer.holdCount = %d, want 1", reviewer.holdCalls())
		}
		workersAny, ok := out.Review.Provenance["workers"].([]any)
		if !ok || len(workersAny) != 1 {
			t.Fatalf("Provenance[\"workers\"] = %v, want a []any of len 1 (partial per-worker result)", out.Review.Provenance["workers"])
		}
	})
}
