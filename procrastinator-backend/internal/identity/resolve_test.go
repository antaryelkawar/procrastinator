package identity

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"procrastinator-backend/internal/llm/extraction"
)

// Compile-time assertion that fakeStore implements AssetStore.
var _ AssetStore = (*fakeStore)(nil)

type fakeRow struct {
	a  Asset
	nb string // normalized brand
	nm string // normalized model
	ns string // normalized serial
}

type fakeStore struct {
	mu    sync.Mutex
	rows  map[string]fakeRow
	count int

	created          []Asset
	conflictCreates  []Asset
	updateIDs        []string
	lastUpdate       UpdateFields
	skipSerialLookup bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		rows: make(map[string]fakeRow),
	}
}

func (f *fakeStore) seed(t *testing.T, a Asset) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()

	f.rows[a.ID] = fakeRow{
		a:  copyAsset(a),
		nb: normNamePtr(a.Brand),
		nm: normNamePtr(a.Model),
		ns: normSerialPtr(a.SerialNumber),
	}
}

func normNamePtr(p *string) string {
	if p == nil {
		return ""
	}
	return NormalizeName(*p)
}

func normSerialPtr(p *string) string {
	if p == nil {
		return ""
	}
	return NormalizeSerial(*p)
}

func (f *fakeStore) FindBySerial(_ context.Context, normSerial string) (Asset, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.skipSerialLookup {
		return Asset{}, false, nil
	}

	for _, row := range f.rows {
		if row.ns != "" && row.ns == normSerial {
			return copyAsset(row.a), true, nil
		}
	}
	return Asset{}, false, nil
}

func (f *fakeStore) FindByBrandModel(_ context.Context, normBrand, normModel string) (Asset, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, row := range f.rows {
		if row.nb != "" && row.nm != "" && row.nb == normBrand && row.nm == normModel {
			return copyAsset(row.a), true, nil
		}
	}
	return Asset{}, false, nil
}

func (f *fakeStore) Create(_ context.Context, a Asset) (Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if a.ID == "" {
		f.count++
		a.ID = fmt.Sprintf("gen-%d", f.count)
	}

	row := fakeRow{
		a:  copyAsset(a),
		nb: normNamePtr(a.Brand),
		nm: normNamePtr(a.Model),
		ns: normSerialPtr(a.SerialNumber),
	}
	f.rows[a.ID] = row
	f.created = append(f.created, copyAsset(a))
	return copyAsset(a), nil
}

func (f *fakeStore) CreateOnConflictSerial(_ context.Context, a Asset) (Asset, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.conflictCreates = append(f.conflictCreates, copyAsset(a))

	normSerial := normSerialPtr(a.SerialNumber)
	if normSerial != "" {
		for _, row := range f.rows {
			if row.ns == normSerial {
				return copyAsset(row.a), false, nil
			}
		}
	}

	if a.ID == "" {
		f.count++
		a.ID = fmt.Sprintf("gen-%d", f.count)
	}

	row := fakeRow{
		a:  copyAsset(a),
		nb: normNamePtr(a.Brand),
		nm: normNamePtr(a.Model),
		ns: normSerialPtr(a.SerialNumber),
	}
	f.rows[a.ID] = row
	return copyAsset(a), true, nil
}

func (f *fakeStore) UpdateFields(_ context.Context, id string, fields UpdateFields) (Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	row, ok := f.rows[id]
	if !ok {
		return Asset{}, fmt.Errorf("asset not found: %s", id)
	}

	if fields.Brand != nil {
		row.a.Brand = strPtr(*fields.Brand)
		row.nb = NormalizeName(*fields.Brand)
	}
	if fields.Model != nil {
		row.a.Model = strPtr(*fields.Model)
		row.nm = NormalizeName(*fields.Model)
	}
	if fields.SerialNumber != nil {
		row.a.SerialNumber = strPtr(*fields.SerialNumber)
		row.ns = NormalizeSerial(*fields.SerialNumber)
	}
	if fields.PurchaseDate != nil {
		t := *fields.PurchaseDate
		row.a.PurchaseDate = &t
	}
	if fields.Price != nil {
		row.a.Price = strPtr(*fields.Price)
	}
	if fields.Currency != nil {
		row.a.Currency = strPtr(*fields.Currency)
	}
	if fields.WarrantyStart != nil {
		t := *fields.WarrantyStart
		row.a.WarrantyStart = &t
	}
	if fields.WarrantyEnd != nil {
		t := *fields.WarrantyEnd
		row.a.WarrantyEnd = &t
	}

	f.rows[id] = row
	f.lastUpdate = copyUpdateFields(fields)
	f.updateIDs = append(f.updateIDs, id)
	return copyAsset(row.a), nil
}

