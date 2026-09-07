package processing

import (
	"context"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// testUser is the owner id used across dedupe tests.
const testUser = "test-user"

func timePtr(t time.Time) *time.Time { return &t }

// ---------------------------------------------------------------------------
// In-memory repository fakes
// ---------------------------------------------------------------------------

type fakeSourceRepo struct {
	mu          sync.Mutex
	rows        []entity.Source
	createCount int
}

var _ repo.Repository[entity.Source] = (*fakeSourceRepo)(nil)

func (f *fakeSourceRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Source, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	for _, s := range f.rows {
		if s.ID == id && (o.OwnerID == "" || s.OwnerID == o.OwnerID) {
			return s, nil
		}
	}
	return entity.Source{}, repo.ErrNotFound
}

func (f *fakeSourceRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Source, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	var out []entity.Source
	for _, s := range f.rows {
		if o.OwnerID != "" && s.OwnerID != o.OwnerID {
			continue
		}
		match := true
		for _, flt := range o.Filters {
			if flt.Field == "sha256" && flt.Op == "=" && s.SHA256 != flt.Value {
				match = false
			}
		}
		if match {
			out = append(out, s)
		}
	}
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (f *fakeSourceRepo) Create(_ context.Context, e entity.Source, _ ...repo.Option) (entity.Source, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCount++
	f.rows = append(f.rows, e)
	return e, nil
}

func (f *fakeSourceRepo) Update(_ context.Context, e entity.Source, _ ...repo.Option) (entity.Source, error) {
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

func (f *fakeSourceRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
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

type fakeDocumentRepo struct {
	mu          sync.Mutex
	rows        []entity.Document
	createCount int
}

var _ repo.Repository[entity.Document] = (*fakeDocumentRepo)(nil)

func (f *fakeDocumentRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	for _, d := range f.rows {
		if d.ID == id && (o.OwnerID == "" || d.OwnerID == o.OwnerID) {
			return d, nil
		}
	}
	return entity.Document{}, repo.ErrNotFound
}

func (f *fakeDocumentRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	var out []entity.Document
	for _, d := range f.rows {
		if o.OwnerID != "" && d.OwnerID != o.OwnerID {
			continue
		}
		match := true
		for _, flt := range o.Filters {
			if flt.Field == "source_id" && flt.Op == "=" && d.SourceID != flt.Value {
				match = false
			}
		}
		if match {
			out = append(out, d)
		}
	}
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (f *fakeDocumentRepo) Create(_ context.Context, e entity.Document, _ ...repo.Option) (entity.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCount++
	f.rows = append(f.rows, e)
	return e, nil
}

func (f *fakeDocumentRepo) Update(_ context.Context, e entity.Document, _ ...repo.Option) (entity.Document, error) {
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

func (f *fakeDocumentRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
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

type fakeAssetRepo struct {
	mu          sync.Mutex
	rows        []entity.Asset
	createCount int
}

var _ repo.Repository[entity.Asset] = (*fakeAssetRepo)(nil)

func (f *fakeAssetRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Asset, error) {
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

func (f *fakeAssetRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	var out []entity.Asset
	for _, a := range f.rows {
		if o.OwnerID != "" && a.OwnerID != o.OwnerID {
			continue
		}
		out = append(out, a)
	}
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (f *fakeAssetRepo) Create(_ context.Context, e entity.Asset, _ ...repo.Option) (entity.Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCount++
	f.rows = append(f.rows, e)
	return e, nil
}

func (f *fakeAssetRepo) Update(_ context.Context, e entity.Asset, _ ...repo.Option) (entity.Asset, error) {
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

func (f *fakeAssetRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestDedupeSource_KnownHash verifies that a byte-identical re-upload
// short-circuits to a duplicate outcome with the existing source/document/asset
// IDs, and that no new source is stored. The "no LLM call" guarantee is
// structural: DedupeSource has no LLM dependency (no extractor/Chatter
// param), so returning ok=true lets the caller skip extraction entirely.
func TestDedupeSource_KnownHash(t *testing.T) {
	t.Parallel()

	const hash = "abc123def456"

	sources := &fakeSourceRepo{}
	docs := &fakeDocumentRepo{}
	assets := &fakeAssetRepo{}

	// Seed source S with SHA256 = H.
	s := entity.Source{ID: "S1", OwnerID: testUser, SHA256: hash}
	sources.rows = append(sources.rows, s)

	// Seed asset A (active, DeletedAt nil).
	a := entity.Asset{ID: "A1", OwnerID: testUser}
	assets.rows = append(assets.rows, a)

	// Seed document D linked to source S and asset A.
	d := entity.Document{ID: "D1", OwnerID: testUser, SourceID: s.ID, AssetID: a.ID}
	docs.rows = append(docs.rows, d)

	ctx := user.WithUser(context.Background(), testUser)
	out, ok, err := DedupeSource(ctx, sources, docs, assets, hash)
	if err != nil {
		t.Fatalf("DedupeSource: unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("DedupeSource: expected ok=true, got false")
	}
	if out.Kind != OutcomeDuplicate {
		t.Errorf("out.Kind = %q, want %q", out.Kind, OutcomeDuplicate)
	}
	if out.Duplicate == nil {
		t.Fatal("out.Duplicate is nil, want non-nil")
	}
	if out.Duplicate.SourceID != s.ID {
		t.Errorf("out.Duplicate.SourceID = %q, want %q", out.Duplicate.SourceID, s.ID)
	}
	if out.Duplicate.DocumentID != d.ID {
		t.Errorf("out.Duplicate.DocumentID = %q, want %q", out.Duplicate.DocumentID, d.ID)
	}
	if out.Duplicate.AssetID != a.ID {
		t.Errorf("out.Duplicate.AssetID = %q, want %q", out.Duplicate.AssetID, a.ID)
	}
	if out.Duplicate.AssetDeleted {
		t.Error("out.Duplicate.AssetDeleted = true, want false (asset is active)")
	}
	if sources.createCount != 0 {
		t.Errorf("sources.createCount = %d, want 0 (no new source stored)", sources.createCount)
	}
}

// TestDedupeSource_SoftDeletedAsset verifies that when the linked asset is
// soft-deleted, AssetDeleted is flagged true so the UI can offer restore.
func TestDedupeSource_SoftDeletedAsset(t *testing.T) {
	t.Parallel()

	const hash = "deleted-asset-hash"

	sources := &fakeSourceRepo{}
	docs := &fakeDocumentRepo{}
	assets := &fakeAssetRepo{}

	now := time.Now()

	s := entity.Source{ID: "S2", OwnerID: testUser, SHA256: hash}
	sources.rows = append(sources.rows, s)

	a := entity.Asset{ID: "A2", OwnerID: testUser, DeletedAt: timePtr(now)}
	assets.rows = append(assets.rows, a)

	d := entity.Document{ID: "D2", OwnerID: testUser, SourceID: s.ID, AssetID: a.ID}
	docs.rows = append(docs.rows, d)

	ctx := user.WithUser(context.Background(), testUser)
	out, ok, err := DedupeSource(ctx, sources, docs, assets, hash)
	if err != nil {
		t.Fatalf("DedupeSource: unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("DedupeSource: expected ok=true, got false")
	}
	if out.Duplicate == nil {
		t.Fatal("out.Duplicate is nil, want non-nil")
	}
	if !out.Duplicate.AssetDeleted {
		t.Error("out.Duplicate.AssetDeleted = false, want true (asset is soft-deleted)")
	}
}

// TestDedupeSource_UnknownHash verifies that a hash with no matching source
// returns ok=false with a zero Outcome (not a duplicate).
func TestDedupeSource_UnknownHash(t *testing.T) {
	t.Parallel()

	const hash = "no-such-hash"

	sources := &fakeSourceRepo{}
	docs := &fakeDocumentRepo{}
	assets := &fakeAssetRepo{}

	ctx := user.WithUser(context.Background(), testUser)
	out, ok, err := DedupeSource(ctx, sources, docs, assets, hash)
	if err != nil {
		t.Fatalf("DedupeSource: unexpected error: %v", err)
	}
	if ok {
		t.Fatal("DedupeSource: expected ok=false for unknown hash, got true")
	}
	if out.Kind != "" {
		t.Errorf("out.Kind = %q, want zero value (empty string)", out.Kind)
	}
}

// TestDedupeSource_EmptyHash verifies that an empty hash returns ok=false
// (nothing to dedupe on).
func TestDedupeSource_EmptyHash(t *testing.T) {
	t.Parallel()

	sources := &fakeSourceRepo{}
	docs := &fakeDocumentRepo{}
	assets := &fakeAssetRepo{}

	ctx := user.WithUser(context.Background(), testUser)
	out, ok, err := DedupeSource(ctx, sources, docs, assets, "")
	if err != nil {
		t.Fatalf("DedupeSource: unexpected error: %v", err)
	}
	if ok {
		t.Fatal("DedupeSource: expected ok=false for empty hash, got true")
	}
	if out.Kind != "" {
		t.Errorf("out.Kind = %q, want zero value (empty string)", out.Kind)
	}
}

// TestDedupeSource_CrossTenantIsolation seeds a source owned by a different
// user. DedupeSource run under the test user must not see it (owner-scoped)
// and returns ok=false.
func TestDedupeSource_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	const hash = "tenant-hash"

	sources := &fakeSourceRepo{}
	docs := &fakeDocumentRepo{}
	assets := &fakeAssetRepo{}

	other := "other-user"
	s := entity.Source{ID: "S9", OwnerID: other, SHA256: hash}
	sources.rows = append(sources.rows, s)

	ctx := user.WithUser(context.Background(), testUser)
	out, ok, err := DedupeSource(ctx, sources, docs, assets, hash)
	if err != nil {
		t.Fatalf("DedupeSource: unexpected error: %v", err)
	}
	if ok {
		t.Fatal("DedupeSource: expected ok=false for another owner's source, got true")
	}
	if out.Kind != "" {
		t.Errorf("out.Kind = %q, want zero value", out.Kind)
	}
}

// TestDedupeSource_AssetMissing seeds a source + document whose AssetID points
// at a nonexistent asset. DedupeSource must still report the duplicate with
// AssetDeleted=false (a missing/invisible asset is treated as active, not
// deleted).
func TestDedupeSource_AssetMissing(t *testing.T) {
	t.Parallel()

	const hash = "missing-asset-hash"

	sources := &fakeSourceRepo{}
	docs := &fakeDocumentRepo{}
	assets := &fakeAssetRepo{}

	s := entity.Source{ID: "S3", OwnerID: testUser, SHA256: hash}
	sources.rows = append(sources.rows, s)
	d := entity.Document{ID: "D3", OwnerID: testUser, SourceID: s.ID, AssetID: "no-such-asset"}
	docs.rows = append(docs.rows, d)

	ctx := user.WithUser(context.Background(), testUser)
	out, ok, err := DedupeSource(ctx, sources, docs, assets, hash)
	if err != nil {
		t.Fatalf("DedupeSource: unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("DedupeSource: expected ok=true, got false")
	}
	if out.Duplicate == nil {
		t.Fatal("out.Duplicate is nil, want non-nil")
	}
	if out.Duplicate.AssetID != "no-such-asset" {
		t.Errorf("out.Duplicate.AssetID = %q, want %q", out.Duplicate.AssetID, "no-such-asset")
	}
	if out.Duplicate.AssetDeleted {
		t.Error("out.Duplicate.AssetDeleted = true, want false (asset missing is treated as active)")
	}
}

// TestDedupeSource_SourceWithoutDocument seeds a source with no linked document
// (e.g. a held-for-review upload). DedupeSource must still report the
// duplicate, referencing the source with empty DocumentID/AssetID.
func TestDedupeSource_SourceWithoutDocument(t *testing.T) {
	t.Parallel()

	const hash = "no-doc-hash"

	sources := &fakeSourceRepo{}
	docs := &fakeDocumentRepo{}
	assets := &fakeAssetRepo{}

	s := entity.Source{ID: "S4", OwnerID: testUser, SHA256: hash}
	sources.rows = append(sources.rows, s)

	ctx := user.WithUser(context.Background(), testUser)
	out, ok, err := DedupeSource(ctx, sources, docs, assets, hash)
	if err != nil {
		t.Fatalf("DedupeSource: unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("DedupeSource: expected ok=true, got false")
	}
	if out.Kind != OutcomeDuplicate {
		t.Errorf("out.Kind = %q, want %q", out.Kind, OutcomeDuplicate)
	}
	if out.Duplicate == nil {
		t.Fatal("out.Duplicate is nil, want non-nil")
	}
	if out.Duplicate.SourceID != s.ID {
		t.Errorf("out.Duplicate.SourceID = %q, want %q", out.Duplicate.SourceID, s.ID)
	}
	if out.Duplicate.DocumentID != "" {
		t.Errorf("out.Duplicate.DocumentID = %q, want empty (no linked document)", out.Duplicate.DocumentID)
	}
	if out.Duplicate.AssetID != "" {
		t.Errorf("out.Duplicate.AssetID = %q, want empty (no linked document)", out.Duplicate.AssetID)
	}
}
