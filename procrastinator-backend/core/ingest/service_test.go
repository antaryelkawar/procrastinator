package ingest

import (
	"bytes"
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

// testUser is the user ID used by all ingest tests.
const testUser = "test-user"

// defaultRaw is the canonical LLM extraction payload used as the default
// extractor result and in the invoice test cases.
const defaultRaw = `{"classification":"invoice","brand":"LG","model":"WM-2000","serial_number":"SN-123","purchase_date":"2024-01-12","price":"39999.99","currency":"INR","confidence":0.95,"metadata":{"invoice_number":"INV-1"}}`

// Test sentinels shared by multiple cases.
var (
	errUnsupportedType = errors.New("unsupported file type")
	errDocInsert       = errors.New("document insert failed")
)

// Compile-time interface guards.
var (
	_ repo.Extractor                   = (*fakeExtractor)(nil)
	_ repo.FileStorage                 = (*fakeStorage)(nil)
	_ repo.Repository[entity.Asset]    = (*fakeAssetRepo)(nil)
	_ repo.Repository[entity.Source]   = (*fakeSourceRepo)(nil)
	_ repo.Repository[entity.Document] = (*fakeDocumentRepo)(nil)
)

type extractorCall struct {
	contentType string
	data        []byte
}

// fakeExtractor is a configurable in-memory implementation of repo.Extractor
// with call recording.
type fakeExtractor struct {
	mu sync.Mutex

	calls  []extractorCall
	queue  []string
	result string
	err    error
}

func (e *fakeExtractor) Extract(_ context.Context, contentType string, data []byte) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	payload := make([]byte, len(data))
	copy(payload, data)
	e.calls = append(e.calls, extractorCall{contentType: contentType, data: payload})
	if e.err != nil {
		return "", e.err
	}
	if len(e.queue) > 0 {
		out := e.queue[0]
		e.queue = e.queue[1:]
		return out, nil
	}
	return e.result, nil
}

// callCount returns the number of Extract calls.
func (e *fakeExtractor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

// lastCall returns the contentType and data of the most recent Extract call.
func (e *fakeExtractor) lastCall() (string, []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.calls) == 0 {
		return "", nil
	}
	last := e.calls[len(e.calls)-1]
	return last.contentType, last.data
}

// fakeStorage is a configurable in-memory implementation of repo.FileStorage.
type fakeStorage struct {
	mu     sync.Mutex
	stored [][]byte
	putErr error
	nextID int
}

func (s *fakeStorage) Put(_ context.Context, b []byte) (entity.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putErr != nil {
		return entity.Source{}, s.putErr
	}
	payload := make([]byte, len(b))
	copy(payload, b)
	s.stored = append(s.stored, payload)
	s.nextID++
	return entity.Source{
		ID:          "src-" + strconv.Itoa(s.nextID),
		Filename:    "upload.bin",
		ContentType: "application/pdf",
		Size:        int64(len(b)),
		UploadedAt:  time.Now(),
	}, nil
}

// storedCount returns the number of successful Put calls.
func (s *fakeStorage) storedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.stored)
}

// ownerFromOpts extracts the owner ID from options ("" if absent).
func ownerFromOpts(opts []repo.Option) string {
	return repo.ApplyOptions(opts...).OwnerID
}

// fakeAssetRepo is an in-memory implementation of repo.Repository[entity.Asset].
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

// matchAssetFilter reports whether asset a satisfies a filter. It supports
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
	a.UpdatedAt = time.Now()
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

// get returns the asset with the given ID.
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

