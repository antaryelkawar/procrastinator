package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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

// seedAsset inserts a minimal asset row for search testing.
func seedAsset(t *testing.T, pool *pgxpool.Pool, id, ownerID, brand, model, serial string) {
	t.Helper()
	ctx := context.Background()
	query := `INSERT INTO assets (id, owner_id, doc_type, brand, model, serial_number, created_at, updated_at)
	          VALUES ($1, $2, 'invoice', $3, $4, $5, now(), now())`
	if _, err := pool.Exec(ctx, query, id, ownerID, brand, model, serial); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
}

func TestQuickSearch_Hits(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")
	seedAsset(t, e.pool, "asset-qs-1", "alice", "Samsung", "WF80A", "SN-100")

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
		if r.Type == "asset" && r.Id == "asset-qs-1" {
			found = true
			if !strings.Contains(r.Title, "Samsung") {
				t.Errorf("title = %q, want contains \"Samsung\"", r.Title)
			}
		}
	}
	if !found {
		t.Errorf("results = %v, want asset-qs-1", resp.Results)
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
	seedAsset(t, e.pool, "asset-sp-1", "alice", "Samsung", "WF80A", "SN-200")

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
