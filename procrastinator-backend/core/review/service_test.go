package review

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/identity"
)

// testUser is the user ID used by all review tests.
const testUser = "test-user"

// otherUser is a distinct user used to verify cross-owner isolation.
const otherUser = "other-user"

// defaultRaw is the canonical LLM extraction payload carried on reviews.
const defaultRaw = `{"classification":"invoice","brand":"LG","model":"WM-2000","serial_number":"SN-123"}`

// Test sentinel for the document-create rollback case.
var errDocInsert = errors.New("document insert failed")

// Compile-time interface guards.
var (
	_ repo.Repository[entity.IngestReview] = (*fakeReviewRepo)(nil)
	_ repo.Repository[entity.Asset]        = (*fakeAssetRepo)(nil)
	_ repo.Repository[entity.Document]     = (*fakeDocumentRepo)(nil)
)

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// testCtx returns a context carrying the test user.
func testCtx() context.Context {
	return user.WithUser(context.Background(), testUser)
}

// ownerFromOpts extracts the owner ID from options ("" if absent).
func ownerFromOpts(opts []repo.Option) string {
	return repo.ApplyOptions(opts...).OwnerID
}

// copyMap shallow-copies a map (nil-safe).
func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// fakeReviewRepo
// ---------------------------------------------------------------------------

// fakeReviewRepo is an in-memory implementation of repo.Repository[entity.IngestReview].
type fakeReviewRepo struct {
	mu        sync.Mutex
	reviews   map[string]entity.IngestReview
	nextID    int
	createErr error
	updateErr error
	saved     map[string]entity.IngestReview
}

func newFakeReviewRepo() *fakeReviewRepo {
	return &fakeReviewRepo{reviews: make(map[string]entity.IngestReview)}
}

func (r *fakeReviewRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.IngestReview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rev, ok := r.reviews[id]
	if !ok {
		return entity.IngestReview{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" && rev.OwnerID != tid {
		return entity.IngestReview{}, repo.ErrNotFound
	}
	return rev, nil
}

func (r *fakeReviewRepo) List(_ context.Context, opts ...repo.Option) ([]entity.IngestReview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)

	out := make([]entity.IngestReview, 0, len(r.reviews))
	for _, rev := range r.reviews {
		if o.OwnerID != "" && rev.OwnerID != o.OwnerID {
			continue
		}
		match := true
		for _, f := range o.Filters {
			if !matchReviewFilter(rev, f) {
				match = false
				break
			}
		}
		if match {
			out = append(out, rev)
		}
	}

	if o.OrderBy == "created_at, id" {
		sort.Slice(out, func(i, j int) bool {
			if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
				return out[i].CreatedAt.Before(out[j].CreatedAt)
			}
			return out[i].ID < out[j].ID
		})
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}

	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

// matchReviewFilter supports "=" on the state field. Unsupported fields yield false.
func matchReviewFilter(rev entity.IngestReview, f repo.Filter) bool {
	if f.Field != "state" || f.Op != "=" {
		return false
	}
	want, ok := f.Value.(string)
	return ok && string(rev.State) == want
}

func (r *fakeReviewRepo) Create(_ context.Context, rev entity.IngestReview, opts ...repo.Option) (entity.IngestReview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.IngestReview{}, r.createErr
	}
	if rev.ID == "" {
		r.nextID++
		rev.ID = "rev-" + strconv.Itoa(r.nextID)
	}
	if tid := ownerFromOpts(opts); tid != "" {
		rev.OwnerID = tid
	}
	rev.CreatedAt = time.Now().UTC()
	rev.CandidateFields = copyMap(rev.CandidateFields)
	r.reviews[rev.ID] = rev
	return rev, nil
}

func (r *fakeReviewRepo) Update(_ context.Context, rev entity.IngestReview, opts ...repo.Option) (entity.IngestReview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.reviews[rev.ID]; !ok {
		return entity.IngestReview{}, repo.ErrNotFound
	}
	if r.updateErr != nil {
		return entity.IngestReview{}, r.updateErr
	}
	if tid := ownerFromOpts(opts); tid != "" {
		rev.OwnerID = tid
	}
	rev.CandidateFields = copyMap(rev.CandidateFields)
	r.reviews[rev.ID] = rev
	return rev, nil
}

func (r *fakeReviewRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.reviews[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.reviews, id)
	return nil
}

// count returns the number of stored reviews.
func (r *fakeReviewRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reviews)
}

