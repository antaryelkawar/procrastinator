package identity

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"testing"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Compile-time checks: the dedupe fakes satisfy their generic repositories.
var (
	_ repo.Repository[entity.Source]   = (*fakeSourceRepo)(nil)
	_ repo.Repository[entity.Document] = (*fakeDocumentRepo)(nil)
)

// fakeSourceRepo is a minimal in-memory implementation of
// repo.Repository[entity.Source] that honors "=" filters on SHA256 and Limit,
// and records its Create count.
type fakeSourceRepo struct {
	mu        sync.Mutex
	sources   map[string]entity.Source
	nextID    int
	created   int
	listCalls int
	listErr   error
}

// newFakeSourceRepo returns a fakeSourceRepo seeded with the given sources.
func newFakeSourceRepo(srcs ...entity.Source) *fakeSourceRepo {
	r := &fakeSourceRepo{sources: make(map[string]entity.Source, len(srcs))}
	for _, s := range srcs {
		r.sources[s.ID] = s
	}
	return r
}

// createCount returns the number of successful Create calls.
func (r *fakeSourceRepo) createCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created
}

// listCount returns the number of List calls.
func (r *fakeSourceRepo) listCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listCalls
}

func (r *fakeSourceRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	if !ok {
		return entity.Source{}, repo.ErrNotFound
	}
	return s, nil
}

