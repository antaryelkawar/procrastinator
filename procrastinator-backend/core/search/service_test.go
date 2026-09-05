package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Compile-time guard: fakeBackend must satisfy repo.SearchBackend.
var _ repo.SearchBackend = (*fakeBackend)(nil)

// fakeBackend is an in-memory implementation of repo.SearchBackend. It
// records whether it was called and the last pattern it received, and
// returns pre-configured slices per method (or an error if err is set).
type fakeBackend struct {
	called    bool
	pattern   string
	assets    []entity.Asset
	accounts  []entity.FinancialAccount
	movements []entity.MoneyMovement
	documents []entity.Document
	batches   []entity.ImportBatch
	err       error
}

func (b *fakeBackend) mark(pattern string) {
	b.called = true
	b.pattern = pattern
}

func (b *fakeBackend) SearchAssets(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.Asset, error) {
	b.mark(pattern)
	if b.err != nil {
		return nil, b.err
	}
	return b.assets, nil
}

func (b *fakeBackend) SearchAccounts(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.FinancialAccount, error) {
	b.mark(pattern)
	if b.err != nil {
		return nil, b.err
	}
	return b.accounts, nil
}

func (b *fakeBackend) SearchMovements(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	b.mark(pattern)
	if b.err != nil {
		return nil, b.err
	}
	return b.movements, nil
}

func (b *fakeBackend) SearchDocuments(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.Document, error) {
	b.mark(pattern)
	if b.err != nil {
		return nil, b.err
	}
	return b.documents, nil
}

