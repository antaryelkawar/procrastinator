package lifecycle

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
)

// testUser is the user ID used by all lifecycle tests.
const testUser = "test-user"

// otherUser is a distinct user used to verify cross-owner isolation.
const otherUser = "other-user"

// testNow is the canonical clock value used across tests.
var testNow = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

// Compile-time interface guards.
var (
	_ repo.Repository[entity.Asset]    = (*fakeAssetRepo)(nil)
	_ repo.Repository[entity.Document] = (*fakeDocumentRepo)(nil)
	_ repo.Repository[entity.Source]   = (*fakeSourceRepo)(nil)
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

// ---------------------------------------------------------------------------
// fakeAssetRepo
// ---------------------------------------------------------------------------

// fakeAssetRepo is an in-memory implementation of repo.Repository[entity.Asset].
type fakeAssetRepo struct {
	mu     sync.Mutex
	assets map[string]entity.Asset
	nextID int
	saved  map[string]entity.Asset
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
	return out, nil
}

// matchAssetFilter reports whether asset a satisfies filter f on deleted_at.
// It supports "IS NULL", "=", and "<" on deleted_at (value is time.Time).
func matchAssetFilter(a entity.Asset, f repo.Filter) bool {
	if f.Field != "deleted_at" {
		return false
	}
	switch f.Op {
	case "IS NULL":
		return a.DeletedAt == nil
	case "=":
		want, ok := f.Value.(time.Time)
		if !ok {
			return false
		}
		return a.DeletedAt != nil && a.DeletedAt.Equal(want)
	case "<":
		want, ok := f.Value.(time.Time)
		if !ok {
			return false
		}
		return a.DeletedAt != nil && a.DeletedAt.Before(want)
	}
	return false
}

func (r *fakeAssetRepo) Create(_ context.Context, a entity.Asset, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == "" {
		r.nextID++
		a.ID = "asset-" + strconv.Itoa(r.nextID)
	}
	if tid := ownerFromOpts(opts); tid != "" {
		a.OwnerID = tid
	}
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
	applyAssetUpdateSets(&a, opts)
	r.assets[a.ID] = a
	return a, nil
}

// applyAssetUpdateSets applies explicit UpdateSets from Set() to the asset.
func applyAssetUpdateSets(a *entity.Asset, opts []repo.Option) {
	s := repo.ApplyOptions(opts...).UpdateSets
	if s == nil {
		return
	}
	for field, val := range s {
		switch field {
		case "deleted_at":
			if val == nil {
				a.DeletedAt = nil
			} else if t, ok := val.(*time.Time); ok {
				a.DeletedAt = t
			}
		}
	}
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
		match := true
		for _, f := range o.Filters {
			if !matchDocFilter(d, f) {
				match = false
				break
			}
		}
		if match {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// matchDocFilter reports whether document d satisfies filter f on asset_id.
// It supports "=" on asset_id (value is string).
func matchDocFilter(d entity.Document, f repo.Filter) bool {
	if f.Field != "asset_id" {
		return false
	}
	want, ok := f.Value.(string)
	if !ok {
		return false
	}
	return d.AssetID == want
}

func (r *fakeDocumentRepo) Create(_ context.Context, d entity.Document, opts ...repo.Option) (entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.ID == "" {
		r.nextID++
		d.ID = "doc-" + strconv.Itoa(r.nextID)
	}
	if tid := ownerFromOpts(opts); tid != "" {
		d.OwnerID = tid
	}
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
	applyDocUpdateSets(&d, opts)
	r.documents[d.ID] = d
	return d, nil
}

// applyDocUpdateSets applies explicit UpdateSets from Set() to the document.
func applyDocUpdateSets(d *entity.Document, opts []repo.Option) {
	s := repo.ApplyOptions(opts...).UpdateSets
	if s == nil {
		return
	}
	for field, val := range s {
		switch field {
		case "asset_id":
			if val == nil {
				d.AssetID = ""
			} else if v, ok := val.(string); ok {
				d.AssetID = v
			}
		}
	}
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
		r.documents[id] = c
	}
	r.saved = nil
}

// ---------------------------------------------------------------------------
// fakeSourceRepo
// ---------------------------------------------------------------------------

// fakeSourceRepo is a minimal in-memory implementation of
// repo.Repository[entity.Source]. It is only used to assert sources are
// retained by purge.
type fakeSourceRepo struct {
	mu      sync.Mutex
	sources map[string]entity.Source
	nextID  int
	saved   map[string]entity.Source
}

func newFakeSourceRepo() *fakeSourceRepo {
	return &fakeSourceRepo{sources: make(map[string]entity.Source)}
}

func (r *fakeSourceRepo) Get(_ context.Context, id string, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	if !ok {
		return entity.Source{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" && s.OwnerID != tid {
		return entity.Source{}, repo.ErrNotFound
	}
	return s, nil
}

func (r *fakeSourceRepo) List(_ context.Context, opts ...repo.Option) ([]entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	out := make([]entity.Source, 0, len(r.sources))
	for _, s := range r.sources {
		if o.OwnerID != "" && s.OwnerID != o.OwnerID {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeSourceRepo) Create(_ context.Context, s entity.Source, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		r.nextID++
		s.ID = "src-" + strconv.Itoa(r.nextID)
	}
	if tid := ownerFromOpts(opts); tid != "" {
		s.OwnerID = tid
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *fakeSourceRepo) Update(_ context.Context, s entity.Source, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[s.ID]; !ok {
		return entity.Source{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" {
		s.OwnerID = tid
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *fakeSourceRepo) Delete(_ context.Context, id string, _ ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.sources, id)
	return nil
}

// count returns the number of stored sources.
func (r *fakeSourceRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sources)
}

// snapshot deep-copies the source map for transaction rollback support.
func (r *fakeSourceRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]entity.Source, len(r.sources))
	for id, s := range r.sources {
		c := s
		saved[id] = c
	}
	r.saved = saved
}

// rollback restores the source map from the last snapshot.
func (r *fakeSourceRepo) rollback() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		return
	}
	r.sources = make(map[string]entity.Source, len(r.saved))
	for id, s := range r.saved {
		c := s
		r.sources[id] = c
	}
	r.saved = nil
}

// ---------------------------------------------------------------------------
// fakeFactory
// ---------------------------------------------------------------------------

// fakeFactory builds a concrete *repo.Factory from the fakes. InTx snapshots
// all three repos, builds a *repo.Repos bundle from the same fakes, runs the
// callback, and rolls all three back on error (leaving them intact on
// success).
type fakeFactory struct {
	factory   *repo.Factory
	assetRepo *fakeAssetRepo
	docRepo   *fakeDocumentRepo
	srcRepo   *fakeSourceRepo
}

func newFakeFactory(assetRepo *fakeAssetRepo, docRepo *fakeDocumentRepo, srcRepo *fakeSourceRepo) *fakeFactory {
	f := &repo.Factory{
		Assets:    assetRepo,
		Documents: docRepo,
		Sources:   srcRepo,
		InTx: func(ctx context.Context, fn func(ctx context.Context, repos *repo.Repos) error) error {
			assetRepo.snapshot()
			docRepo.snapshot()
			srcRepo.snapshot()
			repos := &repo.Repos{
				Assets:    assetRepo,
				Documents: docRepo,
				Sources:   srcRepo,
			}
			if err := fn(ctx, repos); err != nil {
				assetRepo.rollback()
				docRepo.rollback()
				srcRepo.rollback()
				return err
			}
			return nil
		},
	}
	return &fakeFactory{
		factory:   f,
		assetRepo: assetRepo,
		docRepo:   docRepo,
		srcRepo:   srcRepo,
	}
}

// ---------------------------------------------------------------------------
// harness
// ---------------------------------------------------------------------------

// harness wires the fakes together with the Service under test.
type harness struct {
	svc *Service
	ff  *fakeFactory
}

// newHarness seeds the fakes (assigning OwnerID = testUser when empty), builds
// the fake factory, and constructs the Service with the given retention window
// and clock.
func newHarness(t *testing.T, retentionDays int, now func() time.Time, seedAssets []entity.Asset, seedDocs []entity.Document, seedSources []entity.Source) *harness {
	t.Helper()

	assetRepo := newFakeAssetRepo()
	docRepo := newFakeDocumentRepo()
	srcRepo := newFakeSourceRepo()

	for _, a := range seedAssets {
		if a.OwnerID == "" {
			a.OwnerID = testUser
		}
		assetRepo.assets[a.ID] = a
	}
	for _, d := range seedDocs {
		if d.OwnerID == "" {
			d.OwnerID = testUser
		}
		docRepo.documents[d.ID] = d
	}
	for _, s := range seedSources {
		if s.OwnerID == "" {
			s.OwnerID = testUser
		}
		srcRepo.sources[s.ID] = s
	}

	ff := newFakeFactory(assetRepo, docRepo, srcRepo)
	svc := New(ff.factory, retentionDays)
	svc.now = now
	return &harness{svc: svc, ff: ff}
}

// ---------------------------------------------------------------------------
// TestSoftDelete
// ---------------------------------------------------------------------------

func TestSoftDelete(t *testing.T) {
	t.Parallel()

	t.Run("sets deleted_at and retains the row", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, Brand: ptr("LG"), Model: ptr("WM-2000"), Name: ptr("Microwave")},
		}
		seedDocs := []entity.Document{
			{ID: "doc-1", OwnerID: testUser, SourceID: "src-1", AssetID: "asset-1", DocType: "invoice"},
		}
		h := newHarness(t, 30, func() time.Time { return testNow.Add(1 * time.Hour) }, seedAssets, seedDocs, nil)

		got, err := h.svc.SoftDelete(testCtx(), "asset-1")
		if err != nil {
			t.Fatalf("SoftDelete returned error %v, want nil", err)
		}
		if got.DeletedAt == nil {
			t.Fatalf("returned DeletedAt = nil, want non-nil")
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after SoftDelete (row must be retained)")
		}
		if a.DeletedAt == nil {
			t.Fatalf("stored DeletedAt = nil, want non-nil (soft delete, no removal)")
		}
		d, ok := h.ff.docRepo.get("doc-1")
		if !ok {
			t.Fatalf("doc-1 not found after SoftDelete")
		}
		if d.AssetID != "asset-1" {
			t.Fatalf("doc AssetID = %q, want \"asset-1\" (soft delete must NOT detach docs)", d.AssetID)
		}
	})

	t.Run("excludes the asset from the active-only list", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-A", OwnerID: testUser, Brand: ptr("LG"), Model: ptr("WM-2000")},
			{ID: "asset-B", OwnerID: testUser, Brand: ptr("Bosch"), Model: ptr("WAT")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow.Add(1 * time.Hour) }, seedAssets, nil, nil)

		if _, err := h.svc.SoftDelete(testCtx(), "asset-A"); err != nil {
			t.Fatalf("SoftDelete returned error %v, want nil", err)
		}
		list, err := h.ff.assetRepo.List(testCtx(), repo.Owner(testUser), repo.Where("deleted_at", "IS NULL", nil))
		if err != nil {
			t.Fatalf("List returned error %v, want nil", err)
		}
		if len(list) != 1 || list[0].ID != "asset-B" {
			var ids []string
			for _, a := range list {
				ids = append(ids, a.ID)
			}
			t.Fatalf("active list = %v, want [asset-B]", ids)
		}
	})

	t.Run("unknown id returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, 30, func() time.Time { return testNow }, nil, nil, nil)
		if _, err := h.svc.SoftDelete(testCtx(), "does-not-exist"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("SoftDelete error = %v, want errors.Is repo.ErrNotFound", err)
		}
		if h.ff.assetRepo.count() != 0 {
			t.Fatalf("asset count = %d, want 0 (no write on not-found)", h.ff.assetRepo.count())
		}
	})

	t.Run("other-owner asset returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{{ID: "asset-x", OwnerID: otherUser, Brand: ptr("LG")}}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)
		if _, err := h.svc.SoftDelete(testCtx(), "asset-x"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("SoftDelete error = %v, want errors.Is repo.ErrNotFound", err)
		}
		if h.ff.assetRepo.count() != 1 {
			t.Fatalf("asset count = %d, want 1 (no write on cross-owner)", h.ff.assetRepo.count())
		}
	})

	t.Run("idempotent when already deleted", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{{ID: "asset-1", OwnerID: testUser, DeletedAt: ptr(testNow)}}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)
		got, err := h.svc.SoftDelete(testCtx(), "asset-1")
		if err != nil {
			t.Fatalf("SoftDelete returned error %v, want nil", err)
		}
		if got.DeletedAt == nil {
			t.Fatalf("returned DeletedAt = nil, want non-nil")
		}
		if h.ff.assetRepo.count() != 1 {
			t.Fatalf("asset count = %d, want 1", h.ff.assetRepo.count())
		}
	})

	t.Run("no user in context returns ErrNoUser", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, 30, func() time.Time { return testNow }, nil, nil, nil)
		if _, err := h.svc.SoftDelete(context.Background(), "asset-1"); !errors.Is(err, user.ErrNoUser) {
			t.Fatalf("SoftDelete error = %v, want errors.Is user.ErrNoUser", err)
		}
	})
}

