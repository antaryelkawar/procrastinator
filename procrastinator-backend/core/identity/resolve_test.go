package identity

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
	"procrastinator-backend/commons/tenant"
)

// Compile-time check: fakeAssetRepo satisfies the generic asset repository.
var _ repo.Repository[entity.Asset] = (*fakeAssetRepo)(nil)

// testTenant is the tenant ID used by all tests.
const testTenant = "test-tenant"

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// testCtx returns a context carrying the test tenant.
func testCtx() context.Context {
	return tenant.WithTenant(context.Background(), testTenant)
}

// fakeAssetRepo is a configurable in-memory implementation of
// repo.Repository[entity.Asset] with mutex protection and call recording.
type fakeAssetRepo struct {
	mu     sync.Mutex
	assets map[string]entity.Asset
	nextID int
	calls  []string
	// createErr, when non-nil, is returned by Create.
	createErr error
	// conflictReturn, when non-nil, is returned by Create instead of
	// inserting the asset (simulates a DB-level conflict winner).
	conflictReturn *entity.Asset
}

// newFakeAssetRepo returns a fakeAssetRepo seeded with the given assets.
// Seeded assets are forced to carry the test tenant so they survive the
// tenant filter applied by List.
func newFakeAssetRepo(assets ...entity.Asset) *fakeAssetRepo {
	r := &fakeAssetRepo{
		assets: make(map[string]entity.Asset, len(assets)),
	}
	for _, a := range assets {
		a.TenantID = testTenant
		r.assets[a.ID] = a
	}
	return r
}

// tenantFromOpts extracts the tenant ID from options ("" if absent).
func tenantFromOpts(opts []repo.Option) string {
	return repo.ApplyOptions(opts...).TenantID
}

func (r *fakeAssetRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "Get:"+id)
	a, ok := r.assets[id]
	if !ok {
		return entity.Asset{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" && a.TenantID != tid {
		return entity.Asset{}, repo.ErrNotFound
	}
	return a, nil
}

func (r *fakeAssetRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	r.calls = append(r.calls, "List")

	out := make([]entity.Asset, 0, len(r.assets))
	for _, a := range r.assets {
		if o.TenantID != "" && a.TenantID != o.TenantID {
			continue
		}
		match := true
		for _, f := range o.Filters {
			if !r.matchesFilter(a, f) {
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

// matchesFilter reports whether asset a satisfies filter f (supports "="
// on the norm_* string columns).
func (r *fakeAssetRepo) matchesFilter(a entity.Asset, f repo.Filter) bool {
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

func (r *fakeAssetRepo) Create(ctx context.Context, a entity.Asset, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return entity.Asset{}, r.createErr
	}
	if r.conflictReturn != nil {
		r.calls = append(r.calls, "Create:conflict")
		c := *r.conflictReturn
		return c, nil
	}
	if a.ID == "" {
		r.nextID++
		a.ID = "id-" + strconv.Itoa(r.nextID)
	}
	if tid := tenantFromOpts(opts); tid != "" {
		a.TenantID = tid
	}
	r.assets[a.ID] = a
	r.calls = append(r.calls, "Create:"+a.ID)
	return a, nil
}

func (r *fakeAssetRepo) Update(ctx context.Context, a entity.Asset, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "Update:"+a.ID)
	if _, ok := r.assets[a.ID]; !ok {
		return entity.Asset{}, repo.ErrNotFound
	}
	if tid := tenantFromOpts(opts); tid != "" {
		a.TenantID = tid
	}
	a.UpdatedAt = time.Now()
	r.assets[a.ID] = a
	return a, nil
}

func (r *fakeAssetRepo) Delete(ctx context.Context, id string, opts ...repo.Option) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "Delete:"+id)
	if _, ok := r.assets[id]; !ok {
		return repo.ErrNotFound
	}
	delete(r.assets, id)
	return nil
}

// callsWith returns the recorded calls matching a prefix.
func (r *fakeAssetRepo) callsWith(prefix string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.calls {
		if len(c) >= len(prefix) && c[:len(prefix)] == prefix {
			out = append(out, c)
		}
	}
	return out
}

func TestResolve_SerialMatchWinsOverBrandModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		extraction  entity.Extraction
		repoAssets  []entity.Asset
		wantID      string
		wantCreated bool
		wantCreate  bool
	}{
		{
			name: "serial match found",
			extraction: entity.Extraction{
				SerialNumber: ptr("SN-123"),
				Brand:        ptr("LG"),
				Model:        ptr("W1234"),
			},
			repoAssets: []entity.Asset{
				{
					ID:         "A",
					NormSerial: ptr("SN-123"),
					Brand:      ptr("Samsung"),
					NormBrand:  ptr("samsung"),
				},
				{
					ID:        "B",
					NormBrand: ptr("lg"),
					NormModel: ptr("w1234"),
					Brand:     ptr("LG"),
					Model:     ptr("W1234"),
				},
			},
			wantID:      "A",
			wantCreated: false,
			wantCreate:  false,
		},
		{
			name: "serial match wins, no brand+model conflict",
			extraction: entity.Extraction{
				SerialNumber: ptr("SN-456"),
			},
			repoAssets: []entity.Asset{
				{ID: "C", NormSerial: ptr("SN-456")},
			},
			wantID:      "C",
			wantCreated: false,
			wantCreate:  false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeAssetRepo(tc.repoAssets...)
			got, created, err := Resolve(testCtx(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if got.ID != tc.wantID {
				t.Errorf("resolved asset ID = %q, want %q", got.ID, tc.wantID)
			}
			if created != tc.wantCreated {
				t.Errorf("created = %v, want %v", created, tc.wantCreated)
			}
			if tc.wantCreate && len(repo.callsWith("Create")) == 0 {
				t.Error("expected a Create call, got none")
			}
			if !tc.wantCreate && len(repo.callsWith("Create")) != 0 {
				t.Errorf("expected no Create call, got %v", repo.callsWith("Create"))
			}
		})
	}
}