func (b *fakeBackend) SearchImportBatches(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.ImportBatch, error) {
	b.mark(pattern)
	if b.err != nil {
		return nil, b.err
	}
	return b.batches, nil
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// TestBuildPattern
// ---------------------------------------------------------------------------

func TestBuildPattern(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", "%hello%"},
		{"percent", "100%", "%100\\%%"},
		{"underscore", "a_b", "%a\\_b%"},
		{"backslash", `\`, `%\\%`},
		{"all", `50%_off\`, "%50\\%\\_off\\\\%"},
		{"empty", "", "%%"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildPattern(c.in)
			if got != c.want {
				t.Errorf("buildPattern(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestQuick_BlankQuery
// ---------------------------------------------------------------------------

func TestQuick_BlankQuery(t *testing.T) {
	cases := []string{"", "   ", "\t"}
	for i, q := range cases {
		t.Run(strings.TrimSpace(q)+"/case"+itoa(i), func(t *testing.T) {
			b := &fakeBackend{}
			s := New(b)
			got, err := s.Quick(context.Background(), q, 10)
			if err != nil {
				t.Fatalf("Quick() error = %v, want nil", err)
			}
			if got == nil {
				t.Error("Quick() returned nil slice, want non-nil empty []Hit{}")
			}
			if len(got) != 0 {
				t.Errorf("Quick() len = %d, want 0", len(got))
			}
			if b.called {
				t.Error("backend was called for blank query; expected no call")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestQuick_QueryTooLong
// ---------------------------------------------------------------------------

func TestQuick_QueryTooLong(t *testing.T) {
	b := &fakeBackend{}
	s := New(b)
	q := strings.Repeat("a", 201)
	_, err := s.Quick(context.Background(), q, 10)
	if !errors.Is(err, ErrQueryTooLong) {
		t.Errorf("Quick() error = %v, want ErrQueryTooLong", err)
	}
	if b.called {
		t.Error("backend was called for too-long query; expected no call")
	}
}

// ---------------------------------------------------------------------------
// TestQuick_BoundaryLength
// ---------------------------------------------------------------------------

func TestQuick_BoundaryLength(t *testing.T) {
	b := &fakeBackend{}
	s := New(b)
	q := strings.Repeat("a", 200)
	got, err := s.Quick(context.Background(), q, 10)
	if err != nil {
		t.Fatalf("Quick() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("Quick() len = %d, want 0 (empty backend)", len(got))
	}
	if !b.called {
		t.Error("expected backend to be called for 200-char query")
	}
}

// ---------------------------------------------------------------------------
// TestQuick_LimitValidation
// ---------------------------------------------------------------------------

func TestQuick_LimitValidation(t *testing.T) {
	cases := []struct {
		name    string
		limit   int
		wantErr error
	}{
		{"limit=0", 0, ErrInvalidLimit},
		{"limit=51", 51, ErrInvalidLimit},
		{"limit=1", 1, nil},
		{"limit=50", 50, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBackend{}
			s := New(b)
			_, err := s.Quick(context.Background(), "hello", c.limit)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Errorf("Quick() error = %v, want %v", err, c.wantErr)
				}
			} else if err != nil {
				t.Errorf("Quick() error = %v, want nil", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestQuick_RespectsLimit
// ---------------------------------------------------------------------------

func TestQuick_RespectsLimit(t *testing.T) {
	b := &fakeBackend{
		assets: make([]entity.Asset, 10),
	}
	for i := range b.assets {
		b.assets[i] = entity.Asset{ID: "asset-" + itoa(i)}
	}
	s := New(b)
	got, err := s.Quick(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("Quick() error = %v, want nil", err)
	}
	if len(got) != 5 {
		t.Fatalf("Quick() len = %d, want 5", len(got))
	}
	for i, hit := range got {
		wantID := "asset-" + itoa(i)
		if hit.ID != wantID {
			t.Errorf("Quick()[%d].ID = %q, want %q", i, hit.ID, wantID)
		}
	}
}

// ---------------------------------------------------------------------------
// TestPaged_Validation
// ---------------------------------------------------------------------------

func TestPaged_Validation(t *testing.T) {
	cases := []struct {
		name     string
		page     int
		pageSize int
		wantErr  error
	}{
		{"page=0", 0, 10, ErrInvalidPage},
		{"page=-1", -1, 10, ErrInvalidPage},
		{"pageSize=0", 1, 0, ErrInvalidPageSize},
		{"pageSize=101", 1, 101, ErrInvalidPageSize},
		{"page=1,pageSize=1", 1, 1, nil},
		{"page=1,pageSize=100", 1, 100, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBackend{}
			s := New(b)
			_, err := s.Paged(context.Background(), "hello", c.page, c.pageSize)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Errorf("Paged() error = %v, want %v", err, c.wantErr)
				}
			} else if err != nil {
				t.Errorf("Paged() error = %v, want nil", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPaged_BlankQuery
// ---------------------------------------------------------------------------

func TestPaged_BlankQuery(t *testing.T) {
	b := &fakeBackend{}
	s := New(b)
	got, err := s.Paged(context.Background(), "   ", 1, 20)
	if err != nil {
		t.Fatalf("Paged() error = %v, want nil", err)
	}
	want := Page{Results: []Hit{}, Page: 1, PageSize: 20, Total: 0}
	if got.Page != want.Page || got.PageSize != want.PageSize || got.Total != want.Total {
		t.Errorf("Paged() = %+v, want %+v", got, want)
	}
	if got.Results == nil {
		t.Error("Paged().Results is nil, want non-nil empty slice")
	}
	if len(got.Results) != 0 {
		t.Errorf("Paged().Results len = %d, want 0", len(got.Results))
	}
	if b.called {
		t.Error("backend was called for blank query; expected no call")
	}
}

// ---------------------------------------------------------------------------
// TestPaged_BeyondLastPage
// ---------------------------------------------------------------------------

func TestPaged_BeyondLastPage(t *testing.T) {
	b := &fakeBackend{
		assets: make([]entity.Asset, 5),
	}
	for i := range b.assets {
		b.assets[i] = entity.Asset{ID: "asset-" + itoa(i)}
	}
	s := New(b)
	got, err := s.Paged(context.Background(), "query", 99, 20)
	if err != nil {
		t.Fatalf("Paged() error = %v, want nil", err)
	}
	if got.Total != 5 {
		t.Errorf("Paged().Total = %d, want 5", got.Total)
	}
	if got.Results == nil {
		t.Error("Paged().Results is nil, want non-nil empty slice")
	}
	if len(got.Results) != 0 {
		t.Errorf("Paged().Results len = %d, want 0", len(got.Results))
	}
}

// ---------------------------------------------------------------------------
// TestPaged_ReturnsCorrectPage
// ---------------------------------------------------------------------------

func TestPaged_ReturnsCorrectPage(t *testing.T) {
	b := &fakeBackend{
		assets:    make([]entity.Asset, 25),
	}
	for i := range b.assets {
		b.assets[i] = entity.Asset{ID: "asset-" + itoa(i)}
	}
	s := New(b)

	// Page 1: 20 results.
	p1, err := s.Paged(context.Background(), "query", 1, 20)
	if err != nil {
		t.Fatalf("Paged(1) error = %v, want nil", err)
	}
	if p1.Total != 25 {
		t.Errorf("Paged(1).Total = %d, want 25", p1.Total)
	}
	if len(p1.Results) != 20 {
		t.Errorf("Paged(1).Results len = %d, want 20", len(p1.Results))
	}

	// Page 2: 5 results.
	p2, err := s.Paged(context.Background(), "query", 2, 20)
	if err != nil {
		t.Fatalf("Paged(2) error = %v, want nil", err)
	}
	if p2.Total != 25 {
		t.Errorf("Paged(2).Total = %d, want 25", p2.Total)
	}
	if len(p2.Results) != 5 {
		t.Errorf("Paged(2).Results len = %d, want 5", len(p2.Results))
	}
	// The first result of page 2 should be asset-20 (the 21st overall).
	if p2.Results[0].ID != "asset-20" {
		t.Errorf("Paged(2).Results[0].ID = %q, want %q", p2.Results[0].ID, "asset-20")
	}
}

// ---------------------------------------------------------------------------
// TestCombinedOrder_TypePriority
// ---------------------------------------------------------------------------

func TestCombinedOrder_TypePriority(t *testing.T) {
	b := &fakeBackend{
		assets: []entity.Asset{
			{ID: "asset-0"},
			{ID: "asset-1"},
		},
		accounts: []entity.FinancialAccount{
			{ID: "account-0"},
			{ID: "account-1"},
		},
		movements: []entity.MoneyMovement{
			{ID: "movement-0"},
		},
		documents: []entity.Document{
			{ID: "document-0"},
		},
		batches: []entity.ImportBatch{
			{ID: "batch-0"},
		},
	}
	s := New(b)
	got, err := s.Quick(context.Background(), "query", 50)
	if err != nil {
		t.Fatalf("Quick() error = %v, want nil", err)
	}
	wantIDs := []string{
		"asset-0", "asset-1",
		"account-0", "account-1",
		"movement-0",
		"document-0",
		"batch-0",
	}
	if len(got) != len(wantIDs) {
		t.Fatalf("Quick() len = %d, want %d", len(got), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if got[i].ID != wantID {
			t.Errorf("Quick()[%d].ID = %q, want %q", i, got[i].ID, wantID)
		}
	}
}

// ---------------------------------------------------------------------------
// TestQuick_PrefixOfPaged
// ---------------------------------------------------------------------------

func TestQuick_PrefixOfPaged(t *testing.T) {
	b := &fakeBackend{
		assets: make([]entity.Asset, 10),
	}
	for i := range b.assets {
		b.assets[i] = entity.Asset{ID: "asset-" + itoa(i)}
	}
	s := New(b)

	q, err := s.Quick(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("Quick() error = %v, want nil", err)
	}
	p, err := s.Paged(context.Background(), "query", 1, 5)
	if err != nil {
		t.Fatalf("Paged() error = %v, want nil", err)
	}
	if len(q) != len(p.Results) {
		t.Fatalf("len(Quick)=%d, len(Paged.Results)=%d, want equal", len(q), len(p.Results))
	}
	for i := range q {
		if q[i] != p.Results[i] {
			t.Errorf("Quick()[%d] = %+v, Paged().Results[%d] = %+v", i, q[i], i, p.Results[i])
		}
	}
}

// ---------------------------------------------------------------------------
// TestHitShape_Asset
// ---------------------------------------------------------------------------

func TestHitShape_Asset(t *testing.T) {
	t.Run("brand_model_serial_confidence", func(t *testing.T) {
		a := entity.Asset{
			ID:           "a1",
			Brand:        ptr("LG"),
			Model:        ptr("WM-2000"),
			SerialNumber: ptr("SN-123"),
			Confidence:   ptr(0.82),
		}
		h := assetHit(a)
		if h.Type != "asset" {
			t.Errorf("Type = %q, want %q", h.Type, "asset")
		}
		if h.ID != "a1" {
			t.Errorf("ID = %q, want %q", h.ID, "a1")
		}
		if h.Title != "LG WM-2000" {
			t.Errorf("Title = %q, want %q", h.Title, "LG WM-2000")
		}
		if h.Subtitle == nil || *h.Subtitle != "SN-123" {
			t.Errorf("Subtitle = %v, want ptr(SN-123)", h.Subtitle)
		}
		if h.Confidence == nil || *h.Confidence != 0.82 {
			t.Errorf("Confidence = %v, want ptr(0.82)", h.Confidence)
		}
	})

	t.Run("model_only_no_confidence", func(t *testing.T) {
		a := entity.Asset{
			ID:    "a2",
			Model: ptr("M-100"),
		}
		h := assetHit(a)
		if h.Title != "M-100" {
			t.Errorf("Title = %q, want %q", h.Title, "M-100")
		}
		if h.Subtitle != nil {
			t.Errorf("Subtitle = %v, want nil", h.Subtitle)
		}
		if h.Confidence != nil {
			t.Errorf("Confidence = %v, want nil", h.Confidence)
		}
	})

	t.Run("serial_only", func(t *testing.T) {
		a := entity.Asset{
			ID:           "a3",
			SerialNumber: ptr("SN-999"),
		}
		h := assetHit(a)
		if h.Title != "SN-999" {
			t.Errorf("Title = %q, want %q", h.Title, "SN-999")
		}
		if h.Subtitle == nil || *h.Subtitle != "SN-999" {
			t.Errorf("Subtitle = %v, want ptr(SN-999)", h.Subtitle)
		}
		if h.Confidence != nil {
			t.Errorf("Confidence = %v, want nil", h.Confidence)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHitShape_Account
// ---------------------------------------------------------------------------

func TestHitShape_Account(t *testing.T) {
	t.Run("name_type_institution", func(t *testing.T) {
		a := entity.FinancialAccount{
			ID:          "acc1",
			Name:        "Main Bank",
			Type:        "bank",
			Institution: ptr("Chase"),
		}
		h := accountHit(a)
		if h.Type != "account" {
			t.Errorf("Type = %q, want %q", h.Type, "account")
		}
		if h.Title != "Main Bank" {
			t.Errorf("Title = %q, want %q", h.Title, "Main Bank")
		}
		if h.Subtitle == nil || *h.Subtitle != "bank · Chase" {
			t.Errorf("Subtitle = %v, want ptr(bank · Chase)", h.Subtitle)
		}
		if h.Confidence != nil {
			t.Errorf("Confidence = %v, want nil", h.Confidence)
		}
	})

	t.Run("name_type_no_institution", func(t *testing.T) {
		a := entity.FinancialAccount{
			ID:   "acc2",
			Name: "Cash",
			Type: "cash",
		}
		h := accountHit(a)
		if h.Title != "Cash" {
			t.Errorf("Title = %q, want %q", h.Title, "Cash")
		}
		if h.Subtitle == nil || *h.Subtitle != "cash" {
			t.Errorf("Subtitle = %v, want ptr(cash)", h.Subtitle)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHitShape_Movement
// ---------------------------------------------------------------------------

func TestHitShape_Movement(t *testing.T) {
	t.Run("description_external_ref", func(t *testing.T) {
		m := entity.MoneyMovement{
			ID:                "mv1",
			Description:       "coffee beans",
			ExternalReference: ptr("TXN-90210"),
		}
		h := movementHit(m)
		if h.Type != "movement" {
			t.Errorf("Type = %q, want %q", h.Type, "movement")
		}
		if h.Title != "coffee beans" {
			t.Errorf("Title = %q, want %q", h.Title, "coffee beans")
		}
		if h.Subtitle == nil || *h.Subtitle != "TXN-90210" {
			t.Errorf("Subtitle = %v, want ptr(TXN-90210)", h.Subtitle)
		}
		if h.Confidence != nil {
			t.Errorf("Confidence = %v, want nil", h.Confidence)
		}
	})

	t.Run("description_no_ref", func(t *testing.T) {
		m := entity.MoneyMovement{
			ID:          "mv2",
			Description: "misc",
		}
		h := movementHit(m)
		if h.Title != "misc" {
			t.Errorf("Title = %q, want %q", h.Title, "misc")
		}
		if h.Subtitle != nil {
			t.Errorf("Subtitle = %v, want nil", h.Subtitle)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHitShape_Document
// ---------------------------------------------------------------------------

func TestHitShape_Document(t *testing.T) {
	t.Run("doctype_confidence", func(t *testing.T) {
		d := entity.Document{
			ID:         "doc1",
			DocType:    "invoice",
			Confidence: ptr(0.5),
		}
		h := documentHit(d)
		if h.Type != "document" {
			t.Errorf("Type = %q, want %q", h.Type, "document")
		}
		if h.Title != "invoice" {
			t.Errorf("Title = %q, want %q", h.Title, "invoice")
		}
		if h.Subtitle != nil {
			t.Errorf("Subtitle = %v, want nil", h.Subtitle)
		}
		if h.Confidence == nil || *h.Confidence != 0.5 {
			t.Errorf("Confidence = %v, want ptr(0.5)", h.Confidence)
		}
	})

	t.Run("doctype_no_confidence", func(t *testing.T) {
		d := entity.Document{
			ID:      "doc2",
			DocType: "warranty",
		}
		h := documentHit(d)
		if h.Title != "warranty" {
			t.Errorf("Title = %q, want %q", h.Title, "warranty")
		}
		if h.Confidence != nil {
			t.Errorf("Confidence = %v, want nil", h.Confidence)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHitShape_ImportBatch
// ---------------------------------------------------------------------------

func TestHitShape_ImportBatch(t *testing.T) {
	b := entity.ImportBatch{
		ID:       "b1",
		Filename: "statement.csv",
	}
	h := importBatchHit(b)
	if h.Type != "import_batch" {
		t.Errorf("Type = %q, want %q", h.Type, "import_batch")
	}
	if h.Title != "statement.csv" {
		t.Errorf("Title = %q, want %q", h.Title, "statement.csv")
	}
	if h.Subtitle != nil {
		t.Errorf("Subtitle = %v, want nil", h.Subtitle)
	}
	if h.Confidence != nil {
		t.Errorf("Confidence = %v, want nil", h.Confidence)
	}
}

// ---------------------------------------------------------------------------
// TestNoMatch_EmptyResults
// ---------------------------------------------------------------------------

func TestNoMatch_EmptyResults(t *testing.T) {
	b := &fakeBackend{}
	s := New(b)

	q, err := s.Quick(context.Background(), "nomatch", 10)
	if err != nil {
		t.Fatalf("Quick() error = %v, want nil", err)
	}
	if q == nil {
		t.Error("Quick() returned nil slice, want non-nil empty []Hit{}")
	}
	if len(q) != 0 {
		t.Errorf("Quick() len = %d, want 0", len(q))
	}

	p, err := s.Paged(context.Background(), "nomatch", 1, 20)
	if err != nil {
		t.Fatalf("Paged() error = %v, want nil", err)
	}
	if p.Results == nil {
		t.Error("Paged().Results is nil, want non-nil empty slice")
	}
	if p.Total != 0 {
		t.Errorf("Paged().Total = %d, want 0", p.Total)
	}
}

// itoa is a tiny int-to-string helper for test data.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