func copyAsset(a Asset) Asset {
	c := a
	if a.Brand != nil {
		s := *a.Brand
		c.Brand = &s
	}
	if a.Model != nil {
		s := *a.Model
		c.Model = &s
	}
	if a.SerialNumber != nil {
		s := *a.SerialNumber
		c.SerialNumber = &s
	}
	if a.PurchaseDate != nil {
		t := *a.PurchaseDate
		c.PurchaseDate = &t
	}
	if a.Price != nil {
		s := *a.Price
		c.Price = &s
	}
	if a.Currency != nil {
		s := *a.Currency
		c.Currency = &s
	}
	if a.WarrantyStart != nil {
		t := *a.WarrantyStart
		c.WarrantyStart = &t
	}
	if a.WarrantyEnd != nil {
		t := *a.WarrantyEnd
		c.WarrantyEnd = &t
	}
	return c
}

func copyUpdateFields(u UpdateFields) UpdateFields {
	c := u
	if u.Brand != nil {
		s := *u.Brand
		c.Brand = &s
	}
	if u.Model != nil {
		s := *u.Model
		c.Model = &s
	}
	if u.SerialNumber != nil {
		s := *u.SerialNumber
		c.SerialNumber = &s
	}
	if u.PurchaseDate != nil {
		t := *u.PurchaseDate
		c.PurchaseDate = &t
	}
	if u.Price != nil {
		s := *u.Price
		c.Price = &s
	}
	if u.Currency != nil {
		s := *u.Currency
		c.Currency = &s
	}
	if u.WarrantyStart != nil {
		t := *u.WarrantyStart
		c.WarrantyStart = &t
	}
	if u.WarrantyEnd != nil {
		t := *u.WarrantyEnd
		c.WarrantyEnd = &t
	}
	return c
}

func strPtr(s string) *string {
	c := s
	return &c
}

// TestResolveSerialMatchWinsOverBrandModel ensures that when both a serial
// match and a brand+model match exist, the serial match takes precedence.
func TestResolveSerialMatchWinsOverBrandModel(t *testing.T) {
	t.Parallel()

	f := newFakeStore()
	f.seed(t, Asset{
		ID:           "a1",
		Brand:        strPtr("Alpha"),
		Model:        strPtr("A1"),
		SerialNumber: strPtr("SN-A"),
	})
	f.seed(t, Asset{
		ID:    "b1",
		Brand: strPtr("Beta"),
		Model: strPtr("B1"),
	})

	ctx := context.Background()
	ex := extraction.Extraction{
		Classification: "warranty",
		SerialNumber:   "sn-a",
		Brand:          "beta",
		Model:          "B1",
	}

	got, created, err := Resolve(ctx, f, ex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false, got true")
	}
	if got.ID != "a1" {
		t.Errorf("expected ID='a1', got %q", got.ID)
	}
	if len(f.created) != 0 {
		t.Errorf("expected 0 creates, got %d", len(f.created))
	}
	if len(f.conflictCreates) != 0 {
		t.Errorf("expected 0 conflict creates, got %d", len(f.conflictCreates))
	}
}

// TestResolveBrandModelMatch ensures brand+model matching works with
// normalized comparisons and rejects mismatches.
func TestResolveBrandModelMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		setup      func(t *testing.T, f *fakeStore)
		ex         extraction.Extraction
		wantID     string
		wantCreate bool
	}{
		{
			name: "case and whitespace variants match",
			setup: func(t *testing.T, f *fakeStore) {
				f.seed(t, Asset{
					ID:    "s1",
					Brand: strPtr("Samsung"),
					Model: strPtr("WW90T534DAW"),
				})
			},
			ex: extraction.Extraction{
				Brand: "samsung",
				Model: " WW90T534DAW ",
			},
			wantID:     "s1",
			wantCreate: false,
		},
		{
			name: "different model is not a match",
			setup: func(t *testing.T, f *fakeStore) {
				f.seed(t, Asset{
					ID:    "s2",
					Brand: strPtr("Samsung"),
					Model: strPtr("WW90"),
				})
			},
			ex: extraction.Extraction{
				Brand: "Samsung",
				Model: "WW90T534DAW",
			},
			wantID:     "",
			wantCreate: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeStore()
			tc.setup(t, f)

			ctx := context.Background()
			got, created, err := Resolve(ctx, f, tc.ex)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if created != tc.wantCreate {
				t.Errorf("created: got %v, want %v", created, tc.wantCreate)
			}
			if tc.wantCreate {
				if got.ID == "s2" {
					t.Error("expected new asset, got existing s2")
				}
				if len(f.updateIDs) != 0 {
					t.Errorf("expected 0 updates, got %d", len(f.updateIDs))
				}
			} else {
				if got.ID != tc.wantID {
					t.Errorf("ID: got %q, want %q", got.ID, tc.wantID)
				}
			}
		})
	}
}

