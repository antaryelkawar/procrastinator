package identity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/commons-data"
)

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// fakeRepo is a configurable in-memory implementation of data.AssetRepository
// with mutex protection and call recording for assertions.
type fakeRepo struct {
	mu sync.Mutex

	assets map[string]data.Asset
	nextID int

	// calls records method names and their key arguments in order.
	calls []string

	// createErr is returned by Create and CreateOnConflictSerial when non-nil.
	createErr error

	// conflict enables fault injection on CreateOnConflictSerial:
	// when true, returns conflictWinner instead of storing the new asset.
	conflict       bool
	conflictWinner data.Asset

	// skipFindSerial simulates the race window: FindBySerial returns ErrNotFound
	// even if the asset exists in the map.
	skipFindSerial bool
}

// newFakeRepo returns a fakeRepo seeded with the given assets (keyed by ID).
func newFakeRepo(assets ...data.Asset) *fakeRepo {
	r := &fakeRepo{
		assets: make(map[string]data.Asset, len(assets)),
	}
	for _, a := range assets {
		r.assets[a.ID] = a
	}
	return r
}

func (r *fakeRepo) Create(_ context.Context, a data.Asset) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return data.Asset{}, r.createErr
	}
	if a.ID == "" {
		r.nextID++
		a.ID = "id-" + string(rune('A'+r.nextID-1))
	}
	r.assets[a.ID] = a
	r.calls = append(r.calls, "Create:"+a.ID)
	return a, nil
}

func (r *fakeRepo) CreateOnConflictSerial(_ context.Context, a data.Asset) (data.Asset, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return data.Asset{}, false, r.createErr
	}
	if r.conflict {
		r.calls = append(r.calls, "CreateOnConflictSerial:conflict")
		return r.conflictWinner, true, nil
	}
	if a.ID == "" {
		r.nextID++
		a.ID = "id-" + string(rune('A'+r.nextID-1))
	}
	r.assets[a.ID] = a
	r.calls = append(r.calls, "CreateOnConflictSerial:"+a.ID)
	return a, false, nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "GetByID:"+id)
	a, ok := r.assets[id]
	if !ok {
		return data.Asset{}, data.ErrNotFound
	}
	return a, nil
}

func (r *fakeRepo) List(_ context.Context) ([]data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "List")
	out := make([]data.Asset, 0, len(r.assets))
	for _, a := range r.assets {
		out = append(out, a)
	}
	return out, nil
}

func (r *fakeRepo) FindBySerial(_ context.Context, normSerial string) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "FindBySerial:"+normSerial)
	if r.skipFindSerial {
		return data.Asset{}, data.ErrNotFound
	}
	for _, a := range r.assets {
		if a.NormSerial != nil && *a.NormSerial == normSerial {
			return a, nil
		}
	}
	return data.Asset{}, data.ErrNotFound
}

func (r *fakeRepo) FindByBrandModel(_ context.Context, normBrand, normModel string) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "FindByBrandModel:"+normBrand+"/"+normModel)
	for _, a := range r.assets {
		if a.NormBrand != nil && a.NormModel != nil &&
			*a.NormBrand == normBrand && *a.NormModel == normModel {
			return a, nil
		}
	}
	return data.Asset{}, data.ErrNotFound
}

func (r *fakeRepo) UpdateFields(_ context.Context, id string, f data.UpdateFields) (data.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "UpdateFields:"+id)
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
		if a.Metadata == nil {
			a.Metadata = make(map[string]any, len(f.Metadata))
		}
		for k, v := range f.Metadata {
			a.Metadata[k] = v
		}
	}
	a.UpdatedAt = time.Now()
	r.assets[id] = a
	return a, nil
}

