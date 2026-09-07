package identity

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// Compile-time check: fakeAssetRepo satisfies the generic asset repository.
var _ repo.Repository[entity.Asset] = (*fakeAssetRepo)(nil)

// testUser is the user ID used by all tests.
const testUser = "test-user"

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// testCtx returns a context carrying the test user.
func testCtx() context.Context {
	return user.WithUser(context.Background(), testUser)
}

// fakeAssetRepo is a configurable in-memory implementation of
// repo.Repository[entity.Asset] with mutex protection and call recording.
type fakeAssetRepo struct {
	mu     sync.Mutex
	assets map[string]entity.Asset
	nextID int
	calls  []string
	// listSignatures records, per List call in order, a canonical signature
	// of the identity filter fields used (e.g. "norm_serial",
	// "norm_brand+norm_model", "norm_model+norm_name"). owner-household
	// fencing is ignored.
	listSignatures []string
	// createErr, when non-nil, is returned by Create.
	createErr error
	// conflictReturn, when non-nil, is returned by Create instead of
	// inserting the asset (simulates a DB-level conflict winner).
	conflictReturn *entity.Asset
}

// newFakeAssetRepo returns a fakeAssetRepo seeded with the given assets.
// Seeded assets are forced to carry the test user so they survive the
// user filter applied by List.
func newFakeAssetRepo(assets ...entity.Asset) *fakeAssetRepo {
	r := &fakeAssetRepo{
		assets: make(map[string]entity.Asset, len(assets)),
	}
	for _, a := range assets {
		a.OwnerID = testUser
		r.assets[a.ID] = a
	}
	return r
}

// ownerFromOpts extracts the user ID from options ("" if absent).
func ownerFromOpts(opts []repo.Option) string {
	return repo.ApplyOptions(opts...).OwnerID
}

func (r *fakeAssetRepo) Get(ctx context.Context, id string, opts ...repo.Option) (entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "Get:"+id)
	a, ok := r.assets[id]
	if !ok {
		return entity.Asset{}, repo.ErrNotFound
	}
	if tid := ownerFromOpts(opts); tid != "" && a.OwnerID != tid {
		return entity.Asset{}, repo.ErrNotFound
	}
	return a, nil
}

