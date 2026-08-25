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

	"procrastinator-backend/commons-data"
	"procrastinator-backend/procrastinator-core/identity"
)

// defaultRaw is the canonical LLM extraction payload used as the default
// extractor result and in the invoice test cases.
const defaultRaw = `{"classification":"invoice","brand":"LG","model":"WM-2000","serial_number":"SN-123","purchase_date":"2024-01-12","price":"39999.99","currency":"INR","metadata":{"invoice_number":"INV-1"}}`

// Test sentinels shared by multiple cases.
var (
	errUnsupportedType = errors.New("unsupported file type")
	errDocInsert       = errors.New("document insert failed")
)

// Compile-time interface guards.
var (
	_ data.Extractor          = (*fakeExtractor)(nil)
	_ data.FileStorage        = (*fakeStorage)(nil)
	_ data.SourceRepository   = (*fakeSourceRepo)(nil)
	_ data.AssetRepository    = (*fakeAssetRepo)(nil)
	_ data.DocumentRepository = (*fakeDocumentRepo)(nil)
	_ data.RepoFactory        = (*fakeFactory)(nil)
)

type extractorCall struct {
	contentType string
	data        []byte
}

// fakeExtractor is a configurable in-memory implementation of data.Extractor
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

// fakeStorage is a configurable in-memory implementation of data.FileStorage.
type fakeStorage struct {
	mu     sync.Mutex
	stored [][]byte
	putErr error
	nextID int
}

func (s *fakeStorage) Put(_ context.Context, b []byte) (data.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putErr != nil {
		return data.Source{}, s.putErr
	}
	payload := make([]byte, len(b))
	copy(payload, b)
	s.stored = append(s.stored, payload)
	s.nextID++
	return data.Source{
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

// fakeSourceRepo is an in-memory implementation of data.SourceRepository.
type fakeSourceRepo struct {
	mu      sync.Mutex
	sources map[string]data.Source
	nextID  int
}

func newFakeSourceRepo() *fakeSourceRepo {
	return &fakeSourceRepo{sources: make(map[string]data.Source)}
}

func (r *fakeSourceRepo) Create(ctx context.Context, s data.Source) (data.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		r.nextID++
		s.ID = "src-" + strconv.Itoa(r.nextID)
	}
	if t, err := data.TenantFrom(ctx); err == nil {
		s.TenantID = t
	}
	r.sources[s.ID] = s
	return s, nil
}

func (r *fakeSourceRepo) GetByID(_ context.Context, id string) (data.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	if !ok {
		return data.Source{}, data.ErrNotFound
	}
	return s, nil
}

// count returns the number of stored sources.
func (r *fakeSourceRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sources)
}

// all returns every stored source in deterministic order by ID.
func (r *fakeSourceRepo) all() []data.Source {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]data.Source, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// fakeAssetRepo is an in-memory implementation of data.AssetRepository.
type fakeAssetRepo struct {
	mu        sync.Mutex
	assets    map[string]data.Asset
	nextID    int
	createErr error
	saved     map[string]data.Asset
}

func newFakeAssetRepo() *fakeAssetRepo {
	return &fakeAssetRepo{assets: make(map[string]data.Asset)}
}

func (r *fakeAssetRepo) insertLocked(ctx context.Context, a data.Asset) data.Asset {
	if a.ID == "" {
		r.nextID++
		a.ID = "asset-" + strconv.Itoa(r.nextID)
	}
	if t, err := data.TenantFrom(ctx); err == nil {
		a.TenantID = t
	}
	r.assets[a.ID] = a
	return a
}

func (r *fakeAssetRepo) Create(ctx context.Context, a data.Asset) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return data.Asset{}, r.createErr
	}
	return r.insertLocked(ctx, a), nil
}

func (r *fakeAssetRepo) CreateOnConflictSerial(ctx context.Context, a data.Asset) (data.Asset, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return data.Asset{}, false, r.createErr
	}
	if a.NormSerial != nil {
		for _, existing := range r.assets {
			if existing.NormSerial != nil && *existing.NormSerial == *a.NormSerial {
				return existing, true, nil
			}
		}
	}
	return r.insertLocked(ctx, a), false, nil
}