// all returns every stored review in deterministic order by ID.
func (r *fakeReviewRepo) all() []entity.IngestReview {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.IngestReview, 0, len(r.reviews))
	for _, rev := range r.reviews {
		out = append(out, rev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// get returns the stored review with the given ID (no owner check).
func (r *fakeReviewRepo) get(id string) (entity.IngestReview, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rev, ok := r.reviews[id]
	return rev, ok
}

// snapshot deep-copies the review map for transaction rollback support.
func (r *fakeReviewRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]entity.IngestReview, len(r.reviews))
	for id, rev := range r.reviews {
		c := rev
		c.CandidateFields = copyMap(rev.CandidateFields)
		saved[id] = c
	}
	r.saved = saved
}

// rollback restores the review map from the last snapshot.
func (r *fakeReviewRepo) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		return
	}
	r.reviews = make(map[string]entity.IngestReview, len(r.saved))
	for id, rev := range r.saved {
		c := rev
		c.CandidateFields = copyMap(rev.CandidateFields)
		r.reviews[id] = c
	}
	r.saved = nil
}

// ---------------------------------------------------------------------------
// fakeAssetRepo
// ---------------------------------------------------------------------------

// fakeAssetRepo is an in-memory implementation of repo.Repository[entity.Asset].
// It implements the norm_serial/norm_brand/norm_model and owner_household_id
// filtering that identity.Match and identity.CommitCandidate depend on.
type fakeAssetRepo struct {
	mu        sync.Mutex
	assets    map[string]entity.Asset
	nextID    int
	createErr error
	saved     map[string]entity.Asset
}

func newFakeAssetRepo() *fakeAssetRepo {
	return &fakeAssetRepo{assets: make(map[string]entity.Asset)}
}

func (r *fakeAssetRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.assets[id]
	if !ok {
		return entity.Asset{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" && a.OwnerID != tid {
		return entity.Asset{}, repo.ErrNotFound
	}
	return a, nil
}

func (r *fakeAssetRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)

	out := make([]entity.Asset, 0, len(r.assets))
	for _, a := range r.assets {
		if o.OwnerID != "" && a.OwnerID != o.OwnerID {
			continue
		}
		match := true
		for _, f := range o.Filters {
			if !matchAssetFilter(a, f) {
				match = false
				break
			}
		}
		if match {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

// matchAssetFilter reports whether asset a satisfies filter f. It supports
// "=" and "IS NULL" on owner_household_id (the scope fence), and "=" on the
// norm_* string columns. Unsupported fields or operators yield false.
func matchAssetFilter(a entity.Asset, f repo.Filter) bool {
	if f.Field == "owner_household_id" {
		switch f.Op {
		case "=":
			want, ok := f.Value.(string)
			return ok && a.OwnerHouseholdID != nil && *a.OwnerHouseholdID == want
		case "IS NULL":
			return a.OwnerHouseholdID == nil
		}
		return false
	}
	if f.Op != "=" {
		return false
	}
	want, ok := f.Value.(string)
	if !ok {
		return false
	}
	var got *string
	switch f.Field {
	case "norm_serial":
		got = a.NormSerial
	case "norm_brand":
		got = a.NormBrand
	case "norm_model":
		got = a.NormModel
	default:
		return false
	}
	return got != nil && *got == want
}

func (r *fakeAssetRepo) Create(_ context.Context, a entity.Asset, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.Asset{}, r.createErr
	}
	if a.ID == "" {
		r.nextID++
		a.ID = "asset-" + strconv.Itoa(r.nextID)
	}
	if tid := ownerFromOpts(opts); tid != "" {
		a.OwnerID = tid
	}
	a.Metadata = copyMap(a.Metadata)
	r.assets[a.ID] = a
	return a, nil
}

func (r *fakeAssetRepo) Update(_ context.Context, a entity.Asset, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.assets[a.ID]; !ok {
		return entity.Asset{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" {
		a.OwnerID = tid
	}
	a.Metadata = copyMap(a.Metadata)
	a.UpdatedAt = time.Now().UTC()
	r.assets[a.ID] = a
	return a, nil
}

func (r *fakeAssetRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.assets[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.assets, id)
	return nil
}

// count returns the number of stored assets.
func (r *fakeAssetRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.assets)
}

// all returns every stored asset in deterministic order by ID.
func (r *fakeAssetRepo) all() []entity.Asset {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Asset, 0, len(r.assets))
	for _, a := range r.assets {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// get returns the asset with the given ID (no owner check).
func (r *fakeAssetRepo) get(id string) (entity.Asset, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.assets[id]
	return a, ok
}

// snapshot deep-copies the asset map for transaction rollback support.
func (r *fakeAssetRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]entity.Asset, len(r.assets))
	for id, a := range r.assets {
		c := a
		c.Metadata = copyMap(a.Metadata)
		saved[id] = c
	}
	r.saved = saved
}

// rollback restores the asset map from the last snapshot.
func (r *fakeAssetRepo) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		return
	}
	r.assets = make(map[string]entity.Asset, len(r.saved))
	for id, a := range r.saved {
		c := a
		c.Metadata = copyMap(a.Metadata)
		r.assets[id] = c
	}
	r.saved = nil
}