func (r *fakeSourceRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	o := repo.ApplyOptions(opts...)
	r.listCalls++
	out := make([]entity.Source, 0, len(r.sources))
	for _, s := range r.sources {
		if o.OwnerID != "" && s.OwnerID != o.OwnerID {
			continue
		}
		match := true
		for _, f := range o.Filters {
			if f.Op != "=" || f.Field != "sha256" {
				continue
			}
			if want, ok := f.Value.(string); !ok || s.SHA256 != want {
				match = false
				break
			}
		}
		if match {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (r *fakeSourceRepo) Create(ctx context.Context, s entity.Source, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created++
	if s.ID == "" {
		r.nextID++
		s.ID = "src-id-" + strconv.Itoa(r.nextID)
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *fakeSourceRepo) Update(ctx context.Context, s entity.Source, opts ...repo.Option) (entity.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[s.ID]; !ok {
		return entity.Source{}, repo.ErrNotFound
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *fakeSourceRepo) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.sources, id)
	return nil
}

// fakeDocumentRepo is a minimal in-memory implementation of
// repo.Repository[entity.Document] that honors "=" filters on AssetID and
// SourceID and Limit, and records its Create count.
type fakeDocumentRepo struct {
	mu        sync.Mutex
	documents map[string]entity.Document
	nextID    int
	created   int
	listCalls int
	listErr   error
}

// newFakeDocumentRepo returns a fakeDocumentRepo seeded with the given documents.
func newFakeDocumentRepo(docs ...entity.Document) *fakeDocumentRepo {
	r := &fakeDocumentRepo{documents: make(map[string]entity.Document, len(docs))}
	for _, d := range docs {
		r.documents[d.ID] = d
	}
	return r
}

// createCount returns the number of successful Create calls.
func (r *fakeDocumentRepo) createCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created
}

// listCount returns the number of List calls.
func (r *fakeDocumentRepo) listCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listCalls
}

func (r *fakeDocumentRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.documents[id]
	if !ok {
		return entity.Document{}, repo.ErrNotFound
	}
	return d, nil
}

func (r *fakeDocumentRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	o := repo.ApplyOptions(opts...)
	r.listCalls++
	out := make([]entity.Document, 0, len(r.documents))
	for _, d := range r.documents {
		if o.OwnerID != "" && d.OwnerID != o.OwnerID {
			continue
		}
		match := true
		for _, f := range o.Filters {
			if f.Op != "=" {
				continue
			}
			want, ok := f.Value.(string)
			if !ok {
				match = false
				break
			}
			switch f.Field {
			case "asset_id":
				if d.AssetID != want {
					match = false
				}
			case "source_id":
				if d.SourceID != want {
					match = false
				}
			default:
				match = false
			}
			if !match {
				break
			}
		}
		if match {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (r *fakeDocumentRepo) Create(ctx context.Context, d entity.Document, opts ...repo.Option) (entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created++
	if d.ID == "" {
		r.nextID++
		d.ID = "doc-id-" + strconv.Itoa(r.nextID)
	}
	r.documents[d.ID] = d
	return d, nil
}

func (r *fakeDocumentRepo) Update(ctx context.Context, d entity.Document, opts ...repo.Option) (entity.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.documents[d.ID]; !ok {
		return entity.Document{}, repo.ErrNotFound
	}
	r.documents[d.ID] = d
	return d, nil
}

func (r *fakeDocumentRepo) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.documents[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.documents, id)
	return nil
}

// TestDedupeHash_SameHashDocumentLinked seeds a source S (SHA256=H) and a
// document D (SourceID=S.ID, AssetID=A). DedupeHash must return D and true,
// and must be read-only: no source or document Create calls.
func TestDedupeHash_SameHashDocumentLinked(t *testing.T) {
	t.Parallel()

	ctx := testCtx()
	const (
		H = "hash-1"
		A = "asset-1"
	)
	src := entity.Source{ID: "S", SHA256: H, OwnerID: testUser}
	doc := entity.Document{ID: "D", SourceID: "S", AssetID: A, OwnerID: testUser}
	srcRepo := newFakeSourceRepo(src)
	docRepo := newFakeDocumentRepo(doc)
	deduper := NewDeduper(srcRepo, docRepo)

	got, found := deduper.DedupeHash(ctx, H, A)
	if !found {
		t.Fatalf("found = false, want true")
	}
	if got == nil {
		t.Fatal("got = nil, want non-nil document")
	}
	if got.ID != doc.ID {
		t.Errorf("document ID = %q, want %q", got.ID, doc.ID)
	}
	if got.AssetID != A {
		t.Errorf("document AssetID = %q, want %q", got.AssetID, A)
	}
	if got.SourceID != src.ID {
		t.Errorf("document SourceID = %q, want %q", got.SourceID, src.ID)
	}
	// DedupeHash is read-only: no second document, no source side effects.
	if n := docRepo.createCount(); n != 0 {
		t.Errorf("document Create count = %d, want 0 (no second document)", n)
	}
	if n := srcRepo.createCount(); n != 0 {
		t.Errorf("source Create count = %d, want 0 (no source side effects)", n)
	}
}

// TestDedupeHash_DifferentHash seeds source S1 (SHA256=H1) + document D
// (SourceID=S1.ID, AssetID=A). DedupeHash with a different hash H2 must
// return nil, false.
func TestDedupeHash_DifferentHash(t *testing.T) {
	t.Parallel()

	ctx := testCtx()
	const (
		H1 = "hash-1"
		H2 = "hash-2"
		A  = "asset-1"
	)
	src := entity.Source{ID: "S1", SHA256: H1, OwnerID: testUser}
	doc := entity.Document{ID: "D", SourceID: "S1", AssetID: A, OwnerID: testUser}

	deduper := NewDeduper(newFakeSourceRepo(src), newFakeDocumentRepo(doc))

	got, found := deduper.DedupeHash(ctx, H2, A)
	if found {
		t.Fatalf("found = true, want false")
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

// TestDedupeHash_NoLinkedDocument seeds a source S (SHA256=H) but no document
// linking it to asset A (only a document for a different asset). DedupeHash
// must return nil, false.
func TestDedupeHash_NoLinkedDocument(t *testing.T) {
	t.Parallel()

	ctx := testCtx()
	const (
		H = "hash-1"
		A = "asset-1"
	)
	src := entity.Source{ID: "S", SHA256: H, OwnerID: testUser}
	// Document exists but links S to a DIFFERENT asset, not A.
	doc := entity.Document{ID: "D", SourceID: "S", AssetID: "asset-2", OwnerID: testUser}

	deduper := NewDeduper(newFakeSourceRepo(src), newFakeDocumentRepo(doc))

	got, found := deduper.DedupeHash(ctx, H, A)
	if found {
		t.Fatalf("found = true, want false")
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

// TestDedupeHash_EmptyArgs verifies DedupeHash short-circuits on empty
// sourceHash or assetID without touching the repositories.
func TestDedupeHash_EmptyArgs(t *testing.T) {
	t.Parallel()

	ctx := testCtx()
	const H = "hash-1"
	srcRepo := newFakeSourceRepo()
	docRepo := newFakeDocumentRepo()
	deduper := NewDeduper(srcRepo, docRepo)

	if got, found := deduper.DedupeHash(ctx, "", H); found || got != nil {
		t.Errorf("empty sourceHash: got (%v, %v), want (nil, false)", got, found)
	}
	if got, found := deduper.DedupeHash(ctx, H, ""); found || got != nil {
		t.Errorf("empty assetID: got (%v, %v), want (nil, false)", got, found)
	}
	// Empty args short-circuit before any repository access.
	if n := srcRepo.listCount(); n != 0 {
		t.Errorf("source List count = %d, want 0 (no repo access on empty args)", n)
	}
	if n := docRepo.listCount(); n != 0 {
		t.Errorf("document List count = %d, want 0 (no repo access on empty args)", n)
	}
}

// TestDedupeHash_CrossTenantIsolation seeds a source + document owned by a
// different user. DedupeHash run under the test user must not see them
// (owner-scoped) and returns nil, false.
func TestDedupeHash_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	ctx := testCtx()
	const (
		H = "hash-1"
		A = "asset-1"
	)
	other := "other-user"
	src := entity.Source{ID: "S", SHA256: H, OwnerID: other}
	doc := entity.Document{ID: "D", SourceID: "S", AssetID: A, OwnerID: other}
	deduper := NewDeduper(newFakeSourceRepo(src), newFakeDocumentRepo(doc))

	if got, found := deduper.DedupeHash(ctx, H, A); found || got != nil {
		t.Errorf("cross-tenant: got (%v, %v), want (nil, false)", got, found)
	}
}