func (r *fakeAssetRepo) List(ctx context.Context, opts ...repo.Option) ([]entity.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := repo.ApplyOptions(opts...)
	r.calls = append(r.calls, "List")
	r.listSignatures = append(r.listSignatures, identityFilterSignature(o.Filters))

	out := make([]entity.Asset, 0, len(r.assets))
	for _, a := range r.assets {
		if o.OwnerID != "" && a.OwnerID != o.OwnerID {
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

// matchesFilter reports whether asset a satisfies filter f (supports "=" and
// "IS NULL" on owner_household_id, and "=" on the norm_* string columns).
func (r *fakeAssetRepo) matchesFilter(a entity.Asset, f repo.Filter) bool {
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
	case "norm_name":
		got = a.NormName
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
	if tid := ownerFromOpts(opts); tid != "" {
		a.OwnerID = tid
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
	if tid := ownerFromOpts(opts); tid != "" {
		a.OwnerID = tid
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

// listStages returns, in call order, the canonical identity-filter signatures
// recorded for each List call (see List).
func (r *fakeAssetRepo) listStages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.listSignatures...)
}

// identityFilterSignature builds a canonical signature of a List call's
// identity filter fields: the sorted join of the norm_* fields present in the
// filters, ignoring owner-household fencing, joined with "+". For example
// "norm_serial", "norm_brand+norm_model", or "norm_model+norm_name".
func identityFilterSignature(filters []repo.Filter) string {
	known := map[string]struct{}{
		"norm_serial": {},
		"norm_brand":  {},
		"norm_model":  {},
		"norm_name":   {},
	}
	present := make([]string, 0, len(known))
	for _, f := range filters {
		if _, ok := known[f.Field]; ok {
			present = append(present, f.Field)
		}
	}
	sort.Strings(present)
	return strings.Join(present, "+")
}

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
			got, created, err := Resolve(testCtx(), repo, tc.extraction, nil, DefaultCandidateLimit)
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
		wantErr     error
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
			repoAssets: []entity.Asset{},
			wantErr:    ErrNoIdentity, // no eligible stage without a model
		},
		{
			name: "model alone, no match",
			extraction: entity.Extraction{
				Model: ptr("W1234"),
			},
			repoAssets: []entity.Asset{},
			wantErr:    ErrNoIdentity, // no eligible stage without a serial/brand+name
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeAssetRepo(tc.repoAssets...)
			got, created, err := Resolve(testCtx(), repo, tc.extraction, nil, DefaultCandidateLimit)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Resolve error = %v, want %v", err, tc.wantErr)
				}
				if len(repo.calls) != 0 {
					t.Errorf("expected no repo calls, got %v", repo.calls)
				}
				return
			}
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
			_, _, err := Resolve(testCtx(), repo, tc.extraction, nil, DefaultCandidateLimit)
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
			got, created, err := Resolve(testCtx(), repo, tc.extraction, nil, DefaultCandidateLimit)
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
			_, _, err := Resolve(testCtx(), repo, tc.extraction, nil, DefaultCandidateLimit)
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
			_, _, err := Resolve(testCtx(), repo, tc.extraction, nil, DefaultCandidateLimit)
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

// sharedSerialAssets returns a personal asset (OwnerHouseholdID nil) and a
// household asset (OwnerHouseholdID = &h) that share the same norm_serial, so
// scope fencing is what disambiguates them.
func sharedSerialAssets(h string) (personal, household entity.Asset) {
	return entity.Asset{
			ID:         "PERS",
			NormSerial: ptr("SHARED-SN"),
			Brand:      ptr("LG"),
			NormBrand:  ptr("lg"),
		}, entity.Asset{
			ID:               "HOUS",
			NormSerial:       ptr("SHARED-SN"),
			Brand:            ptr("LG"),
			NormBrand:        ptr("lg"),
			OwnerHouseholdID: ptr(h),
		}
}

func TestResolve_HouseholdFence(t *testing.T) {
	t.Parallel()

	h := "household-1"
	personal, household := sharedSerialAssets(h)
	repo := newFakeAssetRepo(personal, household)
	ext := entity.Extraction{
		SerialNumber: ptr("SHARED-SN"),
		Price:        ptr("777.77"),
	}

	got, created, err := Resolve(testCtx(), repo, ext, ptr(h), DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if created {
		t.Errorf("created = true, want false (a household asset should have been merged)")
	}
	if got.ID != "HOUS" {
		t.Errorf("resolved asset ID = %q, want %q (must stay within the household scope)", got.ID, "HOUS")
	}
	if got.OwnerHouseholdID == nil || *got.OwnerHouseholdID != h {
		t.Errorf("resolved OwnerHouseholdID = %v, want %q", got.OwnerHouseholdID, h)
	}

	// The household asset was merged (price applied), the personal one was not.
	if stored, err := repo.Get(testCtx(), "HOUS"); err != nil {
		t.Fatalf("Get(HOUS) error: %v", err)
	} else if stored.Price == nil || *stored.Price != "777.77" {
		t.Errorf("household asset Price = %v, want %q (merged)", stored.Price, "777.77")
	}
	if stored, err := repo.Get(testCtx(), "PERS"); err != nil {
		t.Fatalf("Get(PERS) error: %v", err)
	} else {
		if stored.Price != nil {
			t.Errorf("personal asset Price = %v, want nil (must NOT be modified by a household upload)", *stored.Price)
		}
		if stored.OwnerHouseholdID != nil {
			t.Errorf("personal asset OwnerHouseholdID = %v, want nil (unchanged)", *stored.OwnerHouseholdID)
		}
	}
}

func TestResolve_PersonalFence(t *testing.T) {
	t.Parallel()

	h := "household-1"
	personal, household := sharedSerialAssets(h)
	repo := newFakeAssetRepo(personal, household)
	ext := entity.Extraction{
		SerialNumber: ptr("SHARED-SN"),
		Price:        ptr("888.88"),
	}

	got, created, err := Resolve(testCtx(), repo, ext, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if created {
		t.Errorf("created = true, want false (the personal asset should have been merged)")
	}
	if got.ID != "PERS" {
		t.Errorf("resolved asset ID = %q, want %q (must merge the personal asset)", got.ID, "PERS")
	}
	if got.OwnerHouseholdID != nil {
		t.Errorf("resolved OwnerHouseholdID = %v, want nil", *got.OwnerHouseholdID)
	}

	// The personal asset was merged, the household one was not.
	if stored, err := repo.Get(testCtx(), "PERS"); err != nil {
		t.Fatalf("Get(PERS) error: %v", err)
	} else if stored.Price == nil || *stored.Price != "888.88" {
		t.Errorf("personal asset Price = %v, want %q (merged)", stored.Price, "888.88")
	}
	if stored, err := repo.Get(testCtx(), "HOUS"); err != nil {
		t.Fatalf("Get(HOUS) error: %v", err)
	} else {
		if stored.Price != nil {
			t.Errorf("household asset Price = %v, want nil (must NOT be modified by a personal upload)", *stored.Price)
		}
		if stored.OwnerHouseholdID == nil {
			t.Error("household asset OwnerHouseholdID = nil, want household-1 (unchanged)")
		}
	}
}

func TestResolve_ConfidenceStamped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		existing   entity.Asset
		extraction entity.Extraction
		wantConf   *float64
	}{
		{
			name:     "confidence stamped on create",
			existing: entity.Asset{},
			extraction: entity.Extraction{
				SerialNumber: ptr("CONF-1"),
				Confidence:   ptr(0.95),
			},
			wantConf: ptr(0.95),
		},
		{
			name:     "nil confidence stays nil on create",
			existing: entity.Asset{},
			extraction: entity.Extraction{
				SerialNumber: ptr("CONF-2"),
			},
			wantConf: nil,
		},
		{
			name: "confidence updated on merge",
			existing: entity.Asset{
				ID:         "CONF-M1",
				NormSerial: ptr("CONF-M1"),
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("CONF-M1"),
				Confidence:   ptr(0.4),
			},
			wantConf: ptr(0.4),
		},
		{
			name: "existing confidence preserved when extraction nil",
			existing: entity.Asset{
				ID:         "CONF-M2",
				NormSerial: ptr("CONF-M2"),
				Confidence: ptr(0.8),
			},
			extraction: entity.Extraction{
				SerialNumber: ptr("CONF-M2"),
			},
			wantConf: ptr(0.8),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var repo *fakeAssetRepo
			if tc.existing.ID != "" {
				repo = newFakeAssetRepo(tc.existing)
			} else {
				repo = newFakeAssetRepo()
			}
			got, _, err := Resolve(testCtx(), repo, tc.extraction, nil, DefaultCandidateLimit)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if tc.wantConf == nil {
				if got.Confidence != nil {
					t.Errorf("Confidence = %v, want nil", *got.Confidence)
				}
				return
			}
			if got.Confidence == nil || *got.Confidence != *tc.wantConf {
				t.Errorf("Confidence = %v, want %v", got.Confidence, *tc.wantConf)
			}
		})
	}
}

func TestResolve_ScopeStampedOnCreate(t *testing.T) {
	t.Parallel()

	t.Run("household scope stamped on create", func(t *testing.T) {
		t.Parallel()
		h := "household-2"
		repo := newFakeAssetRepo()
		got, created, err := Resolve(testCtx(), repo, entity.Extraction{SerialNumber: ptr("FRESH-H")}, ptr(h), DefaultCandidateLimit)
		if err != nil {
			t.Fatalf("Resolve returned error: %v", err)
		}
		if !created {
			t.Fatal("created = false, want true")
		}
		if got.OwnerHouseholdID == nil || *got.OwnerHouseholdID != h {
			t.Errorf("created OwnerHouseholdID = %v, want %q", got.OwnerHouseholdID, h)
		}
	})

	t.Run("personal scope stamped on create", func(t *testing.T) {
		t.Parallel()
		repo := newFakeAssetRepo()
		got, created, err := Resolve(testCtx(), repo, entity.Extraction{SerialNumber: ptr("FRESH-P")}, nil, DefaultCandidateLimit)
		if err != nil {
			t.Fatalf("Resolve returned error: %v", err)
		}
		if !created {
			t.Fatal("created = false, want true")
		}
		if got.OwnerHouseholdID != nil {
			t.Errorf("created OwnerHouseholdID = %v, want nil", *got.OwnerHouseholdID)
		}
	})
}

func TestMerge_CategorySticky_UserSetSurvivesLowerConfidence(t *testing.T) {
	t.Parallel()

	existing := entity.Asset{
		ID:                 "STICKY-1",
		NormSerial:         ptr("STICKY-1"),
		AssetCategory:      ptr("electronics"),
		CategoryConfidence: ptr(0.9),
		CategoryUserSet:    true,
	}
	repo := newFakeAssetRepo(existing)
	got, created, err := Resolve(testCtx(), repo, entity.Extraction{
		SerialNumber:       ptr("sticky-1"),
		AssetCategory:      ptr("other"),
		CategoryConfidence: ptr(0.5),
	}, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if created {
		t.Fatal("created = true, want false (should have merged into the existing asset)")
	}
	if got.ID != "STICKY-1" {
		t.Errorf("resolved asset ID = %q, want STICKY-1", got.ID)
	}
	// Assert via stored asset to be safe against returned-value quirks.
	stored, err := repo.Get(testCtx(), "STICKY-1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if stored.AssetCategory == nil || *stored.AssetCategory != "electronics" {
		t.Errorf("AssetCategory = %v, want electronics (user-set is sticky)", stored.AssetCategory)
	}
	if stored.CategoryConfidence == nil || *stored.CategoryConfidence != 0.9 {
		t.Errorf("CategoryConfidence = %v, want 0.9 (unchanged)", stored.CategoryConfidence)
	}
	if !stored.CategoryUserSet {
		t.Error("CategoryUserSet = false, want true (must remain user-set)")
	}
}

func TestMerge_CategoryHigherConfidenceReplaces(t *testing.T) {
	t.Parallel()

	existing := entity.Asset{
		ID:                 "CAT-UP",
		NormSerial:         ptr("CAT-UP"),
		AssetCategory:      ptr("other"),
		CategoryConfidence: ptr(0.5),
		CategoryUserSet:    false,
	}
	repo := newFakeAssetRepo(existing)
	got, _, err := Resolve(testCtx(), repo, entity.Extraction{
		SerialNumber:       ptr("cat-up"),
		AssetCategory:      ptr("appliance"),
		CategoryConfidence: ptr(0.9),
	}, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.ID != "CAT-UP" {
		t.Errorf("resolved asset ID = %q, want CAT-UP", got.ID)
	}
	stored, err := repo.Get(testCtx(), "CAT-UP")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if stored.AssetCategory == nil || *stored.AssetCategory != "appliance" {
		t.Errorf("AssetCategory = %v, want appliance (higher confidence)", stored.AssetCategory)
	}
	if stored.CategoryConfidence == nil || *stored.CategoryConfidence != 0.9 {
		t.Errorf("CategoryConfidence = %v, want 0.9", stored.CategoryConfidence)
	}
	if stored.CategoryUserSet {
		t.Error("CategoryUserSet = true, want false")
	}
}

func TestMerge_CategoryLowerConfidenceKeepsExisting(t *testing.T) {
	t.Parallel()

	existing := entity.Asset{
		ID:                 "CAT-DN",
		NormSerial:         ptr("CAT-DN"),
		AssetCategory:      ptr("appliance"),
		CategoryConfidence: ptr(0.9),
		CategoryUserSet:    false,
	}
	repo := newFakeAssetRepo(existing)
	_, _, err := Resolve(testCtx(), repo, entity.Extraction{
		SerialNumber:       ptr("cat-dn"),
		AssetCategory:      ptr("other"),
		CategoryConfidence: ptr(0.5),
	}, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	stored, err := repo.Get(testCtx(), "CAT-DN")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if stored.AssetCategory == nil || *stored.AssetCategory != "appliance" {
		t.Errorf("AssetCategory = %v, want appliance (unchanged)", stored.AssetCategory)
	}
	if stored.CategoryConfidence == nil || *stored.CategoryConfidence != 0.9 {
		t.Errorf("CategoryConfidence = %v, want 0.9 (unchanged)", stored.CategoryConfidence)
	}
}

func TestMerge_NullCategoryAndNameNeverErase(t *testing.T) {
	t.Parallel()

	existing := entity.Asset{
		ID:                 "NULL-1",
		Name:               ptr("Old Name"),
		AssetCategory:      ptr("appliance"),
		CategoryConfidence: ptr(0.8),
		NormSerial:         ptr("NULL-1"),
		CategoryUserSet:    false,
	}
	repo := newFakeAssetRepo(existing)
	_, _, err := Resolve(testCtx(), repo, entity.Extraction{
		SerialNumber: ptr("null-1"),
	}, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	stored, err := repo.Get(testCtx(), "NULL-1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if stored.Name == nil || *stored.Name != "Old Name" {
		t.Errorf("Name = %v, want \"Old Name\" (nil extraction must not erase)", stored.Name)
	}
	if stored.AssetCategory == nil || *stored.AssetCategory != "appliance" {
		t.Errorf("AssetCategory = %v, want appliance (unchanged)", stored.AssetCategory)
	}
	if stored.CategoryConfidence == nil || *stored.CategoryConfidence != 0.8 {
		t.Errorf("CategoryConfidence = %v, want 0.8 (unchanged)", stored.CategoryConfidence)
	}
}

func TestMerge_NewNameWins(t *testing.T) {
	t.Parallel()

	existing := entity.Asset{
		ID:         "NAME-1",
		Name:       ptr("Old"),
		NormSerial: ptr("NAME-1"),
	}
	repo := newFakeAssetRepo(existing)
	got, _, err := Resolve(testCtx(), repo, entity.Extraction{
		SerialNumber: ptr("name-1"),
		Name:         ptr("New Name"),
	}, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.ID != "NAME-1" {
		t.Errorf("resolved asset ID = %q, want NAME-1", got.ID)
	}
	stored, err := repo.Get(testCtx(), "NAME-1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if stored.Name == nil || *stored.Name != "New Name" {
		t.Errorf("Name = %v, want \"New Name\"", stored.Name)
	}
}

func TestNewAsset_NameCategoryConfidenceStamped(t *testing.T) {
	t.Parallel()

	repo := newFakeAssetRepo()
	got, created, err := Resolve(testCtx(), repo, entity.Extraction{
		SerialNumber:       ptr("FRESH-1"),
		Name:               ptr("Microwave Oven"),
		AssetCategory:      ptr("appliance"),
		CategoryConfidence: ptr(0.6),
	}, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	stored, err := repo.Get(testCtx(), got.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if stored.Name == nil || *stored.Name != "Microwave Oven" {
		t.Errorf("Name = %v, want \"Microwave Oven\"", stored.Name)
	}
	if stored.AssetCategory == nil || *stored.AssetCategory != "appliance" {
		t.Errorf("AssetCategory = %v, want appliance", stored.AssetCategory)
	}
	if stored.CategoryConfidence == nil || *stored.CategoryConfidence != 0.6 {
		t.Errorf("CategoryConfidence = %v, want 0.6", stored.CategoryConfidence)
	}
	if stored.CategoryUserSet {
		t.Error("CategoryUserSet = true, want false (a fresh asset is never user-set)")
	}
	if stored.NormName == nil || *stored.NormName != "microwave oven" {
		t.Errorf("NormName = %v, want \"microwave oven\"", stored.NormName)
	}
}

func TestMatch_SerialShortCircuit(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		SerialNumber: ptr("SN-1"),
		Brand:        ptr("LG"),
		Model:        ptr("W1"),
		Name:         ptr("Cool Box"),
	}
	repo := newFakeAssetRepo(
		entity.Asset{
			ID:         "A",
			NormSerial: ptr("SN-1"),
			NormBrand:  ptr("lg"),
			NormModel:  ptr("w1"),
			NormName:   ptr("cool box"),
		},
	)
	m, err := Match(testCtx(), repo, ext, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if m.Kind != MatchMerge {
		t.Fatalf("Kind = %v, want MatchMerge", m.Kind)
	}
	if m.AssetID == nil || *m.AssetID != "A" {
		t.Fatalf("AssetID = %v, want A", m.AssetID)
	}
	stages := repo.listStages()
	if len(stages) != 1 || stages[0] != "norm_serial" {
		t.Fatalf("listStages = %v, want [norm_serial]", stages)
	}
}

func TestMatch_BrandModelFallback(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		Brand: ptr("LG"),
		Model: ptr("W1"),
	}
	repo := newFakeAssetRepo(
		entity.Asset{
			ID:        "B",
			NormBrand: ptr("lg"),
			NormModel: ptr("w1"),
		},
	)
	m, err := Match(testCtx(), repo, ext, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if m.Kind != MatchMerge {
		t.Fatalf("Kind = %v, want MatchMerge", m.Kind)
	}
	if m.AssetID == nil || *m.AssetID != "B" {
		t.Fatalf("AssetID = %v, want B", m.AssetID)
	}
	stages := repo.listStages()
	if len(stages) != 1 || stages[0] != "norm_brand+norm_model" {
		t.Fatalf("listStages = %v, want [norm_brand+norm_model]", stages)
	}
}

func TestMatch_NameModelFallback(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		Name:  ptr("Cool Box"),
		Model: ptr("W1"),
	}
	repo := newFakeAssetRepo(
		entity.Asset{
			ID:        "C",
			NormName:  ptr("cool box"),
			NormModel: ptr("w1"),
		},
	)
	m, err := Match(testCtx(), repo, ext, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if m.Kind != MatchMerge {
		t.Fatalf("Kind = %v, want MatchMerge", m.Kind)
	}
	if m.AssetID == nil || *m.AssetID != "C" {
		t.Fatalf("AssetID = %v, want C", m.AssetID)
	}
	stages := repo.listStages()
	if len(stages) != 1 || stages[0] != "norm_model+norm_name" {
		t.Fatalf("listStages = %v, want [norm_model+norm_name]", stages)
	}
}

func TestMatch_Ambiguous(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		Brand: ptr("LG"),
		Model: ptr("W1"),
	}
	repo := newFakeAssetRepo(
		entity.Asset{ID: "D1", NormBrand: ptr("lg"), NormModel: ptr("w1")},
		entity.Asset{ID: "D2", NormBrand: ptr("lg"), NormModel: ptr("w1")},
	)
	m, err := Match(testCtx(), repo, ext, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if m.Kind != MatchAmbiguous {
		t.Fatalf("Kind = %v, want MatchAmbiguous", m.Kind)
	}
	if m.AssetID != nil {
		t.Fatalf("AssetID = %v, want nil", *m.AssetID)
	}
	if len(m.Candidates) != 2 {
		t.Fatalf("Candidates = %v, want 2", m.Candidates)
	}
}

func TestMatch_SoftDeletedNotMatched(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		Brand: ptr("LG"),
		Model: ptr("W1"),
	}
	repo := newFakeAssetRepo(
		entity.Asset{ID: "E1", NormBrand: ptr("lg"), NormModel: ptr("w1")},
		entity.Asset{ID: "E2", NormBrand: ptr("lg"), NormModel: ptr("w1"), DeletedAt: ptr(time.Now())},
	)
	m, err := Match(testCtx(), repo, ext, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if m.Kind != MatchMerge {
		t.Fatalf("Kind = %v, want MatchMerge", m.Kind)
	}
	if m.AssetID == nil || *m.AssetID != "E1" {
		t.Fatalf("AssetID = %v, want E1", m.AssetID)
	}
	if len(m.Candidates) != 1 || m.Candidates[0].ID != "E1" {
		t.Fatalf("Candidates = %v, want [E1]", m.Candidates)
	}
}

func TestMatch_DeletedOnlySurfaced(t *testing.T) {
	t.Parallel()

	ext := entity.Extraction{
		Brand: ptr("LG"),
		Model: ptr("W1"),
	}
	repo := newFakeAssetRepo(
		entity.Asset{ID: "F1", NormBrand: ptr("lg"), NormModel: ptr("w1"), DeletedAt: ptr(time.Now())},
	)
	m, err := Match(testCtx(), repo, ext, nil, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if m.Kind != MatchSoftDeleted {
		t.Fatalf("Kind = %v, want MatchSoftDeleted", m.Kind)
	}
	if m.AssetID != nil {
		t.Fatalf("AssetID = %v, want nil", *m.AssetID)
	}
	if len(m.Candidates) != 1 || m.Candidates[0].ID != "F1" {
		t.Fatalf("Candidates = %v, want [F1]", m.Candidates)
	}
}