// ---------------------------------------------------------------------------
// fakeDocumentRepo
// ---------------------------------------------------------------------------

// fakeDocumentRepo is an in-memory implementation of repo.Repository[entity.Document].
type fakeDocumentRepo struct {
	mu        sync.Mutex
	documents map[string]entity.Document
	nextID    int
	createErr error
	saved     map[string]entity.Document
}

func newFakeDocumentRepo() *fakeDocumentRepo {
	return &fakeDocumentRepo{documents: make(map[string]entity.Document)}
}

func (r *fakeDocumentRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.documents[id]
	if !ok {
		return entity.Document{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" && d.OwnerID != tid {
		return entity.Document{}, repo.ErrNotFound
	}
	return d, nil
}

func (r *fakeDocumentRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.Document, 0, len(r.documents))
	for _, d := range r.documents {
		if o.OwnerID != "" && d.OwnerID != o.OwnerID {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (r *fakeDocumentRepo) Create(_ context.Context, d entity.Document, opts ...repo.Option) (entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.Document{}, r.createErr
	}
	if d.ID == "" {
		r.nextID++
		d.ID = "doc-" + strconv.Itoa(r.nextID)
	}
	if tid := ownerFromOpts(opts); tid != "" {
		d.OwnerID = tid
	}
	d.ExtractedFields = copyMap(d.ExtractedFields)
	r.documents[d.ID] = d
	return d, nil
}

func (r *fakeDocumentRepo) Update(_ context.Context, d entity.Document, opts ...repo.Option) (entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.documents[d.ID]; !ok {
		return entity.Document{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" {
		d.OwnerID = tid
	}
	d.ExtractedFields = copyMap(d.ExtractedFields)
	r.documents[d.ID] = d
	return d, nil
}

func (r *fakeDocumentRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.documents[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.documents, id)
	return nil
}

// count returns the number of stored documents.
func (r *fakeDocumentRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.documents)
}

// all returns every stored document in deterministic order by ID.
func (r *fakeDocumentRepo) all() []entity.Document {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Document, 0, len(r.documents))
	for _, d := range r.documents {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// get returns the document with the given ID (no owner check).
func (r *fakeDocumentRepo) get(id string) (entity.Document, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.documents[id]
	return d, ok
}

// snapshot deep-copies the document map for transaction rollback support.
func (r *fakeDocumentRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]entity.Document, len(r.documents))
	for id, d := range r.documents {
		c := d
		c.ExtractedFields = copyMap(d.ExtractedFields)
		saved[id] = c
	}
	r.saved = saved
}

// rollback restores the document map from the last snapshot.
func (r *fakeDocumentRepo) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		return
	}
	r.documents = make(map[string]entity.Document, len(r.saved))
	for id, d := range r.saved {
		c := d
		c.ExtractedFields = copyMap(d.ExtractedFields)
		r.documents[id] = c
	}
	r.saved = nil
}

// ---------------------------------------------------------------------------
// fakeFactory
// ---------------------------------------------------------------------------

// fakeFactory builds a concrete *repo.Factory from the fakes. InTx snapshots
// all three repos, rolls them back on error, and leaves them intact on success.
type fakeFactory struct {
	factory    *repo.Factory
	reviewRepo *fakeReviewRepo
	assetRepo  *fakeAssetRepo
	docRepo    *fakeDocumentRepo
}