// ---------------------------------------------------------------------------
// TestRestore
// ---------------------------------------------------------------------------

func TestRestore(t *testing.T) {
	t.Parallel()

	t.Run("within window restores fields and keeps document links", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{
				ID:            "asset-1",
				OwnerID:       testUser,
				DeletedAt:     ptr(testNow.Add(-5 * 24 * time.Hour)),
				Brand:         ptr("LG"),
				Model:         ptr("WM-2000"),
				Name:          ptr("Microwave"),
				AssetCategory: ptr("appliance"),
			},
		}
		seedDocs := []entity.Document{
			{ID: "doc-1", OwnerID: testUser, SourceID: "src-1", AssetID: "asset-1", DocType: "invoice"},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, seedDocs, nil)

		got, err := h.svc.Restore(testCtx(), "asset-1")
		if err != nil {
			t.Fatalf("Restore returned error %v, want nil", err)
		}
		if got.DeletedAt != nil {
			t.Fatalf("returned DeletedAt = %v, want nil", *got.DeletedAt)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after Restore")
		}
		if a.DeletedAt != nil {
			t.Fatalf("stored DeletedAt = %v, want nil", *a.DeletedAt)
		}
		if a.Brand == nil || *a.Brand != "LG" || a.Model == nil || *a.Model != "WM-2000" {
			t.Fatalf("Brand/Model changed after Restore: Brand=%v Model=%v", a.Brand, a.Model)
		}
		if a.Name == nil || *a.Name != "Microwave" {
			t.Fatalf("Name changed after Restore: Name=%v", a.Name)
		}
		if a.AssetCategory == nil || *a.AssetCategory != "appliance" {
			t.Fatalf("AssetCategory changed after Restore: %v", a.AssetCategory)
		}
		d, ok := h.ff.docRepo.get("doc-1")
		if !ok {
			t.Fatalf("doc-1 not found after Restore")
		}
		if d.AssetID != "asset-1" {
			t.Fatalf("doc AssetID = %q, want \"asset-1\" (Restore must NOT detach docs)", d.AssetID)
		}
	})

	t.Run("beyond window returns ErrConflict", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, DeletedAt: ptr(testNow.Add(-35 * 24 * time.Hour))},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)
		if _, err := h.svc.Restore(testCtx(), "asset-1"); !errors.Is(err, ErrConflict) {
			t.Fatalf("Restore error = %v, want errors.Is ErrConflict", err)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after Restore")
		}
		if a.DeletedAt == nil {
			t.Fatalf("stored DeletedAt = nil, want non-nil (asset untouched beyond window)")
		}
	})

	t.Run("not-deleted is a no-op", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{{ID: "asset-1", OwnerID: testUser, Brand: ptr("LG")}}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)
		got, err := h.svc.Restore(testCtx(), "asset-1")
		if err != nil {
			t.Fatalf("Restore returned error %v, want nil", err)
		}
		if got.DeletedAt != nil {
			t.Fatalf("returned DeletedAt = %v, want nil (active asset unchanged)", *got.DeletedAt)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found")
		}
		if a.DeletedAt != nil {
			t.Fatalf("stored DeletedAt = %v, want nil", *a.DeletedAt)
		}
	})

	t.Run("unknown id returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, 30, func() time.Time { return testNow }, nil, nil, nil)
		if _, err := h.svc.Restore(testCtx(), "nope"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Restore error = %v, want errors.Is repo.ErrNotFound", err)
		}
	})

	t.Run("other-owner asset returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{{ID: "asset-x", OwnerID: otherUser, DeletedAt: ptr(testNow)}}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)
		if _, err := h.svc.Restore(testCtx(), "asset-x"); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("Restore error = %v, want errors.Is repo.ErrNotFound", err)
		}
		if h.ff.assetRepo.count() != 1 {
			t.Fatalf("asset count = %d, want 1 (no write on cross-owner)", h.ff.assetRepo.count())
		}
	})

	t.Run("no user in context returns ErrNoUser", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, 30, func() time.Time { return testNow }, nil, nil, nil)
		if _, err := h.svc.Restore(context.Background(), "asset-1"); !errors.Is(err, user.ErrNoUser) {
			t.Fatalf("Restore error = %v, want errors.Is user.ErrNoUser", err)
		}
	})

	t.Run("at the exact boundary is still restorable", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, DeletedAt: ptr(testNow.Add(-30 * 24 * time.Hour)), Brand: ptr("LG")},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, nil, nil)

		if _, err := h.svc.Restore(testCtx(), "asset-1"); err != nil {
			t.Fatalf("Restore returned error %v, want nil (exact boundary is restorable)", err)
		}
		a, ok := h.ff.assetRepo.get("asset-1")
		if !ok {
			t.Fatalf("asset-1 not found after Restore")
		}
		if a.DeletedAt != nil {
			t.Fatalf("stored DeletedAt = %v, want nil (cleared)", *a.DeletedAt)
		}
	})
}