// TestResolveBrandAloneIsNotAUsableIdentity ensures that brand or model
// alone (without the other or a serial) triggers ErrNoIdentity.
func TestResolveBrandAloneIsNotAUsableIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(*testing.T, *fakeStore)
		ex    extraction.Extraction
	}{
		{
			name: "brand only",
			setup: func(t *testing.T, f *fakeStore) {
				f.seed(t, Asset{
					ID:    "s1",
					Brand: strPtr("Samsung"),
				})
			},
			ex: extraction.Extraction{
				Brand: "Samsung",
			},
		},
		{
			name: "model only",
			setup: func(t *testing.T, f *fakeStore) {
				// no seed
			},
			ex: extraction.Extraction{
				Model: "W-200",
			},
		},
		{
			name: "brand with whitespace-only model",
			setup: func(t *testing.T, f *fakeStore) {
				// no seed
			},
			ex: extraction.Extraction{
				Brand: "Samsung",
				Model: "   ",
			},
		},
		{
			name: "whitespace-only serial with brand",
			setup: func(t *testing.T, f *fakeStore) {
				// no seed
			},
			ex: extraction.Extraction{
				SerialNumber: "  ",
				Brand:        "Samsung",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeStore()
			tc.setup(t, f)

			ctx := context.Background()
			got, created, err := Resolve(ctx, f, tc.ex)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrNoIdentity) {
				t.Errorf("expected ErrNoIdentity, got %v", err)
			}
			if created {
				t.Error("expected created=false")
			}
			var zero Asset
			if got != zero {
				t.Errorf("expected zero Asset, got %+v", got)
			}
			if len(f.created) != 0 {
				t.Errorf("expected 0 creates, got %d", len(f.created))
			}
			if len(f.conflictCreates) != 0 {
				t.Errorf("expected 0 conflict creates, got %d", len(f.conflictCreates))
			}
			if len(f.updateIDs) != 0 {
				t.Errorf("expected 0 updates, got %d", len(f.updateIDs))
			}
		})
	}
}

// TestResolveNoIdentity ensures that an extraction with no usable identity
// returns ErrNoIdentity without any store interactions.
func TestResolveNoIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ex   extraction.Extraction
	}{
		{
			name: "classification only",
			ex:   extraction.Extraction{Classification: "invoice"},
		},
		{
			name: "all fields empty",
			ex:   extraction.Extraction{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeStore()
			ctx := context.Background()
			got, created, err := Resolve(ctx, f, tc.ex)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrNoIdentity) {
				t.Errorf("expected ErrNoIdentity, got %v", err)
			}
			if created {
				t.Error("expected created=false")
			}
			var zero Asset
			if got != zero {
				t.Errorf("expected zero Asset, got %+v", got)
			}
			if len(f.created) != 0 {
				t.Errorf("expected 0 creates, got %d", len(f.created))
			}
			if len(f.conflictCreates) != 0 {
				t.Errorf("expected 0 conflict creates, got %d", len(f.conflictCreates))
			}
			if len(f.updateIDs) != 0 {
				t.Errorf("expected 0 updates, got %d", len(f.updateIDs))
			}
		})
	}
}