func (r *fakeAssetRepo) GetByID(_ context.Context, id string) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.assets[id]
	if !ok {
		return data.Asset{}, data.ErrNotFound
	}
	return a, nil
}

func (r *fakeAssetRepo) List(_ context.Context) ([]data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]data.Asset, 0, len(r.assets))
	for _, a := range r.assets {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeAssetRepo) FindBySerial(_ context.Context, normSerial string) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []data.Asset
	for _, a := range r.assets {
		if a.NormSerial != nil && *a.NormSerial == normSerial {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return data.Asset{}, data.ErrNotFound
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out[0], nil
}

func (r *fakeAssetRepo) FindByBrandModel(_ context.Context, normBrand, normModel string) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []data.Asset
	for _, a := range r.assets {
		if a.NormBrand != nil && a.NormModel != nil &&
			*a.NormBrand == normBrand && *a.NormModel == normModel {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return data.Asset{}, data.ErrNotFound
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out[0], nil
}

func (r *fakeAssetRepo) UpdateFields(_ context.Context, id string, f data.UpdateFields) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.assets[id]
	if !ok {
		return data.Asset{}, data.ErrNotFound
	}
	if f.Brand != nil {
		a.Brand = f.Brand
	}
	if f.Model != nil {
		a.Model = f.Model
	}
	if f.SerialNumber != nil {
		a.SerialNumber = f.SerialNumber
	}
	if f.PurchaseDate != nil {
		a.PurchaseDate = f.PurchaseDate
	}
	if f.WarrantyEnd != nil {
		a.WarrantyEnd = f.WarrantyEnd
	}
	if f.Price != nil {
		a.Price = f.Price
	}
	if f.Currency != nil {
		a.Currency = f.Currency
	}
	if f.DocType != nil {
		a.DocType = *f.DocType
	}
	if f.Metadata != nil {
		a.Metadata = f.Metadata
	}
	a.UpdatedAt = time.Now()
	r.assets[id] = a
	return a, nil
}

// count returns the number of stored assets.
func (r *fakeAssetRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.assets)
}

// all returns every stored asset in deterministic order by ID.
func (r *fakeAssetRepo) all() []data.Asset {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]data.Asset, 0, len(r.assets))
	for _, a := range r.assets {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// get returns the asset with the given ID.
func (r *fakeAssetRepo) get(id string) (data.Asset, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.assets[id]
	return a, ok
}

// snapshot deep-copies the asset map for transaction rollback support.
func (r *fakeAssetRepo) snapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := make(map[string]data.Asset, len(r.assets))
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
	r.assets = make(map[string]data.Asset, len(r.saved))
	for id, a := range r.saved {
		c := a
		c.Metadata = copyMap(a.Metadata)
		r.assets[id] = c
	}
	r.saved = nil
}

// fakeDocumentRepo is an in-memory implementation of data.DocumentRepository.
type fakeDocumentRepo struct {
	mu        sync.Mutex
	documents map[string]data.Document
	nextID    int
	createErr error
	saved     map[string]data.Document
}

func newFakeDocumentRepo() *fakeDocumentRepo {
	return &fakeDocumentRepo{documents: make(map[string]data.Document)}
}

func (r *fakeDocumentRepo) Create(ctx context.Context, d data.Document) (data.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return data.Document{}, r.createErr
	}
	if d.ID == "" {
		r.nextID++
		d.ID = "doc-" + strconv.Itoa(r.nextID)
	}
	if t, err := data.TenantFrom(ctx); err == nil {
		d.TenantID = t
	}
	d.ExtractedFields = copyMap(d.ExtractedFields)
	r.documents[d.ID] = d
	return d, nil
}

func (r *fakeDocumentRepo) GetByID(_ context.Context, id string) (data.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.documents[id]
	if !ok {
		return data.Document{}, data.ErrNotFound
	}
	return d, nil
}