func newFakeFactory(reviewRepo *fakeReviewRepo, assetRepo *fakeAssetRepo, docRepo *fakeDocumentRepo) *fakeFactory {
	f := &repo.Factory{
		Assets:    assetRepo,
		Documents: docRepo,
		Reviews:   reviewRepo,
		InTx: func(ctx context.Context, fn func(ctx context.Context, repos *repo.Repos) error) error {
			reviewRepo.snapshot()
			assetRepo.snapshot()
			docRepo.snapshot()
			repos := &repo.Repos{
				Assets:    assetRepo,
				Documents: docRepo,
				Reviews:   reviewRepo,
			}
			if err := fn(ctx, repos); err != nil {
				reviewRepo.rollback()
				assetRepo.rollback()
				docRepo.rollback()
				return err
			}
			return nil
		},
	}
	return &fakeFactory{factory: f, reviewRepo: reviewRepo, assetRepo: assetRepo, docRepo: docRepo}
}

// ---------------------------------------------------------------------------
// harness
// ---------------------------------------------------------------------------

// harness wires the fakes together with the Service under test.
type harness struct {
	svc     *Service
	factory *fakeFactory
}

func newHarness(t *testing.T, seedAssets []entity.Asset, seedReviews []entity.IngestReview) *harness {
	t.Helper()

	reviewRepo := newFakeReviewRepo()
	assetRepo := newFakeAssetRepo()
	docRepo := newFakeDocumentRepo()

	for _, a := range seedAssets {
		if a.OwnerID == "" {
			a.OwnerID = testUser
		}
		assetRepo.assets[a.ID] = a
	}
	for _, rev := range seedReviews {
		if rev.OwnerID == "" {
			rev.OwnerID = testUser
		}
		rev.CandidateFields = copyMap(rev.CandidateFields)
		reviewRepo.reviews[rev.ID] = rev
	}

	ff := newFakeFactory(reviewRepo, assetRepo, docRepo)
	return &harness{
		svc:     New(ff.factory),
		factory: ff,
	}
}

// ---------------------------------------------------------------------------
// TestHold
// ---------------------------------------------------------------------------

func TestHold(t *testing.T) {
	t.Parallel()

	t.Run("low confidence no match creates pending review", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil, nil)
		ctx := testCtx()

		conf := 0.4
		created, err := h.svc.Hold(ctx, repo.HoldInput{
			Extraction: entity.Extraction{
				Classification: "invoice",
				SerialNumber:   ptr("SN-999"),
				Confidence:     &conf,
				RawPayload:     defaultRaw,
			},
			SourceID: "src-1",
		})
		if err != nil {
			t.Fatalf("Hold returned error %v, want nil", err)
		}
		if created.State != entity.ReviewStatePending {
			t.Fatalf("State = %q, want pending", created.State)
		}
		if created.OwnerID != testUser {
			t.Fatalf("OwnerID = %q, want %q", created.OwnerID, testUser)
		}
		if created.SourceID != "src-1" {
			t.Fatalf("SourceID = %q, want \"src-1\"", created.SourceID)
		}
		if created.BestMatchedAssetID != nil {
			t.Fatalf("BestMatchedAssetID = %v, want nil (no match)", *created.BestMatchedAssetID)
		}
		if created.Confidence == nil || *created.Confidence != 0.4 {
			t.Fatalf("Confidence = %v, want 0.4", created.Confidence)
		}
		if h.factory.reviewRepo.count() != 1 {
			t.Fatalf("review count = %d, want 1", h.factory.reviewRepo.count())
		}
		if h.factory.assetRepo.count() != 0 {
			t.Fatalf("asset count = %d, want 0 (Hold is read-only)", h.factory.assetRepo.count())
		}
	})

	t.Run("low confidence with match sets BestMatchedAssetID", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{
				ID:           "asset-1",
				OwnerID:      testUser,
				SerialNumber: ptr("SN-123"),
				NormSerial:   ptr("SN-123"),
			},
		}
		h := newHarness(t, seedAssets, nil)
		ctx := testCtx()

		conf := 0.3
		created, err := h.svc.Hold(ctx, repo.HoldInput{
			Extraction: entity.Extraction{
				Classification: "invoice",
				SerialNumber:   ptr("SN-123"),
				Confidence:     &conf,
				RawPayload:     defaultRaw,
			},
			SourceID: "src-2",
		})
		if err != nil {
			t.Fatalf("Hold returned error %v, want nil", err)
		}
		if created.BestMatchedAssetID == nil {
			t.Fatalf("BestMatchedAssetID = nil, want \"asset-1\"")
		}
		if *created.BestMatchedAssetID != "asset-1" {
			t.Fatalf("BestMatchedAssetID = %q, want \"asset-1\"", *created.BestMatchedAssetID)
		}
	})

	t.Run("absent confidence stored as nil", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil, nil)
		ctx := testCtx()

		created, err := h.svc.Hold(ctx, repo.HoldInput{
			Extraction: entity.Extraction{
				Classification: "invoice",
				SerialNumber:   ptr("SN-555"),
				// Confidence intentionally nil
				RawPayload: defaultRaw,
			},
			SourceID: "src-3",
		})
		if err != nil {
			t.Fatalf("Hold returned error %v, want nil", err)
		}
		if created.Confidence != nil {
			t.Fatalf("Confidence = %v, want nil (fail-safe)", *created.Confidence)
		}
	})

	t.Run("no identity returns ErrNoIdentity", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil, nil)
		ctx := testCtx()

		_, err := h.svc.Hold(ctx, repo.HoldInput{
			Extraction: entity.Extraction{
				Classification: "invoice",
				// No serial, no brand, no model
				RawPayload: defaultRaw,
			},
			SourceID: "src-4",
		})
		if !errors.Is(err, identity.ErrNoIdentity) {
			t.Fatalf("Hold error = %v, want errors.Is identity.ErrNoIdentity", err)
		}
		if h.factory.reviewRepo.count() != 0 {
			t.Fatalf("review count = %d, want 0 (no review created)", h.factory.reviewRepo.count())
		}
	})
}

