package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedUser inserts a user into the registry (idempotent).
func seedUser(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO Users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, id); err != nil {
		t.Fatalf("seed user %q: %v", id, err)
	}
}

// seedAsset inserts a minimal asset row for search testing and returns its
// generated id (assets.id is a uuid with a gen_random_uuid() default). The
// insert runs in a transaction bound to ownerID (app.user_id) so the RLS
// backstop admits it.
func seedAsset(t *testing.T, pool *pgxpool.Pool, ownerID, brand, model, serial string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("seed asset begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, ownerID); err != nil {
		t.Fatalf("seed asset set user: %v", err)
	}
	query := `INSERT INTO assets (owner_id, brand, model, serial_number, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, now(), now()) RETURNING id`
	var id string
	if err := tx.QueryRow(ctx, query, ownerID, brand, model, serial).Scan(&id); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("seed asset commit: %v", err)
	}
	return id
}

// seedAssetFull inserts an asset with category and warranty_end for filter
// testing and returns its generated id (assets.id is a uuid). The insert runs
// in a transaction bound to ownerID (app.user_id) so the RLS backstop admits it.
func seedAssetFull(t *testing.T, pool *pgxpool.Pool, ownerID, brand, model, serial, category string, warrantyEnd *time.Time) string {
	t.Helper()
	ctx := context.Background()
	var we any
	if warrantyEnd != nil {
		we = *warrantyEnd
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("seed asset full begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, ownerID); err != nil {
		t.Fatalf("seed asset full set user: %v", err)
	}
	query := `INSERT INTO assets (owner_id, brand, model, serial_number, asset_category, warranty_end, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6, now(), now()) RETURNING id`
	var id string
	if err := tx.QueryRow(ctx, query, ownerID, brand, model, serial, category, we).Scan(&id); err != nil {
		t.Fatalf("seed asset full: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("seed asset full commit: %v", err)
	}
	return id
}

func TestQuickSearch_Hits(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")
	assetID := seedAsset(t, e.pool, "alice", "Samsung", "WF80A", "SN-100")

	rec := do(t, e.handler, "GET", "/api/users/alice/search/quick?q=samsung", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Type    string `json:"type"`
			Id      string `json:"id"`
			Title   string `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	found := false
	for _, r := range resp.Results {
		if r.Type == "asset" && r.Id == assetID {
			found = true
			if !strings.Contains(r.Title, "Samsung") {
				t.Errorf("title = %q, want contains \"Samsung\"", r.Title)
			}
		}
	}
	if !found {
		t.Errorf("results = %v, want asset %s", resp.Results, assetID)
	}
}

func TestQuickSearch_BlankQ(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/search/quick", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("results = %d items, want 0", len(resp.Results))
	}
}

func TestQuickSearch_InvalidLimit(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/search/quick?q=x&limit=0", "", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSearch_Paged(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")
	seedAsset(t, e.pool, "alice", "Samsung", "WF80A", "SN-200")

	rec := do(t, e.handler, "GET", "/api/users/alice/search?q=samsung", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results  []json.RawMessage `json:"results"`
		Page     int               `json:"page"`
		PageSize int               `json:"page_size"`
		Total    int               `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total < 1 {
		t.Errorf("total = %d, want >= 1", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}
	if resp.PageSize != 20 {
		t.Errorf("page_size = %d, want 20", resp.PageSize)
	}
	if len(resp.Results) < 1 {
		t.Errorf("results = %d items, want >= 1", len(resp.Results))
	}
}

func TestSearch_BlankQ(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/search", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}

func TestSearch_InvalidPageSize(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/search?q=x&page_size=0", "", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSearch_InvalidPage(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/search?q=x&page=0", "", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSearch_QueryTooLong(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	longQ := strings.Repeat("a", 201)
	rec := do(t, e.handler, "GET", "/api/users/alice/search?q="+longQ, "", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSearch_UnregisteredUser(t *testing.T) {
	e := newEnv(t, envOpts{})

	rec := do(t, e.handler, "GET", "/api/users/ghost/search/quick?q=x", "", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestSearch_FilterCategory verifies that q=microwave&category=appliance
// returns only assets in the appliance category that match the query.
func TestSearch_FilterCategory(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")
	applianceID := seedAssetFull(t, e.pool, "alice", "Samsung", "Microwave", "SN-FC1", "appliance", nil)
	electronicsID := seedAssetFull(t, e.pool, "alice", "Bosch", "Microwave", "SN-FC2", "electronics", nil)

	rec := do(t, e.handler, "GET", "/api/users/alice/search?q=microwave&category=appliance", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Id string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ids := make(map[string]bool)
	for _, r := range resp.Results {
		ids[r.Id] = true
	}
	if !ids[applianceID] {
		t.Errorf("expected appliance asset %s in results, got: %v", applianceID, ids)
	}
	if ids[electronicsID] {
		t.Errorf("electronics asset %s should not appear when category=appliance, got: %v", electronicsID, ids)
	}
}

// TestSearch_FilterWarrantyStatus verifies that warranty_status=expiring_within:90
// returns only assets whose warranty_end is within 90 days from now.
func TestSearch_FilterWarrantyStatus(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")
	in30 := time.Now().AddDate(0, 0, 30)
	in200 := time.Now().AddDate(0, 0, 200)
	past30 := time.Now().AddDate(0, 0, -30)
	in30ID := seedAssetFull(t, e.pool, "alice", "TestBrand", "M1", "SN-FW1", "", &in30)
	in200ID := seedAssetFull(t, e.pool, "alice", "TestBrand", "M2", "SN-FW2", "", &in200)
	past30ID := seedAssetFull(t, e.pool, "alice", "TestBrand", "M3", "SN-FW3", "", &past30)

	rec := do(t, e.handler, "GET", "/api/users/alice/search?q=TestBrand&warranty_status=expiring_within:90", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Id string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ids := make(map[string]bool)
	for _, r := range resp.Results {
		ids[r.Id] = true
	}
	if !ids[in30ID] {
		t.Errorf("expected asset %s (warranty in 30d) in results, got: %v", in30ID, ids)
	}
	if ids[in200ID] {
		t.Errorf("asset %s (warranty in 200d) should not appear for expiring_within:90, got: %v", in200ID, ids)
	}
	if ids[past30ID] {
		t.Errorf("asset %s (warranty expired) should not appear for expiring_within:90, got: %v", past30ID, ids)
	}
}

// TestSearch_FilterCombined verifies that q, category, and brand filters are
// ANDed together: all three must match for a result to appear.
func TestSearch_FilterCombined(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")
	// match-all: Samsung + appliance → matches all 3
	matchAllID := seedAssetFull(t, e.pool, "alice", "Samsung", "Microwave", "SN-FA", "appliance", nil)
	// fail-category: Samsung + furniture → fails category=appliance
	failCatID := seedAssetFull(t, e.pool, "alice", "Samsung", "Microwave", "SN-FB", "furniture", nil)
	// fail-brand: LG + appliance → fails brand=samsung and q=samsung
	failBrandID := seedAssetFull(t, e.pool, "alice", "LG", "Microwave", "SN-FC", "appliance", nil)

	rec := do(t, e.handler, "GET", "/api/users/alice/search?q=samsung&category=appliance&brand=samsung", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Id string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ids := make(map[string]bool)
	for _, r := range resp.Results {
		ids[r.Id] = true
	}
	if !ids[matchAllID] {
		t.Errorf("expected match-all asset %s in results, got: %v", matchAllID, ids)
	}
	if ids[failCatID] {
		t.Errorf("furniture asset %s should not appear when category=appliance, got: %v", failCatID, ids)
	}
	if ids[failBrandID] {
		t.Errorf("LG asset %s should not appear when brand=samsung, got: %v", failBrandID, ids)
	}
}

// TestSearch_FilterKindLabels verifies that every search result carries a
// non-empty kind label (type field).
func TestSearch_FilterKindLabels(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")
	flID := seedAssetFull(t, e.pool, "alice", "Samsung", "Microwave", "SN-FL1", "appliance", nil)

	rec := do(t, e.handler, "GET", "/api/users/alice/search?q=samsung&category=appliance", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Type string `json:"type"`
			Id   string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	found := false
	for _, r := range resp.Results {
		if r.Type == "" {
			t.Errorf("result %s has empty type label", r.Id)
		}
		if r.Type != "asset" && r.Type != "account" && r.Type != "movement" && r.Type != "document" && r.Type != "import_batch" {
			t.Errorf("result %s has unknown type %q", r.Id, r.Type)
		}
		if r.Id == flID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected seeded asset %s in results", flID)
	}
}