// TestResolveCreateOnNoMatch ensures that when no match is found, a new
// asset is created with all extracted fields populated correctly.
func TestResolveCreateOnNoMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		ex       extraction.Extraction
		validate func(t *testing.T, got Asset, created bool, f *fakeStore)
	}{
		{
			name: "new serial creates asset with all extracted fields",
			ex: extraction.Extraction{
				Classification: "invoice",
				Brand:          "  Samsung ",
				Model:          "WW90T534DAW",
				SerialNumber:   " sn-999 ",
				PurchaseDate:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				Price:          "39999.99",
				Currency:       "EUR",
				WarrantyStart:  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				WarrantyEnd:    time.Date(2028, 5, 1, 0, 0, 0, 0, time.UTC),
			},
			validate: func(t *testing.T, got Asset, created bool, f *fakeStore) {
				if !created {
					t.Error("expected created=true")
				}
				if got.ID == "" {
					t.Error("expected non-empty ID")
				}
				if got.Brand == nil || *got.Brand != "Samsung" {
					t.Errorf("Brand: got %v, want 'Samsung'", got.Brand)
				}
				if got.Model == nil || *got.Model != "WW90T534DAW" {
					t.Errorf("Model: got %v, want 'WW90T534DAW'", got.Model)
				}
				if got.SerialNumber == nil || *got.SerialNumber != "sn-999" {
					t.Errorf("SerialNumber: got %v, want 'sn-999'", got.SerialNumber)
				}
				if got.Price == nil || *got.Price != "39999.99" {
					t.Errorf("Price: got %v, want '39999.99'", got.Price)
				}
				if got.Currency == nil || *got.Currency != "EUR" {
					t.Errorf("Currency: got %v, want 'EUR'", got.Currency)
				}
				if got.PurchaseDate == nil || !got.PurchaseDate.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("PurchaseDate: got %v, want 2026-05-01", got.PurchaseDate)
				}
				if got.WarrantyStart == nil || !got.WarrantyStart.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("WarrantyStart: got %v, want 2026-05-01", got.WarrantyStart)
				}
				if got.WarrantyEnd == nil || !got.WarrantyEnd.Equal(time.Date(2028, 5, 1, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("WarrantyEnd: got %v, want 2028-05-01", got.WarrantyEnd)
				}
				// Serial-bearing creates must use the conflict-safe path
				// (design D4 step 5), not plain Create.
				if len(f.conflictCreates) != 1 {
					t.Errorf("expected 1 conflict-safe create, got %d", len(f.conflictCreates))
				} else {
					c := f.conflictCreates[0]
					if c.Brand == nil || *c.Brand != "Samsung" {
						t.Errorf("conflictCreates[0].Brand: got %v, want 'Samsung'", c.Brand)
					}
					if c.SerialNumber == nil || *c.SerialNumber != "sn-999" {
						t.Errorf("conflictCreates[0].SerialNumber: got %v, want 'sn-999'", c.SerialNumber)
					}
				}
				if len(f.created) != 0 {
					t.Errorf("expected 0 plain creates, got %d", len(f.created))
				}
				if len(f.updateIDs) != 0 {
					t.Errorf("expected 0 updates, got %d", len(f.updateIDs))
				}
			},
		},
		{
			name: "new brand+model without serial creates asset",
			ex: extraction.Extraction{
				Brand: "Acme",
				Model: "W-200",
				Price: "89.50",
			},
			validate: func(t *testing.T, got Asset, created bool, f *fakeStore) {
				if !created {
					t.Error("expected created=true")
				}
				if got.SerialNumber != nil {
					t.Errorf("expected nil SerialNumber, got %v", got.SerialNumber)
				}
				if got.Price == nil || *got.Price != "89.50" {
					t.Errorf("Price: got %v, want '89.50'", got.Price)
				}
				if len(f.conflictCreates) != 0 {
					t.Errorf("expected 0 conflict creates, got %d", len(f.conflictCreates))
				}
			},
		},
		{
			name: "absent extraction fields stay absent on create",
			ex: extraction.Extraction{
				Brand: "Bosch",
				Model: "SERIES6",
			},
			validate: func(t *testing.T, got Asset, created bool, f *fakeStore) {
				if !created {
					t.Error("expected created=true")
				}
				if got.Price != nil {
					t.Errorf("expected nil Price, got %v", got.Price)
				}
				if got.Currency != nil {
					t.Errorf("expected nil Currency, got %v", got.Currency)
				}
				if got.PurchaseDate != nil {
					t.Errorf("expected nil PurchaseDate, got %v", got.PurchaseDate)
				}
				if got.WarrantyStart != nil {
					t.Errorf("expected nil WarrantyStart, got %v", got.WarrantyStart)
				}
				if got.WarrantyEnd != nil {
					t.Errorf("expected nil WarrantyEnd, got %v", got.WarrantyEnd)
				}
				if got.SerialNumber != nil {
					t.Errorf("expected nil SerialNumber, got %v", got.SerialNumber)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeStore()
			ctx := context.Background()
			got, created, err := Resolve(ctx, f, tc.ex)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.validate(t, got, created, f)
		})
	}
}