func TestResolve_BrandModelMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		extraction  entity.Extraction
		repoAssets  []entity.Asset
		wantID      string
		wantCreated bool
		wantCreate  bool
	}{
		{
			name: "both brand and model present, match found",
			extraction: entity.Extraction{
				Brand: ptr("samsung"),
				Model: ptr("WW90"),
			},
			repoAssets: []entity.Asset{
				{ID: "D", NormBrand: ptr("samsung"), NormModel: ptr("ww90"), Brand: ptr("samsung"), Model: ptr("WW90")},
			},
			wantID:      "D",
			wantCreated: false,
			wantCreate:  false,
		},
		{
			name: "brand alone, no match",
			extraction: entity.Extraction{
				Brand: ptr("LG"),
			},
			repoAssets:  []entity.Asset{},
			wantID:      "", // new asset gets a generated ID
			wantCreated: true,
			wantCreate:  true,
		},
		{
			name: "model alone, no match",
			extraction: entity.Extraction{
				Model: ptr("W1234"),
			},
			repoAssets:  []entity.Asset{},
			wantID:      "", // new asset gets a generated ID
			wantCreated: true,
			wantCreate:  true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeAssetRepo(tc.repoAssets...)
			got, created, err := Resolve(testCtx(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if tc.wantID != "" && got.ID != tc.wantID {
				t.Errorf("resolved asset ID = %q, want %q", got.ID, tc.wantID)
			}
			if created != tc.wantCreated {
				t.Errorf("created = %v, want %v", created, tc.wantCreated)
			}
			if tc.wantCreate && len(repo.callsWith("Create")) == 0 {
				t.Error("expected a Create call, got none")
			}
			if !tc.wantCreate && len(repo.callsWith("Create")) != 0 {
				t.Errorf("expected no Create call, got %v", repo.callsWith("Create"))
			}
			// When a brand+model match is found, the repo should have been
			// listed (List) to find it.
			if tc.name == "both brand and model present, match found" {
				if len(repo.callsWith("List")) == 0 {
					t.Error("expected List to be called, got none")
				}
			}
		})
	}
}