// fakeSourceRepo is an in-memory implementation of repo.Repository[entity.Source].
type fakeSourceRepo struct {
	mu      sync.Mutex
	sources map[string]entity.Source
	nextID  int
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
	if o.Limit > 0 && len(out) > o.Limit {
		out = out[:o.Limit]
	}
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

// all returns every stored source in deterministic order by ID.
func (r *fakeSourceRepo) all() []entity.Source {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Source, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

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

// fakeFactory wraps a concrete *repo.Factory built from the fake repositories.
// InTx snapshots the asset and document repos (sources are created before the
// transaction and are not part of the snapshot); on error it rolls both back.
type fakeFactory struct {
	factory    *repo.Factory
	assetRepo  *fakeAssetRepo
	sourceRepo *fakeSourceRepo
	docRepo    *fakeDocumentRepo
}

func newFakeFactory(assetRepo *fakeAssetRepo, sourceRepo *fakeSourceRepo, docRepo *fakeDocumentRepo) *fakeFactory {
	f := &repo.Factory{
		Assets:    assetRepo,
		Sources:   sourceRepo,
		Documents: docRepo,
		InTx: func(ctx context.Context, fn func(ctx context.Context, repos *repo.Repos) error) error {
			assetRepo.snapshot()
			docRepo.snapshot()
			repos := &repo.Repos{
				Assets:    assetRepo,
				Sources:   sourceRepo,
				Documents: docRepo,
			}
			if err := fn(ctx, repos); err != nil {
				assetRepo.rollback()
				docRepo.rollback()
				return err
			}
			return nil
		},
	}
	return &fakeFactory{factory: f, assetRepo: assetRepo, sourceRepo: sourceRepo, docRepo: docRepo}
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

// fakeReviewer is a configurable in-memory implementation of repo.Reviewer
// with call recording.
type fakeReviewer struct {
	mu      sync.Mutex
	calls   []repo.HoldInput
	err     error
	created entity.IngestReview
}

func (f *fakeReviewer) Hold(_ context.Context, input repo.HoldInput) (entity.IngestReview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, input)
	if f.err != nil {
		return entity.IngestReview{}, f.err
	}
	// Simulate identity.Match: ErrNoIdentity when no usable identity.
	ext := input.Extraction
	hasIdentity := ext.SerialNumber != nil || (ext.Brand != nil && ext.Model != nil)
	if !hasIdentity {
		return entity.IngestReview{}, identity.ErrNoIdentity
	}
	out := f.created
	out.SourceID = input.SourceID
	out.DocType = input.Extraction.Classification
	out.Confidence = input.Extraction.Confidence
	out.State = entity.ReviewStatePending
	out.CandidateFields = make(map[string]any)
	out.RawExtraction = input.Extraction.RawPayload
	out.OwnerHouseholdID = input.OwnerHouseholdID
	return out, nil
}

func (f *fakeReviewer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

var _ repo.Reviewer = (*fakeReviewer)(nil)

// harness wires the fakes together with the Service under test.
type harness struct {
	svc       *Service
	factory   *fakeFactory
	extractor *fakeExtractor
	storage   *fakeStorage
	reviewer  *fakeReviewer
	payload   []byte
}

// processCase describes one table-driven TestProcess scenario.
type processCase struct {
	name         string
	maxBytes     int64 // default 100 when 0
	dataLen      int   // 0 → default payload "fake pdf payload"
	putErr       error // fakeStorage fault
	extractErr   error // fakeExtractor fault
	docCreateErr error // fakeDocumentRepo fault
	seed         []entity.Asset
	scope        *string  // ownerHouseholdID passed to Process; nil = personal
	raws         []string // one LLM raw payload per Process call; default [defaultRaw]
	wantErrs     []error  // one per Process call; nil entry = success
	wantStored   int      // successful storage.Put count
	wantSources  int
	wantAssets   int
	wantDocs     int
	verify       func(t *testing.T, h *harness) // optional extra assertions
}

func newHarness(tc processCase) *harness {
	assetRepo := newFakeAssetRepo()
	sourceRepo := newFakeSourceRepo()
	documentRepo := newFakeDocumentRepo()
	for _, a := range tc.seed {
		a.OwnerID = testUser
		assetRepo.assets[a.ID] = a
	}

	extractor := &fakeExtractor{}
	raws := tc.raws
	if len(raws) == 0 {
		raws = []string{defaultRaw}
	}
	extractor.queue = raws
	extractor.err = tc.extractErr
	extractor.result = defaultRaw

	storage := &fakeStorage{putErr: tc.putErr}
	documentRepo.createErr = tc.docCreateErr

	ff := newFakeFactory(assetRepo, sourceRepo, documentRepo)

	payload := []byte("fake pdf payload")
	if tc.dataLen > 0 {
		payload = bytes.Repeat([]byte("x"), tc.dataLen)
	}
	maxBytes := tc.maxBytes
	if maxBytes == 0 {
		maxBytes = 100
	}

	reviewer := &fakeReviewer{}
	return &harness{
		svc:       New(ff.factory, extractor, storage, maxBytes, 0.7, reviewer),
		factory:   ff,
		extractor: extractor,
		storage:   storage,
		reviewer:  reviewer,
		payload:   payload,
	}
}

func TestProcess(t *testing.T) {
	t.Parallel()

	householdScope := "hh-1"

	cases := []processCase{
		{
			name:        "accepted upload",
			wantErrs:    []error{nil},
			wantStored:  1,
			wantSources: 1,
			wantAssets:  1,
			wantDocs:    1,
			verify: func(t *testing.T, h *harness) {
				t.Helper()
				if got := h.extractor.callCount(); got != 1 {
					t.Fatalf("extractor callCount = %d, want 1", got)
				}
				ct, payload := h.extractor.lastCall()
				if ct != "application/pdf" {
					t.Fatalf("lastCall contentType = %q, want \"application/pdf\"", ct)
				}
				if !bytes.Equal(payload, h.payload) {
					t.Fatalf("lastCall payload = %q, want the upload payload %q", payload, h.payload)
				}
				assets := h.factory.assetRepo.all()
				if len(assets) != 1 {
					t.Fatalf("asset count = %d, want 1", len(assets))
				}
				sources := h.factory.sourceRepo.all()
				if len(sources) != 1 {
					t.Fatalf("source count = %d, want 1", len(sources))
				}
				docs := h.factory.docRepo.all()
				if len(docs) != 1 {
					t.Fatalf("document count = %d, want 1", len(docs))
				}
				if docs[0].AssetID != assets[0].ID {
					t.Fatalf("doc AssetID = %q, want asset ID %q", docs[0].AssetID, assets[0].ID)
				}
				if docs[0].SourceID != sources[0].ID {
					t.Fatalf("doc SourceID = %q, want source ID %q", docs[0].SourceID, sources[0].ID)
				}
				if assets[0].NormSerial == nil || *assets[0].NormSerial != "SN-123" {
					t.Fatalf("asset NormSerial = %v, want \"SN-123\"", assets[0].NormSerial)
				}
				if assets[0].OwnerID != testUser {
					t.Fatalf("asset OwnerID = %q, want %q", assets[0].OwnerID, testUser)
				}
				if sources[0].OwnerID != testUser {
					t.Fatalf("source OwnerID = %q, want %q", sources[0].OwnerID, testUser)
				}
				if docs[0].OwnerID != testUser {
					t.Fatalf("document OwnerID = %q, want %q", docs[0].OwnerID, testUser)
				}
			},
		},
		{
			name:        "household scope stamped on source, asset, document",
			scope:       &householdScope,
			wantErrs:    []error{nil},
			wantStored:  1,
			wantSources: 1,
			wantAssets:  1,
			wantDocs:    1,
			verify: func(t *testing.T, h *harness) {
				t.Helper()
				sources := h.factory.sourceRepo.all()
				if len(sources) != 1 {
					t.Fatalf("source count = %d, want 1", len(sources))
				}
				if sources[0].OwnerHouseholdID == nil || *sources[0].OwnerHouseholdID != "hh-1" {
					t.Fatalf("source OwnerHouseholdID = %v, want \"hh-1\"", sources[0].OwnerHouseholdID)
				}
				if sources[0].OwnerID != testUser {
					t.Fatalf("source OwnerID = %q, want %q", sources[0].OwnerID, testUser)
				}
				assets := h.factory.assetRepo.all()
				if len(assets) != 1 {
					t.Fatalf("asset count = %d, want 1", len(assets))
				}
				if assets[0].OwnerHouseholdID == nil || *assets[0].OwnerHouseholdID != "hh-1" {
					t.Fatalf("asset OwnerHouseholdID = %v, want \"hh-1\"", assets[0].OwnerHouseholdID)
				}
				if assets[0].OwnerID != testUser {
					t.Fatalf("asset OwnerID = %q, want %q", assets[0].OwnerID, testUser)
				}
				docs := h.factory.docRepo.all()
				if len(docs) != 1 {
					t.Fatalf("document count = %d, want 1", len(docs))
				}
				if docs[0].OwnerHouseholdID == nil || *docs[0].OwnerHouseholdID != "hh-1" {
					t.Fatalf("document OwnerHouseholdID = %v, want \"hh-1\"", docs[0].OwnerHouseholdID)
				}
				if docs[0].OwnerID != testUser {
					t.Fatalf("document OwnerID = %q, want %q", docs[0].OwnerID, testUser)
				}
			},
		},
		{
			name:        "llm failure retains source",
			extractErr:  errors.New("llm down"),
			wantErrs:    []error{ErrExtraction},
			wantStored:  1,
			wantSources: 1,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:        "no identity retains source",
			raws:        []string{`{"classification":"other"}`},
			wantErrs:    []error{identity.ErrNoIdentity},
			wantStored:  1,
			wantSources: 1,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:        "invoice document keeps raw payload",
			wantErrs:    []error{nil},
			wantStored:  1,
			wantSources: 1,
			wantAssets:  1,
			wantDocs:    1,
			verify: func(t *testing.T, h *harness) {
				t.Helper()
				docs := h.factory.docRepo.all()
				if len(docs) != 1 {
					t.Fatalf("document count = %d, want 1", len(docs))
				}
				if docs[0].RawExtraction != defaultRaw {
					t.Fatalf("RawExtraction = %q, want verbatim defaultRaw", docs[0].RawExtraction)
				}
				if docs[0].DocType != "invoice" {
					t.Fatalf("doc DocType = %q, want \"invoice\"", docs[0].DocType)
				}
				if docs[0].ExtractedFields["serial_number"] != "SN-123" {
					t.Fatalf("ExtractedFields[serial_number] = %v, want \"SN-123\"", docs[0].ExtractedFields["serial_number"])
				}
				if docs[0].ExtractedFields["brand"] != "LG" {
					t.Fatalf("ExtractedFields[brand] = %v, want \"LG\"", docs[0].ExtractedFields["brand"])
				}
				if _, ok := docs[0].ExtractedFields["metadata"]; !ok {
					t.Fatalf("ExtractedFields missing key \"metadata\"")
				}
				assets := h.factory.assetRepo.all()
				if len(assets) != 1 {
					t.Fatalf("asset count = %d, want 1", len(assets))
				}
				if assets[0].DocType != "invoice" {
					t.Fatalf("asset DocType = %q, want \"invoice\"", assets[0].DocType)
				}
			},
		},
		{
		name: "second upload matching serial links and merges",
		raws: []string{
			`{"classification":"invoice","brand":"LG","model":"WM-2000","serial_number":"SN-123","price":"100.00","currency":"INR","confidence":0.95}`,
			`{"classification":"warranty","serial_number":"SN-123","warranty_end":"2026-01-01","price":"200.00","confidence":0.95}`,
		},
			wantErrs:    []error{nil, nil},
			wantStored:  2,
			wantSources: 2,
			wantAssets:  1,
			wantDocs:    2,
			verify: func(t *testing.T, h *harness) {
				t.Helper()
				assets := h.factory.assetRepo.all()
				if len(assets) != 1 {
					t.Fatalf("asset count = %d, want 1", len(assets))
				}
				asset := assets[0]
				docs := h.factory.docRepo.all()
				if len(docs) != 2 {
					t.Fatalf("document count = %d, want 2", len(docs))
				}
				for _, d := range docs {
					if d.AssetID != asset.ID {
						t.Fatalf("doc %s AssetID = %q, want asset ID %q", d.ID, d.AssetID, asset.ID)
					}
				}
				if asset.Price == nil || *asset.Price != "200.00" {
					t.Fatalf("asset Price = %v, want \"200.00\"", asset.Price)
				}
				if asset.Brand == nil || *asset.Brand != "LG" {
					t.Fatalf("asset Brand = %v, want \"LG\" (preserved)", asset.Brand)
				}
				if asset.Model == nil || *asset.Model != "WM-2000" {
					t.Fatalf("asset Model = %v, want \"WM-2000\" (preserved)", asset.Model)
				}
				if asset.DocType != "warranty" {
					t.Fatalf("asset DocType = %q, want \"warranty\" (last write wins)", asset.DocType)
				}
				if asset.Currency == nil || *asset.Currency != "INR" {
					t.Fatalf("asset Currency = %v, want \"INR\" (preserved)", asset.Currency)
				}
			},
		},
		{
			name:        "oversize rejected before anything",
			dataLen:     101,
			wantErrs:    []error{ErrTooLarge},
			wantStored:  0,
			wantSources: 0,
			wantAssets:  0,
			wantDocs:    0,
			verify: func(t *testing.T, h *harness) {
				t.Helper()
				if got := h.extractor.callCount(); got != 0 {
					t.Fatalf("extractor callCount = %d, want 0 (size checked before LLM)", got)
				}
			},
		},
		{
			name:        "unsupported type propagated",
			putErr:      errUnsupportedType,
			wantErrs:    []error{errUnsupportedType},
			wantStored:  0,
			wantSources: 0,
			wantAssets:  0,
			wantDocs:    0,
		},
		{
			name:         "document insert failure rolls back transaction",
			docCreateErr: errDocInsert,
			wantErrs:     []error{errDocInsert},
			wantStored:   1,
			wantSources:  1,
			wantAssets:   0,
			wantDocs:     0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raws := tc.raws
			if len(raws) == 0 {
				raws = []string{defaultRaw}
			}
			h := newHarness(tc)
			ctx := user.WithUser(context.Background(), testUser)
			for i := range raws {
				_, err := h.svc.Process(ctx, "invoice.pdf", h.payload, "application/pdf", tc.scope)
				if tc.wantErrs[i] == nil {
					if err != nil {
						t.Fatalf("Process call %d returned error %v, want nil", i, err)
					}
				} else if !errors.Is(err, tc.wantErrs[i]) {
					t.Fatalf("Process call %d returned error %v, want errors.Is %v", i, err, tc.wantErrs[i])
				}
			}
			if got := h.storage.storedCount(); got != tc.wantStored {
				t.Fatalf("storedCount = %d, want %d", got, tc.wantStored)
			}
			if got := h.factory.sourceRepo.count(); got != tc.wantSources {
				t.Fatalf("source count = %d, want %d", got, tc.wantSources)
			}
			if got := h.factory.assetRepo.count(); got != tc.wantAssets {
				t.Fatalf("asset count = %d, want %d", got, tc.wantAssets)
			}
			if got := h.factory.docRepo.count(); got != tc.wantDocs {
				t.Fatalf("document count = %d, want %d", got, tc.wantDocs)
			}
			if tc.verify != nil {
				tc.verify(t, h)
			}
		})
	}
}