// TestResolveMerge ensures that on update, non-empty extracted values
// overwrite existing fields, but absent/null values never erase them.
func TestResolveMerge(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		setup    func(*testing.T, *fakeStore)
		ex       extraction.Extraction
		validate func(t *testing.T, got Asset, created bool, f *fakeStore)
	}{
		{
			name: "warranty dates added, price retained when extraction price is null",
			setup: func(t *testing.T, f *fakeStore) {
				f.seed(t, Asset{
					ID:           "m1",
					Brand:        strPtr("Samsung"),
					Model:        strPtr("WW90T534DAW"),
					SerialNumber: strPtr("SN1"),
					PurchaseDate: timePtr(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)),
					Price:        strPtr("100.00"),
					Currency:     strPtr("EUR"),
				})
			},
			ex: extraction.Extraction{
				Classification: "warranty",
				SerialNumber:   " sn1 ",
				WarrantyStart:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				WarrantyEnd:    time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			validate: func(t *testing.T, got Asset, created bool, f *fakeStore) {
				if created {
					t.Error("expected created=false")
				}
				if got.ID != "m1" {
					t.Errorf("expected ID='m1', got %q", got.ID)
				}
				if got.Price == nil || *got.Price != "100.00" {
					t.Errorf("Price: got %v, want '100.00'", got.Price)
				}
				if got.Currency == nil || *got.Currency != "EUR" {
					t.Errorf("Currency: got %v, want 'EUR'", got.Currency)
				}
				if got.Brand == nil || *got.Brand != "Samsung" {
					t.Errorf("Brand: got %v, want 'Samsung'", got.Brand)
				}
				if got.Model == nil || *got.Model != "WW90T534DAW" {
					t.Errorf("Model: got %v, want 'WW90T534DAW'", got.Model)
				}
				if got.PurchaseDate == nil || !got.PurchaseDate.Equal(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("PurchaseDate: got %v, want 2026-01-15", got.PurchaseDate)
				}
				if got.WarrantyStart == nil || !got.WarrantyStart.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("WarrantyStart: got %v, want 2026-01-01", got.WarrantyStart)
				}
				if got.WarrantyEnd == nil || !got.WarrantyEnd.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("WarrantyEnd: got %v, want 2027-01-01", got.WarrantyEnd)
				}
				if got.SerialNumber == nil || *got.SerialNumber != "sn1" {
					t.Errorf("SerialNumber: got %v, want 'sn1'", got.SerialNumber)
				}

				names := updateFieldNames(f.lastUpdate)
				want := []string{"SerialNumber", "WarrantyEnd", "WarrantyStart"}
				sort.Strings(names)
				if len(names) != len(want) {
					t.Errorf("lastUpdate non-nil fields: got %v, want %v", names, want)
				} else {
					for i := range names {
						if names[i] != want[i] {
							t.Errorf("lastUpdate non-nil fields: got %v, want %v", names, want)
							break
						}
					}
				}
			},
		},
		{
			name: "model upgraded",
			setup: func(t *testing.T, f *fakeStore) {
				f.seed(t, Asset{
					ID:           "m2",
					Brand:        strPtr("Samsung"),
					Model:        strPtr("WW90"),
					SerialNumber: strPtr("SN2"),
				})
			},
			ex: extraction.Extraction{
				SerialNumber: "SN2",
				Model:        "WW90T534DAW",
			},
			validate: func(t *testing.T, got Asset, created bool, f *fakeStore) {
				if created {
					t.Error("expected created=false")
				}
				if got.ID != "m2" {
					t.Errorf("expected ID='m2', got %q", got.ID)
				}
				if got.Model == nil || *got.Model != "WW90T534DAW" {
					t.Errorf("Model: got %v, want 'WW90T534DAW'", got.Model)
				}
				if got.Brand == nil || *got.Brand != "Samsung" {
					t.Errorf("Brand: got %v, want 'Samsung'", got.Brand)
				}

				names := updateFieldNames(f.lastUpdate)
				want := []string{"Model", "SerialNumber"}
				sort.Strings(names)
				if len(names) != len(want) {
					t.Errorf("lastUpdate non-nil fields: got %v, want %v", names, want)
				} else {
					for i := range names {
						if names[i] != want[i] {
							t.Errorf("lastUpdate non-nil fields: got %v, want %v", names, want)
							break
						}
					}
				}
			},
		},
		{
			name: "brand+model match merges price into existing asset",
			setup: func(t *testing.T, f *fakeStore) {
				f.seed(t, Asset{
					ID:    "m3",
					Brand: strPtr("Bosch"),
					Model: strPtr("WAA28460"),
				})
			},
			ex: extraction.Extraction{
				Brand: " bosch ",
				Model: "WAA28460",
				Price: "599.00",
			},
			validate: func(t *testing.T, got Asset, created bool, f *fakeStore) {
				if created {
					t.Error("expected created=false")
				}
				if got.ID != "m3" {
					t.Errorf("expected ID='m3', got %q", got.ID)
				}
				if got.Price == nil || *got.Price != "599.00" {
					t.Errorf("Price: got %v, want '599.00'", got.Price)
				}
				if got.Brand == nil || *got.Brand != "bosch" {
					t.Errorf("Brand: got %v, want 'bosch'", got.Brand)
				}
				if got.Model == nil || *got.Model != "WAA28460" {
					t.Errorf("Model: got %v, want 'WAA28460'", got.Model)
				}

				names := updateFieldNames(f.lastUpdate)
				want := []string{"Brand", "Model", "Price"}
				sort.Strings(names)
				if len(names) != len(want) {
					t.Errorf("lastUpdate non-nil fields: got %v, want %v", names, want)
				} else {
					for i := range names {
						if names[i] != want[i] {
							t.Errorf("lastUpdate non-nil fields: got %v, want %v", names, want)
							break
						}
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeStore()
			tc.setup(t, f)

			ctx := context.Background()
			got, created, err := Resolve(ctx, f, tc.ex)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.validate(t, got, created, f)
		})
	}
}