func (r *fakeDocumentRepo) ListByAsset(_ context.Context, assetID string) ([]data.DocumentWithSource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]data.DocumentWithSource, 0)
	for _, d := range r.documents {
		if d.AssetID == assetID {
			out = append(out, data.DocumentWithSource{Document: d})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// count returns the number of stored documents.
func (r *fakeDocumentRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.documents)
}

// all returns every stored document in deterministic order by ID.
func (r *fakeDocumentRepo) all() []data.Document {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]data.Document, 0, len(r.documents))
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
	saved := make(map[string]data.Document, len(r.documents))
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
	r.documents = make(map[string]data.Document, len(r.saved))
	for id, d := range r.saved {
		c := d
		c.ExtractedFields = copyMap(d.ExtractedFields)
		r.documents[id] = c
	}
	r.saved = nil
}

// fakeFactory is an in-memory implementation of data.RepoFactory.
// InTransaction snapshots the asset and document repos (sources are
// created before the transaction and are not part of the snapshot).
type fakeFactory struct {
	assetRepo    *fakeAssetRepo
	sourceRepo   *fakeSourceRepo
	documentRepo *fakeDocumentRepo
}

func newFakeFactory(assetRepo *fakeAssetRepo, sourceRepo *fakeSourceRepo, documentRepo *fakeDocumentRepo) *fakeFactory {
	return &fakeFactory{assetRepo: assetRepo, sourceRepo: sourceRepo, documentRepo: documentRepo}
}

func (f *fakeFactory) AssetRepo() data.AssetRepository {
	return f.assetRepo
}

func (f *fakeFactory) SourceRepo() data.SourceRepository {
	return f.sourceRepo
}

func (f *fakeFactory) DocumentRepo() data.DocumentRepository {
	return f.documentRepo
}

func (f *fakeFactory) InTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	f.assetRepo.snapshot()
	f.documentRepo.snapshot()
	ctx = data.WithRepoFactory(ctx, f)
	if err := fn(ctx); err != nil {
		f.assetRepo.rollback()
		f.documentRepo.rollback()
		return err
	}
	return nil
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

// harness wires the fakes together with the Service under test.
type harness struct {
	svc       *Service
	factory   *fakeFactory
	extractor *fakeExtractor
	storage   *fakeStorage
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
	seed         []data.Asset
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

	factory := newFakeFactory(assetRepo, sourceRepo, documentRepo)

	payload := []byte("fake pdf payload")
	if tc.dataLen > 0 {
		payload = bytes.Repeat([]byte("x"), tc.dataLen)
	}
	maxBytes := tc.maxBytes
	if maxBytes == 0 {
		maxBytes = 100
	}

	return &harness{
		svc:       New(factory, extractor, storage, maxBytes),
		factory:   factory,
		extractor: extractor,
		storage:   storage,
		payload:   payload,
	}
}

func TestProcess(t *testing.T) {
	t.Parallel()

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
				docs := h.factory.documentRepo.all()
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
				if assets[0].TenantID != "test-tenant" {
					t.Fatalf("asset TenantID = %q, want \"test-tenant\"", assets[0].TenantID)
				}
				if sources[0].TenantID != "test-tenant" {
					t.Fatalf("source TenantID = %q, want \"test-tenant\"", sources[0].TenantID)
				}
				if docs[0].TenantID != "test-tenant" {
					t.Fatalf("document TenantID = %q, want \"test-tenant\"", docs[0].TenantID)
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
				docs := h.factory.documentRepo.all()
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
				`{"classification":"invoice","brand":"LG","model":"WM-2000","serial_number":"SN-123","price":"100.00","currency":"INR"}`,
				`{"classification":"warranty","serial_number":"SN-123","warranty_end":"2026-01-01","price":"200.00"}`,
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
				docs := h.factory.documentRepo.all()
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
			ctx := data.WithTenant(context.Background(), "test-tenant")
			for i := range raws {
				_, err := h.svc.Process(ctx, "invoice.pdf", h.payload, "application/pdf")
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
			if got := h.factory.documentRepo.count(); got != tc.wantDocs {
				t.Fatalf("document count = %d, want %d", got, tc.wantDocs)
			}
			if tc.verify != nil {
				tc.verify(t, h)
			}
		})
	}
}