func TestResolve_NoIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		extraction entity.Extraction
	}{
		{
			name: "no serial no brand no model",
			extraction: entity.Extraction{
				Price:    ptr("999.99"),
				Currency: ptr("EUR"),
			},
		},
		{
			name: "empty strings all identity fields",
			extraction: entity.Extraction{
				SerialNumber: ptr(""),
				Brand:        ptr(""),
				Model:        ptr(""),
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeAssetRepo()
			_, _, err := Resolve(testCtx(), repo, tc.extraction)
			if !errors.Is(err, ErrNoIdentity) {
				t.Errorf("Resolve error = %v, want ErrNoIdentity", err)
			}
			if len(repo.calls) != 0 {
				t.Errorf("expected no repo calls, got %v", repo.calls)
			}
		})
	}
}

func TestResolve_CreateNew(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		extraction  entity.Extraction
		wantCreated bool
	}{
		{
			name: "serial present, no existing",
			extraction: entity.Extraction{
				SerialNumber: ptr("NEW-SN"),
			},
			wantCreated: true,
		},
		{
			name: "brand+model only, no existing",
			extraction: entity.Extraction{
				Brand: ptr("LG"),
				Model: ptr("W1234"),
			},
			wantCreated: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeAssetRepo()
			got, created, err := Resolve(testCtx(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if got.ID == "" {
				t.Error("expected a non-empty ID on the created asset")
			}
			if created != tc.wantCreated {
				t.Errorf("created = %v, want %v", created, tc.wantCreated)
			}
			if len(repo.callsWith("Create")) == 0 {
				t.Error("expected a Create call, got none")
			}
		})
	}
}