// TestResolveSerialConflictConvergesToMerge ensures that when the serial
// lookup misses (race window), the conflict-safe insert converges to the
// existing row and merges into it instead of duplicating.
func TestResolveSerialConflictConvergesToMerge(t *testing.T) {
	t.Parallel()

	f := newFakeStore()
	f.seed(t, Asset{
		ID:           "c1",
		Brand:        strPtr("Xerox"),
		Model:        strPtr("X1"),
		SerialNumber: strPtr("SNX"),
		Price:        strPtr("5.00"),
	})
	f.skipSerialLookup = true

	ctx := context.Background()
	ex := extraction.Extraction{
		SerialNumber: "snx",
		Price:        "7.00",
	}

	got, created, err := Resolve(ctx, f, ex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false")
	}
	if got.ID != "c1" {
		t.Errorf("expected ID='c1', got %q", got.ID)
	}
	if got.Price == nil || *got.Price != "7.00" {
		t.Errorf("Price: got %v, want '7.00'", got.Price)
	}
	if got.Brand == nil || *got.Brand != "Xerox" {
		t.Errorf("Brand: got %v, want 'Xerox'", got.Brand)
	}
	if len(f.conflictCreates) != 1 {
		t.Errorf("expected 1 conflict create, got %d", len(f.conflictCreates))
	}

	snxCount := 0
	for _, row := range f.rows {
		if row.ns == "SNX" {
			snxCount++
		}
	}
	if snxCount != 1 {
		t.Errorf("expected exactly 1 row with normalized serial 'SNX', got %d", snxCount)
	}
}

func updateFieldNames(u UpdateFields) []string {
	var names []string
	if u.Brand != nil {
		names = append(names, "Brand")
	}
	if u.Model != nil {
		names = append(names, "Model")
	}
	if u.SerialNumber != nil {
		names = append(names, "SerialNumber")
	}
	if u.PurchaseDate != nil {
		names = append(names, "PurchaseDate")
	}
	if u.Price != nil {
		names = append(names, "Price")
	}
	if u.Currency != nil {
		names = append(names, "Currency")
	}
	if u.WarrantyStart != nil {
		names = append(names, "WarrantyStart")
	}
	if u.WarrantyEnd != nil {
		names = append(names, "WarrantyEnd")
	}
	return names
}

func timePtr(t time.Time) *time.Time {
	return &t
}