// callsWith returns the recorded calls matching a prefix.
func (r *fakeRepo) callsWith(prefix string) []string {
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
		extraction  data.Extraction
		repoAssets  []data.Asset
		wantID      string
		wantCreated bool
		wantCreate  bool
	}{
		{
			name: "serial match found",
			extraction: data.Extraction{
				SerialNumber: ptr("SN-123"),
				Brand:        ptr("LG"),
				Model:        ptr("W1234"),
			},
			repoAssets: []data.Asset{
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
			extraction: data.Extraction{
				SerialNumber: ptr("SN-456"),
			},
			repoAssets: []data.Asset{
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
			repo := newFakeRepo(tc.repoAssets...)
			got, created, err := Resolve(context.Background(), repo, tc.extraction)
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
		extraction  data.Extraction
		repoAssets  []data.Asset
		wantID      string
		wantCreated bool
		wantCreate  bool
	}{
		{
			name: "both brand and model present, match found",
			extraction: data.Extraction{
				Brand: ptr("samsung"),
				Model: ptr("WW90"),
			},
			repoAssets: []data.Asset{
				{ID: "D", NormBrand: ptr("samsung"), NormModel: ptr("ww90"), Brand: ptr("samsung"), Model: ptr("WW90")},
			},
			wantID:      "D",
			wantCreated: false,
			wantCreate:  false,
		},
		{
			name: "brand alone, no match",
			extraction: data.Extraction{
				Brand: ptr("LG"),
			},
			repoAssets:  []data.Asset{},
			wantID:      "", // new asset gets a generated ID
			wantCreated: true,
			wantCreate:  true,
		},
		{
			name: "model alone, no match",
			extraction: data.Extraction{
				Model: ptr("W1234"),
			},
			repoAssets:  []data.Asset{},
			wantID:      "", // new asset gets a generated ID
			wantCreated: true,
			wantCreate:  true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepo(tc.repoAssets...)
			got, created, err := Resolve(context.Background(), repo, tc.extraction)
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
			// When a brand+model match is found, FindByBrandModel should have been called.
			if tc.name == "both brand and model present, match found" {
				if len(repo.callsWith("FindByBrandModel")) == 0 {
					t.Error("expected FindByBrandModel to be called, got none")
				}
			}
		})
	}
}

func TestResolve_NoIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		extraction data.Extraction
	}{
		{
			name: "no serial no brand no model",
			extraction: data.Extraction{
				Price:    ptr("999.99"),
				Currency: ptr("EUR"),
			},
		},
		{
			name: "empty strings all identity fields",
			extraction: data.Extraction{
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
			repo := newFakeRepo()
			_, _, err := Resolve(context.Background(), repo, tc.extraction)
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
		extraction  data.Extraction
		wantCreated bool
	}{
		{
			name: "serial present, no existing",
			extraction: data.Extraction{
				SerialNumber: ptr("NEW-SN"),
			},
			wantCreated: true,
		},
		{
			name: "brand+model only, no existing",
			extraction: data.Extraction{
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
			repo := newFakeRepo()
			got, created, err := Resolve(context.Background(), repo, tc.extraction)
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
		existing   data.Asset
		extraction data.Extraction
		wantFields data.UpdateFields
	}{
		{
			name: "non-nil extraction fields overwrite",
			existing: data.Asset{
				ID:           "M1",
				Brand:        ptr("Samsung"),
				Model:        ptr("Old"),
				SerialNumber: ptr("M1-SN"),
				NormSerial:   ptr("M1-SN"),
			},
			extraction: data.Extraction{
				SerialNumber: ptr("M1-SN"),
				Model:        ptr("New"),
				Price:        ptr("999.99"),
			},
			wantFields: data.UpdateFields{
				Model: ptr("New"),
				Price: ptr("999.99"),
				Brand: nil, // untouched
			},
		},
		{
			name: "nil extraction fields never erase",
			existing: data.Asset{
				ID:           "M2",
				Price:        ptr("500.00"),
				Brand:        ptr("LG"),
				SerialNumber: ptr("M2-SN"),
				NormSerial:   ptr("M2-SN"),
			},
			extraction: data.Extraction{
				SerialNumber: ptr("M2-SN"),
				WarrantyEnd:  ptr(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
			wantFields: data.UpdateFields{
				WarrantyEnd: ptr(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
				Price:       nil,
				Brand:       nil,
			},
		},
		{
			name: "price retained when extraction has nil price",
			existing: data.Asset{
				ID:           "M3",
				Price:        ptr("399.99"),
				SerialNumber: ptr("M3-SN"),
				NormSerial:   ptr("M3-SN"),
			},
			extraction: data.Extraction{
				SerialNumber: ptr("M3-SN"),
				Model:        ptr("X"),
			},
			wantFields: data.UpdateFields{
				Model: ptr("X"),
				Price: nil, // must stay nil so price is retained
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepo(tc.existing)
			_, _, err := Resolve(context.Background(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}

			// Verify UpdateFields was called with the expected field set.
			// We can't directly inspect the UpdateFields passed to the repo,
			// so we verify via the resulting asset state:
			// - non-nil extraction fields are applied to the asset
			// - nil extraction fields leave the asset unchanged
			stored, err := repo.GetByID(context.Background(), tc.existing.ID)
			if err != nil {
				t.Fatalf("GetByID returned error: %v", err)
			}

			// Model: if extraction has non-nil model, asset should have it;
			// if nil, asset should retain original.
			if tc.extraction.Model != nil {
				if stored.Model == nil || *stored.Model != *tc.extraction.Model {
					t.Errorf("Model = %v, want %q", stored.Model, *tc.extraction.Model)
				}
			} else {
				if stored.Model != tc.existing.Model {
					t.Errorf("Model = %v, want original %v (untouched)", stored.Model, tc.existing.Model)
				}
			}

			// Price: same logic
			if tc.extraction.Price != nil {
				if stored.Price == nil || *stored.Price != *tc.extraction.Price {
					t.Errorf("Price = %v, want %q", stored.Price, *tc.extraction.Price)
				}
			} else {
				if stored.Price != tc.existing.Price {
					t.Errorf("Price = %v, want original %v (untouched)", stored.Price, tc.existing.Price)
				}
			}

			// Brand: same logic
			if tc.extraction.Brand != nil {
				if stored.Brand == nil || *stored.Brand != *tc.extraction.Brand {
					t.Errorf("Brand = %v, want %q", stored.Brand, *tc.extraction.Brand)
				}
			} else {
				if stored.Brand != tc.existing.Brand {
					t.Errorf("Brand = %v, want original %v (untouched)", stored.Brand, tc.existing.Brand)
				}
			}

			// WarrantyEnd: same logic
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
		existing   data.Asset
		extraction data.Extraction
		wantMeta   map[string]any
	}{
		{
			name: "new metadata keys added",
			existing: data.Asset{
				ID:         "MD1",
				NormSerial: ptr("MD1-SN"),
				Metadata:   map[string]any{"amc_card_number": "C1"},
			},
			extraction: data.Extraction{
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
			existing: data.Asset{
				ID:         "MD2",
				NormSerial: ptr("MD2-SN"),
				Metadata:   map[string]any{"name": "old"},
			},
			extraction: data.Extraction{
				SerialNumber: ptr("MD2-SN"),
				Metadata:     map[string]any{"name": "new"},
			},
			wantMeta: map[string]any{
				"name": "new",
			},
		},
		{
			name: "extraction metadata nil",
			existing: data.Asset{
				ID:         "MD3",
				NormSerial: ptr("MD3-SN"),
				Metadata:   map[string]any{"a": "b"},
			},
			extraction: data.Extraction{
				SerialNumber: ptr("MD3-SN"),
				Metadata:     nil,
			},
			wantMeta: map[string]any{"a": "b"}, // unchanged
		},
		{
			name: "both nil",
			existing: data.Asset{
				ID:         "MD4",
				NormSerial: ptr("MD4-SN"),
				Metadata:   nil,
			},
			extraction: data.Extraction{
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
			repo := newFakeRepo(tc.existing)
			_, _, err := Resolve(context.Background(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			stored, err := repo.GetByID(context.Background(), tc.existing.ID)
			if err != nil {
				t.Fatalf("GetByID returned error: %v", err)
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
		existing   data.Asset
		extraction data.Extraction
		wantDoc    string
	}{
		{
			name: "doc_type updated on merge",
			existing: data.Asset{
				ID:         "DT1",
				NormSerial: ptr("DT1-SN"),
				DocType:    "invoice",
			},
			extraction: data.Extraction{
				SerialNumber:   ptr("DT1-SN"),
				Classification: "amc",
			},
			wantDoc: "amc",
		},
		{
			name:     "doc_type set on create",
			existing: data.Asset{},
			extraction: data.Extraction{
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
			var repo *fakeRepo
			var wantID string
			if tc.existing.ID != "" {
				repo = newFakeRepo(tc.existing)
				wantID = tc.existing.ID
			} else {
				repo = newFakeRepo()
			}

			got, _, err := Resolve(context.Background(), repo, tc.extraction)
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

func TestResolve_ConflictPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		extraction  data.Extraction
		winner      data.Asset
		wantMerge   bool
		wantCreated bool
	}{
		{
			name: "serial conflict, merge into winner",
			extraction: data.Extraction{
				SerialNumber: ptr("RACE-SN"),
				Brand:        ptr("LG"),
			},
			winner: data.Asset{
				ID:         "W1",
				NormSerial: ptr("RACE-SN"),
				Brand:      ptr("LG"),
			},
			wantMerge:   true,
			wantCreated: false,
		},
		{
			name: "serial conflict, winner has metadata",
			extraction: data.Extraction{
				SerialNumber: ptr("RACE-SN2"),
				Metadata:     map[string]any{"new": "val"},
			},
			winner: data.Asset{
				ID:         "W2",
				NormSerial: ptr("RACE-SN2"),
				Metadata:   map[string]any{"existing": "key"},
			},
			wantMerge:   true,
			wantCreated: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Seed the winner so UpdateFields can find it; skipFindSerial simulates
			// the race window where FindBySerial misses but the serial is taken by
			// the time CreateOnConflictSerial runs.
			repo := newFakeRepo(tc.winner)
			repo.conflict = true
			repo.conflictWinner = tc.winner
			repo.skipFindSerial = true

			got, created, err := Resolve(context.Background(), repo, tc.extraction)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if got.ID != tc.winner.ID {
				t.Errorf("resolved asset ID = %q, want %q (winner)", got.ID, tc.winner.ID)
			}
			if created != tc.wantCreated {
				t.Errorf("created = %v, want %v", created, tc.wantCreated)
			}
			if !tc.wantMerge && len(repo.callsWith("UpdateFields")) != 0 {
				t.Errorf("expected no UpdateFields call, got %v", repo.callsWith("UpdateFields"))
			}
			if tc.wantMerge && len(repo.callsWith("UpdateFields")) == 0 {
				t.Error("expected UpdateFields to be called on the winner, got none")
			}

			// For the metadata conflict case, verify the merged metadata.
			if tc.name == "serial conflict, winner has metadata" {
				stored, err := repo.GetByID(context.Background(), tc.winner.ID)
				if err != nil {
					t.Fatalf("GetByID returned error: %v", err)
				}
				if stored.Metadata["existing"] != "key" {
					t.Errorf("Metadata[\"existing\"] = %v, want \"key\"", stored.Metadata["existing"])
				}
				if stored.Metadata["new"] != "val" {
					t.Errorf("Metadata[\"new\"] = %v, want \"val\"", stored.Metadata["new"])
				}
			}
		})
	}
}