// ---------------------------------------------------------------------------
// TestPurgeExpired
// ---------------------------------------------------------------------------

func TestPurgeExpired(t *testing.T) {
	t.Parallel()

	t.Run("removes only expired soft-deleted, nulls their doc links, retains docs and sources", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-A", OwnerID: testUser, DeletedAt: ptr(testNow.Add(-40 * 24 * time.Hour))},
			{ID: "asset-B", OwnerID: testUser, DeletedAt: ptr(testNow.Add(-5 * 24 * time.Hour))},
			{ID: "asset-C", OwnerID: testUser},
		}
		seedDocs := []entity.Document{
			{ID: "docA1", OwnerID: testUser, SourceID: "src-A1", AssetID: "asset-A", DocType: "invoice"},
			{ID: "docA2", OwnerID: testUser, SourceID: "src-A2", AssetID: "asset-A", DocType: "invoice"},
			{ID: "docB1", OwnerID: testUser, SourceID: "src-B1", AssetID: "asset-B", DocType: "invoice"},
			{ID: "docC1", OwnerID: testUser, SourceID: "src-C1", AssetID: "asset-C", DocType: "invoice"},
		}
		seedSources := []entity.Source{
			{ID: "src-A1", OwnerID: testUser, Filename: "a1.pdf"},
			{ID: "src-A2", OwnerID: testUser, Filename: "a2.pdf"},
			{ID: "src-B1", OwnerID: testUser, Filename: "b1.pdf"},
			{ID: "src-C1", OwnerID: testUser, Filename: "c1.pdf"},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, seedDocs, seedSources)

		purged, err := h.svc.PurgeExpired(testCtx())
		if err != nil {
			t.Fatalf("PurgeExpired returned error %v, want nil", err)
		}
		if purged != 1 {
			t.Fatalf("purged = %d, want 1 (only asset-A expired)", purged)
		}
		if _, ok := h.ff.assetRepo.get("asset-A"); ok {
			t.Fatalf("asset-A still present after purge (should be hard-deleted)")
		}
		b, ok := h.ff.assetRepo.get("asset-B")
		if !ok {
			t.Fatalf("asset-B missing after purge")
		}
		if b.DeletedAt == nil {
			t.Fatalf("asset-B DeletedAt = nil, want non-nil (not purged, within window)")
		}
		c, ok := h.ff.assetRepo.get("asset-C")
		if !ok {
			t.Fatalf("asset-C missing after purge")
		}
		if c.DeletedAt != nil {
			t.Fatalf("asset-C DeletedAt = %v, want nil (active, not purged)", *c.DeletedAt)
		}
		if _, ok := h.ff.docRepo.get("docA1"); !ok {
			t.Fatalf("docA1 missing (doc rows must be retained)")
		}
		if _, ok := h.ff.docRepo.get("docA2"); !ok {
			t.Fatalf("docA2 missing (doc rows must be retained)")
		}
		dA1, _ := h.ff.docRepo.get("docA1")
		dA2, _ := h.ff.docRepo.get("docA2")
		if dA1.AssetID != "" {
			t.Fatalf("docA1 AssetID = %q, want \"\" (links nulled)", dA1.AssetID)
		}
		if dA2.AssetID != "" {
			t.Fatalf("docA2 AssetID = %q, want \"\" (links nulled)", dA2.AssetID)
		}
		dB1, _ := h.ff.docRepo.get("docB1")
		if dB1.AssetID != "asset-B" {
			t.Fatalf("docB1 AssetID = %q, want \"asset-B\" (unchanged)", dB1.AssetID)
		}
		dC1, _ := h.ff.docRepo.get("docC1")
		if dC1.AssetID != "asset-C" {
			t.Fatalf("docC1 AssetID = %q, want \"asset-C\" (unchanged)", dC1.AssetID)
		}
		if h.ff.docRepo.count() != 4 {
			t.Fatalf("doc count = %d, want 4 (all retained, A's just unlinked)", h.ff.docRepo.count())
		}
		if h.ff.srcRepo.count() != 4 {
			t.Fatalf("source count = %d, want 4 (purge never touches sources)", h.ff.srcRepo.count())
		}
	})

	t.Run("no-op when nothing is expired", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser},
			{ID: "asset-2", OwnerID: testUser, DeletedAt: ptr(testNow.Add(-5 * 24 * time.Hour))},
		}
		seedDocs := []entity.Document{
			{ID: "doc-1", OwnerID: testUser, SourceID: "src-1", AssetID: "asset-1", DocType: "invoice"},
			{ID: "doc-2", OwnerID: testUser, SourceID: "src-2", AssetID: "asset-2", DocType: "invoice"},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, seedDocs, nil)

		purged, err := h.svc.PurgeExpired(testCtx())
		if err != nil {
			t.Fatalf("PurgeExpired returned error %v, want nil", err)
		}
		if purged != 0 {
			t.Fatalf("purged = %d, want 0 (nothing expired)", purged)
		}
		if _, ok := h.ff.assetRepo.get("asset-1"); !ok {
			t.Fatalf("asset-1 missing after no-op purge")
		}
		if _, ok := h.ff.assetRepo.get("asset-2"); !ok {
			t.Fatalf("asset-2 missing after no-op purge")
		}
		d1, _ := h.ff.docRepo.get("doc-1")
		d2, _ := h.ff.docRepo.get("doc-2")
		if d1.AssetID != "asset-1" {
			t.Fatalf("doc-1 AssetID = %q, want \"asset-1\" (unchanged)", d1.AssetID)
		}
		if d2.AssetID != "asset-2" {
			t.Fatalf("doc-2 AssetID = %q, want \"asset-2\" (unchanged)", d2.AssetID)
		}
		if h.ff.docRepo.count() != 2 {
			t.Fatalf("doc count = %d, want 2 (unchanged)", h.ff.docRepo.count())
		}
	})

	t.Run("at the exact boundary is retained", func(t *testing.T) {
		t.Parallel()
		seedAssets := []entity.Asset{
			{ID: "asset-1", OwnerID: testUser, DeletedAt: ptr(testNow.Add(-30 * 24 * time.Hour))},
		}
		seedDocs := []entity.Document{
			{ID: "doc-1", OwnerID: testUser, SourceID: "src-1", AssetID: "asset-1", DocType: "invoice"},
		}
		h := newHarness(t, 30, func() time.Time { return testNow }, seedAssets, seedDocs, nil)

		purged, err := h.svc.PurgeExpired(testCtx())
		if err != nil {
			t.Fatalf("PurgeExpired returned error %v, want nil", err)
		}
		if purged != 0 {
			t.Fatalf("purged = %d, want 0 (exact boundary is retained)", purged)
		}
		if _, ok := h.ff.assetRepo.get("asset-1"); !ok {
			t.Fatalf("asset-1 missing after purge (exact boundary must be retained)")
		}
		d1, ok := h.ff.docRepo.get("doc-1")
		if !ok {
			t.Fatalf("doc-1 missing after purge")
		}
		if d1.AssetID != "asset-1" {
			t.Fatalf("doc-1 AssetID = %q, want \"asset-1\" (link NOT nulled)", d1.AssetID)
		}
	})
}