// ---------------------------------------------------------------------------
// TestApprove
// ---------------------------------------------------------------------------

func TestApprove(t *testing.T) {
	t.Parallel()

	t.Run("merge into matched asset", func(t *testing.T) {
		t.Parallel()
		assetID := "asset-1"
		seedAssets := []entity.Asset{
			{
				ID:           "asset-1",
				OwnerID:      testUser,
				SerialNumber: ptr("SN-123"),
				NormSerial:   ptr("SN-123"),
				Brand:        ptr("LG"),
				NormBrand:    ptr("lg"),
			},
		}
		seedReviews := []entity.IngestReview{
			{
				ID:                 "rev-1",
				OwnerID:            testUser,
				SourceID:           "src-10",
				DocType:            "invoice",
				CandidateFields:    map[string]any{"classification": "invoice", "serial_number": "SN-123", "price": "50000.00"},
				RawExtraction:      defaultRaw,
				State:              entity.ReviewStatePending,
				BestMatchedAssetID: &assetID,
			},
		}
		h := newHarness(t, seedAssets, seedReviews)
		ctx := testCtx()

		asset, updated, err := h.svc.Approve(ctx, "rev-1")
		if err != nil {
			t.Fatalf("Approve returned error %v, want nil", err)
		}
		if asset.ID != "asset-1" {
			t.Fatalf("asset ID = %q, want \"asset-1\" (merged into existing)", asset.ID)
		}
		if updated.State != entity.ReviewStateApproved {
			t.Fatalf("review State = %q, want approved", updated.State)
		}
		if updated.DecidedAt == nil {
			t.Fatalf("DecidedAt = nil, want set")
		}
		if updated.DecidedBy == nil || *updated.DecidedBy != testUser {
			t.Fatalf("DecidedBy = %v, want %q", updated.DecidedBy, testUser)
		}
		if h.factory.docRepo.count() != 1 {
			t.Fatalf("document count = %d, want 1", h.factory.docRepo.count())
		}
		docs := h.factory.docRepo.all()
		if docs[0].AssetID != "asset-1" {
			t.Fatalf("doc AssetID = %q, want \"asset-1\"", docs[0].AssetID)
		}
		// Verify the asset was updated (price merged in).
		a, ok := h.factory.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found in repo after Approve")
		}
		if a.Price == nil || *a.Price != "50000.00" {
			t.Fatalf("asset Price = %v, want \"50000.00\" (merged)", a.Price)
		}
	})

	t.Run("create new asset", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:                 "rev-1",
				OwnerID:            testUser,
				SourceID:           "src-20",
				DocType:            "warranty",
				CandidateFields:    map[string]any{"classification": "warranty", "serial_number": "SN-777"},
				RawExtraction:      defaultRaw,
				State:              entity.ReviewStatePending,
				BestMatchedAssetID: nil, // no match → new asset
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		asset, updated, err := h.svc.Approve(ctx, "rev-1")
		if err != nil {
			t.Fatalf("Approve returned error %v, want nil", err)
		}
		if asset.ID == "" {
			t.Fatalf("asset ID = empty, want a newly-created ID")
		}
		if updated.State != entity.ReviewStateApproved {
			t.Fatalf("review State = %q, want approved", updated.State)
		}
		if updated.DecidedAt == nil {
			t.Fatalf("DecidedAt = nil, want set")
		}
		if updated.DecidedBy == nil || *updated.DecidedBy != testUser {
			t.Fatalf("DecidedBy = %v, want %q", updated.DecidedBy, testUser)
		}
		if h.factory.assetRepo.count() != 1 {
			t.Fatalf("asset count = %d, want 1", h.factory.assetRepo.count())
		}
		if h.factory.docRepo.count() != 1 {
			t.Fatalf("document count = %d, want 1", h.factory.docRepo.count())
		}
		docs := h.factory.docRepo.all()
		if docs[0].AssetID != asset.ID {
			t.Fatalf("doc AssetID = %q, want %q (new asset ID)", docs[0].AssetID, asset.ID)
		}
	})

	t.Run("non-pending already approved returns ErrConflict", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:            "rev-1",
				OwnerID:       testUser,
				SourceID:      "src-30",
				DocType:       "invoice",
				CandidateFields: map[string]any{"classification": "invoice"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStateApproved,
				DecidedAt:       ptr(time.Now().UTC()),
				DecidedBy:       ptr(testUser),
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		_, _, err := h.svc.Approve(ctx, "rev-1")
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("Approve error = %v, want errors.Is ErrConflict", err)
		}
		if h.factory.assetRepo.count() != 0 {
			t.Fatalf("asset count = %d, want 0 (no asset created)", h.factory.assetRepo.count())
		}
		if h.factory.docRepo.count() != 0 {
			t.Fatalf("document count = %d, want 0 (no document created)", h.factory.docRepo.count())
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil, nil)
		ctx := testCtx()

		_, _, err := h.svc.Approve(ctx, "unknown-id")
		if !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Approve error = %v, want errors.Is repo.ErrNotFound", err)
		}
	})

	t.Run("document create failure rolls back", func(t *testing.T) {
		t.Parallel()
		assetID := "asset-1"
		seedAssets := []entity.Asset{
			{
				ID:           "asset-1",
				OwnerID:      testUser,
				SerialNumber: ptr("SN-123"),
				NormSerial:   ptr("SN-123"),
				Brand:        ptr("LG"),
				NormBrand:    ptr("lg"),
				Price:        ptr("100.00"),
			},
		}
		seedReviews := []entity.IngestReview{
			{
				ID:                 "rev-1",
				OwnerID:            testUser,
				SourceID:           "src-40",
				DocType:            "invoice",
				CandidateFields:    map[string]any{"classification": "invoice", "serial_number": "SN-123", "price": "200.00"},
				RawExtraction:      defaultRaw,
				State:              entity.ReviewStatePending,
				BestMatchedAssetID: &assetID,
			},
		}
		h := newHarness(t, seedAssets, seedReviews)
		h.factory.docRepo.createErr = errDocInsert
		ctx := testCtx()

		_, _, err := h.svc.Approve(ctx, "rev-1")
		if !errors.Is(err, errDocInsert) {
			t.Fatalf("Approve error = %v, want errors.Is errDocInsert", err)
		}
		// Review must be back to pending (rolled back).
		rev, ok := h.factory.reviewRepo.get("rev-1")
		if !ok {
			t.Fatalf("rev-1 not found after rollback")
		}
		if rev.State != entity.ReviewStatePending {
			t.Fatalf("review State = %q, want pending (rolled back)", rev.State)
		}
		// Asset must be unchanged (rolled back).
		a, ok := h.factory.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after rollback")
		}
		if a.Price == nil || *a.Price != "100.00" {
			t.Fatalf("asset Price = %v, want \"100.00\" (unchanged, rolled back)", a.Price)
		}
		// No document should exist.
		if h.factory.docRepo.count() != 0 {
			t.Fatalf("document count = %d, want 0 (rolled back)", h.factory.docRepo.count())
		}
	})
}