func TestResolve_MergeSemantics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		existing   entity.Asset
		extraction entity.Extraction
	}{
		{
			name: "non-nil extraction fields overwrite",
			existing: entity.Asset{
				ID:           "M1",
				Brand:        ptr("Samsung"),
				Model:        ptr("Old"),
				SerialNumber: ptr("M1-SN"),
				NormSerial:   ptr("M1-SN"),
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("M1-SN"),
				Model:        ptr("New"),
				Price:        ptr("999.99"),
			},
		},
		{
			name: "nil extraction fields never erase",
			existing: entity.Asset{
				ID:           "M2",
				Price:        ptr("500.00"),
				Brand:        ptr("LG"),
				SerialNumber: ptr("M2-SN"),
				NormSerial:   ptr("M2-SN"),
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("M2-SN"),
				WarrantyEnd:  ptr(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
		},
		{
			name: "price retained when extraction has nil price",
			existing: entity.Asset{
				ID:           "M3",
				Price:        ptr("399.99"),
				SerialNumber: ptr("M3-SN"),
				NormSerial:   ptr("M3-SN"),
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("M3-SN"),
				Model:        ptr("X"),
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeAssetRepo(tc.existing)
			_, _, err := Resolve(testCtx(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}

			// Verify merge semantics via the resulting asset state:
			// non-nil extraction fields are applied, nil fields are preserved.
			stored, err := repo.Get(testCtx(), tc.existing.ID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}

			if tc.extraction.Model != nil {
				if stored.Model == nil || *stored.Model != *tc.extraction.Model {
					t.Errorf("Model = %v, want %q", stored.Model, *tc.extraction.Model)
				}
			} else {
				if stored.Model != tc.existing.Model {
					t.Errorf("Model = %v, want original %v (untouched)", stored.Model, tc.existing.Model)
				}
			}

			if tc.extraction.Price != nil {
				if stored.Price == nil || *stored.Price != *tc.extraction.Price {
					t.Errorf("Price = %v, want %q", stored.Price, *tc.extraction.Price)
				}
			} else {
				if stored.Price != tc.existing.Price {
					t.Errorf("Price = %v, want original %v (untouched)", stored.Price, tc.existing.Price)
				}
			}

			if tc.extraction.Brand != nil {
				if stored.Brand == nil || *stored.Brand != *tc.extraction.Brand {
					t.Errorf("Brand = %v, want %q", stored.Brand, *tc.extraction.Brand)
				}
			} else {
				if stored.Brand != tc.existing.Brand {
					t.Errorf("Brand = %v, want original %v (untouched)", stored.Brand, tc.existing.Brand)
				}
			}

			if tc.extraction.WarrantyEnd != nil {
				if stored.WarrantyEnd == nil || !stored.WarrantyEnd.Equal(*tc.extraction.WarrantyEnd) {
					t.Errorf("WarrantyEnd = %v, want %v", stored.WarrantyEnd, tc.extraction.WarrantyEnd)
				}
			} else {
				if stored.WarrantyEnd != tc.existing.WarrantyEnd {
					t.Errorf("WarrantyEnd = %v, want original %v (untouched)", stored.WarrantyEnd, tc.existing.WarrantyEnd)
				}
			}
		})
	}
}

func TestResolve_MetadataMerge(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		existing   entity.Asset
		extraction entity.Extraction
		wantMeta   map[string]any
	}{
		{
			name: "new metadata keys added",
			existing: entity.Asset{
				ID:         "MD1",
				NormSerial: ptr("MD1-SN"),
				Metadata:   map[string]any{"amc_card_number": "C1"},
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("MD1-SN"),
				Metadata:     map[string]any{"icr_number": "I9"},
			},
			wantMeta: map[string]any{
				"amc_card_number": "C1",
				"icr_number":      "I9",
			},
		},
		{
			name: "existing keys overwritten if non-nil",
			existing: entity.Asset{
				ID:         "MD2",
				NormSerial: ptr("MD2-SN"),
				Metadata:   map[string]any{"name": "old"},
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("MD2-SN"),
				Metadata:     map[string]any{"name": "new"},
			},
			wantMeta: map[string]any{
				"name": "new",
			},
		},
		{
			name: "extraction metadata nil",
			existing: entity.Asset{
				ID:         "MD3",
				NormSerial: ptr("MD3-SN"),
				Metadata:   map[string]any{"a": "b"},
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("MD3-SN"),
				Metadata:     nil,
			},
			wantMeta: map[string]any{"a": "b"}, // unchanged
		},
		{
			name: "both nil",
			existing: entity.Asset{
				ID:         "MD4",
				NormSerial: ptr("MD4-SN"),
				Metadata:   nil,
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("MD4-SN"),
				Metadata:     nil,
			},
			wantMeta: nil, // stays nil
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeAssetRepo(tc.existing)
			_, _, err := Resolve(testCtx(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			stored, err := repo.Get(testCtx(), tc.existing.ID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if tc.wantMeta == nil {
				if stored.Metadata != nil {
					t.Errorf("Metadata = %v, want nil", stored.Metadata)
				}
				return
			}
			if stored.Metadata == nil {
				t.Fatalf("Metadata = nil, want %v", tc.wantMeta)
			}
			for k, want := range tc.wantMeta {
				got, ok := stored.Metadata[k]
				if !ok {
					t.Errorf("Metadata missing key %q, want %v", k, want)
					continue
				}
				if got != want {
					t.Errorf("Metadata[%q] = %v, want %v", k, got, want)
				}
			}
			if len(stored.Metadata) != len(tc.wantMeta) {
				t.Errorf("Metadata has %d keys, want %d: %v", len(stored.Metadata), len(tc.wantMeta), stored.Metadata)
			}
		})
	}
}

func TestResolve_DocTypeSet(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		existing   entity.Asset
		extraction entity.Extraction
		wantDoc    string
	}{
		{
			name: "doc_type updated on merge",
			existing: entity.Asset{
				ID:         "DT1",
				NormSerial: ptr("DT1-SN"),
				DocType:    "invoice",
			},
			extraction: entity.Extraction{
				SerialNumber:   ptr("DT1-SN"),
				Classification: "amc",
			},
			wantDoc: "amc",
		},
		{
			name:     "doc_type set on create",
			existing: entity.Asset{},
			extraction: entity.Extraction{
				SerialNumber:   ptr("DT2-SN"),
				Classification: "warranty",
			},
			wantDoc: "warranty",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var repo *fakeAssetRepo
			var wantID string
			if tc.existing.ID != "" {
				repo = newFakeAssetRepo(tc.existing)
				wantID = tc.existing.ID
			} else {
				repo = newFakeAssetRepo()
			}

			got, _, err := Resolve(testCtx(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if wantID != "" && got.ID != wantID {
				t.Errorf("resolved asset ID = %q, want %q", got.ID, wantID)
			}
			if got.DocType != tc.wantDoc {
				t.Errorf("DocType = %q, want %q", got.DocType, tc.wantDoc)
			}
		})
	}
}