// ---------------------------------------------------------------------------
// TestReject
// ---------------------------------------------------------------------------

func TestReject(t *testing.T) {
	t.Parallel()

	t.Run("pending to rejected", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:              "rev-1",
				OwnerID:         testUser,
				SourceID:        "src-50",
				DocType:         "invoice",
				CandidateFields: map[string]any{"classification": "invoice", "serial_number": "SN-123"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStatePending,
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		updated, err := h.svc.Reject(ctx, "rev-1")
		if err != nil {
			t.Fatalf("Reject returned error %v, want nil", err)
		}
		if updated.State != entity.ReviewStateRejected {
			t.Fatalf("State = %q, want rejected", updated.State)
		}
		if updated.DecidedAt == nil {
			t.Fatalf("DecidedAt = nil, want set")
		}
		if updated.DecidedBy == nil || *updated.DecidedBy != testUser {
			t.Fatalf("DecidedBy = %v, want %q", updated.DecidedBy, testUser)
		}
		if h.factory.assetRepo.count() != 0 {
			t.Fatalf("asset count = %d, want 0 (no asset created on reject)", h.factory.assetRepo.count())
		}
		if h.factory.docRepo.count() != 0 {
			t.Fatalf("document count = %d, want 0 (no document created on reject)", h.factory.docRepo.count())
		}
	})

	t.Run("non-pending already rejected returns ErrConflict", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:              "rev-1",
				OwnerID:         testUser,
				SourceID:        "src-60",
				DocType:         "invoice",
				CandidateFields: map[string]any{"classification": "invoice"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStateRejected,
				DecidedAt:       ptr(time.Now().UTC()),
				DecidedBy:       ptr(testUser),
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		_, err := h.svc.Reject(ctx, "rev-1")
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("Reject error = %v, want errors.Is ErrConflict", err)
		}
		if h.factory.assetRepo.count() != 0 {
			t.Fatalf("asset count = %d, want 0", h.factory.assetRepo.count())
		}
		if h.factory.docRepo.count() != 0 {
			t.Fatalf("document count = %d, want 0", h.factory.docRepo.count())
		}
	})

	t.Run("source retained no document or asset created", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:              "rev-1",
				OwnerID:         testUser,
				SourceID:        "src-70",
				DocType:         "invoice",
				CandidateFields: map[string]any{"classification": "invoice", "serial_number": "SN-888"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStatePending,
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		_, err := h.svc.Reject(ctx, "rev-1")
		if err != nil {
			t.Fatalf("Reject returned error %v, want nil", err)
		}
		// No document created.
		if h.factory.docRepo.count() != 0 {
			t.Fatalf("document count = %d, want 0 (source retained, no doc)", h.factory.docRepo.count())
		}
		// No asset modified or created.
		if h.factory.assetRepo.count() != 0 {
			t.Fatalf("asset count = %d, want 0 (no asset touched)", h.factory.assetRepo.count())
		}
	})
}

// ---------------------------------------------------------------------------
// TestList
// ---------------------------------------------------------------------------

func TestList(t *testing.T) {
	t.Parallel()

	t.Run("default empty status returns only pending", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:              "rev-1",
				OwnerID:         testUser,
				DocType:         "invoice",
				CandidateFields: map[string]any{"classification": "invoice"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStatePending,
				CreatedAt:       time.Now().UTC().Add(-3 * time.Minute),
			},
			{
				ID:              "rev-2",
				OwnerID:         testUser,
				DocType:         "warranty",
				CandidateFields: map[string]any{"classification": "warranty"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStateApproved,
				CreatedAt:       time.Now().UTC().Add(-2 * time.Minute),
				DecidedAt:       ptr(time.Now().UTC()),
				DecidedBy:       ptr(testUser),
			},
			{
				ID:              "rev-3",
				OwnerID:         testUser,
				DocType:         "amc",
				CandidateFields: map[string]any{"classification": "amc"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStateRejected,
				CreatedAt:       time.Now().UTC().Add(-1 * time.Minute),
				DecidedAt:       ptr(time.Now().UTC()),
				DecidedBy:       ptr(testUser),
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		reviews, err := h.svc.List(ctx, "")
		if err != nil {
			t.Fatalf("List returned error %v, want nil", err)
		}
		if len(reviews) != 1 {
			t.Fatalf("List returned %d reviews, want 1 (only pending)", len(reviews))
		}
		if reviews[0].ID != "rev-1" {
			t.Fatalf("reviews[0].ID = %q, want \"rev-1\"", reviews[0].ID)
		}
	})

	t.Run("filter by rejected status", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:              "rev-1",
				OwnerID:         testUser,
				DocType:         "invoice",
				CandidateFields: map[string]any{"classification": "invoice"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStatePending,
			},
			{
				ID:              "rev-2",
				OwnerID:         testUser,
				DocType:         "warranty",
				CandidateFields: map[string]any{"classification": "warranty"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStateRejected,
				DecidedAt:       ptr(time.Now().UTC()),
				DecidedBy:       ptr(testUser),
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		reviews, err := h.svc.List(ctx, entity.ReviewStateRejected)
		if err != nil {
			t.Fatalf("List returned error %v, want nil", err)
		}
		if len(reviews) != 1 {
			t.Fatalf("List returned %d reviews, want 1 (only rejected)", len(reviews))
		}
		if reviews[0].ID != "rev-2" {
			t.Fatalf("reviews[0].ID = %q, want \"rev-2\"", reviews[0].ID)
		}
	})

	t.Run("empty returns non-nil empty slice", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil, nil)
		ctx := testCtx()

		reviews, err := h.svc.List(ctx, "")
		if err != nil {
			t.Fatalf("List returned error %v, want nil", err)
		}
		if reviews == nil {
			t.Fatalf("List returned nil, want non-nil empty slice")
		}
		if len(reviews) != 0 {
			t.Fatalf("List returned %d reviews, want 0", len(reviews))
		}
	})

	t.Run("cross-owner reviews excluded", func(t *testing.T) {
		t.Parallel()
		// Seed a review belonging to another owner.
		seedReviews := []entity.IngestReview{
			{
				ID:              "rev-1",
				OwnerID:         testUser,
				DocType:         "invoice",
				CandidateFields: map[string]any{"classification": "invoice"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStatePending,
			},
		}
		h := newHarness(t, nil, seedReviews)
		// Manually add another owner's review to the repo.
		h.factory.reviewRepo.reviews["rev-other"] = entity.IngestReview{
			ID:              "rev-other",
			OwnerID:         otherUser,
			DocType:         "invoice",
			CandidateFields: map[string]any{"classification": "invoice"},
			RawExtraction:   defaultRaw,
			State:           entity.ReviewStatePending,
		}
		ctx := testCtx()

		reviews, err := h.svc.List(ctx, "")
		if err != nil {
			t.Fatalf("List returned error %v, want nil", err)
		}
		for _, rev := range reviews {
			if rev.OwnerID == otherUser {
				t.Fatalf("List returned a review for %q, want only %q reviews", otherUser, testUser)
			}
		}
		if len(reviews) != 1 {
			t.Fatalf("List returned %d reviews, want 1 (only testUser's)", len(reviews))
		}
	})
}

// ---------------------------------------------------------------------------
// TestGet
// ---------------------------------------------------------------------------

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("existing review in scope", func(t *testing.T) {
		t.Parallel()
		seedReviews := []entity.IngestReview{
			{
				ID:              "rev-1",
				OwnerID:         testUser,
				SourceID:        "src-100",
				DocType:         "invoice",
				CandidateFields: map[string]any{"classification": "invoice", "serial_number": "SN-123"},
				RawExtraction:   defaultRaw,
				State:           entity.ReviewStatePending,
			},
		}
		h := newHarness(t, nil, seedReviews)
		ctx := testCtx()

		rev, err := h.svc.Get(ctx, "rev-1")
		if err != nil {
			t.Fatalf("Get returned error %v, want nil", err)
		}
		if rev.ID != "rev-1" {
			t.Fatalf("rev.ID = %q, want \"rev-1\"", rev.ID)
		}
		if rev.State != entity.ReviewStatePending {
			t.Fatalf("rev.State = %q, want pending", rev.State)
		}
		if rev.SourceID != "src-100" {
			t.Fatalf("rev.SourceID = %q, want \"src-100\"", rev.SourceID)
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil, nil)
		ctx := testCtx()

		_, err := h.svc.Get(ctx, "unknown-id")
		if !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Get error = %v, want errors.Is repo.ErrNotFound", err)
		}
	})

	t.Run("cross-owner returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil, nil)
		// Seed a review belonging to another owner.
		h.factory.reviewRepo.reviews["rev-other"] = entity.IngestReview{
			ID:              "rev-other",
			OwnerID:         otherUser,
			DocType:         "invoice",
			CandidateFields: map[string]any{"classification": "invoice"},
			RawExtraction:   defaultRaw,
			State:           entity.ReviewStatePending,
		}
		ctx := testCtx()

		_, err := h.svc.Get(ctx, "rev-other")
		if !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Get error = %v, want errors.Is repo.ErrNotFound (cross-owner)", err)
		}
	})
}
