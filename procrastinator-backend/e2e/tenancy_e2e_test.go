// Package e2e contains a whole-system, end-to-end tenancy test. It drives the
// ACTUAL stack — the real api.Server over chi, a real Postgres pool connected
// as the app role (procrastinator, NOBYPASSRLS), and real on-disk file storage
// in a temp dir — and walks the owner-model user journey (household-scoped vs
// personal assets, member visibility, non-member isolation, and the RLS
// backstop on the shared tables).
//
// Precondition: the compose `postgres` service is running and migrations are
// applied to v4 (00004_row_level_security). When the database is unreachable or
// not migrated, the test skips with a reason rather than failing.
//
// The test is self-contained and re-runnable: it provisions a unique
// per-run prefix (e2e-<unix>-<hex>) and three users, then cleans up its rows
// and files afterwards.
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/api"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/household"
	"procrastinator-backend/core/ingest"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/statement"
	"procrastinator-backend/infra/filestorage"
	"procrastinator-backend/infra/pdftext"
	"procrastinator-backend/infra/postgres"
)

// defaultDSN is the app-role DSN for the compose postgres service (role
// procrastinator / password procrastinator, database procrastinator). It can be
// overridden with PROCRASTINATOR_E2E_DATABASE_URL.
const defaultDSN = "postgres://procrastinator:procrastinator@localhost:5432/procrastinator?sslmode=disable"

// fakeExtractor is a deterministic repo.Extractor that returns a fixed, valid
// invoice extraction. The external LLM is the only stubbed collaborator (the
// established pattern in the api package tests); everything else — HTTP, PG,
// file storage — is real.
type fakeExtractor struct{ payload string }

// Extract implements repo.Extractor.
func (e fakeExtractor) Extract(_ context.Context, _ string, _ []byte) (string, error) {
	return e.payload, nil
}

var _ repo.Extractor = fakeExtractor{}

// invoicePayload returns a valid invoice extraction whose serial is unique per
// run so identity resolution creates a distinct personal asset.
func invoicePayload(runID string) string {
	return fmt.Sprintf(
		`{"classification":"invoice","brand":"E2E","model":"Model-%s","serial_number":"E2E-%s-P","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100.00","currency":"INR","metadata":{}}`,
		runID, runID,
	)
}

// buildServer wires the real api.Server (ingest + ledger + statement) over the
// given factory/storage and returns its chi router.
func buildServer(t *testing.T, storageDir, runID string, pool *pgxpool.Pool, factory *repo.Factory) http.Handler {
	t.Helper()
	const maxBytes = int64(20 * 1024 * 1024)

	movRepo := postgres.NewMovementRepository(pool)
	ledgerSvc := ledger.New(factory, movRepo)
	ingestSvc := ingest.New(
		factory,
		fakeExtractor{payload: invoicePayload(runID)},
		filestorage.New(storageDir),
		maxBytes,
	)
	stmtSvc := statement.New(
		factory,
		filestorage.NewStatement(t.TempDir()),
		movRepo,
		postgres.NewDocumentRepository(pool),
		pdftext.New(),
		maxBytes,
		100000,
	)
	srv := api.New(ingestSvc, factory, ledgerSvc, movRepo, maxBytes, stmtSvc, maxBytes, household.New(factory))
	return srv.Routes()
}

// provisionUsers inserts the given user ids into the user registry (explicit
// provisioning — the spec allows users to be created only by explicit
// provisioning; there is no user-registry HTTP endpoint).
func provisionUsers(ctx context.Context, pool *pgxpool.Pool, users ...string) error {
	for _, u := range users {
		if _, err := pool.Exec(ctx, "INSERT INTO Users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING", u); err != nil {
			return fmt.Errorf("insert user %s: %w", u, err)
		}
	}
	return nil
}

// httpGet issues a GET against the handler and returns the status + body.
func httpGet(t *testing.T, h http.Handler, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// httpPostJSON posts a JSON body to the handler and returns the status + body.
func httpPostJSON(t *testing.T, h http.Handler, path string, body any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal JSON body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// httpPostFileForm posts a multipart request with a single "file" part plus
// optional extra form fields (e.g. owner_household_id) and returns the status +
// body. A nil/empty extraFields map posts the file part only.
func httpPostFileForm(t *testing.T, h http.Handler, path, filename string, content []byte, extraFields map[string]string) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename=%q`, filename)},
		"Content-Type":        {"application/pdf"},
	})
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	for k, v := range extraFields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write extra form field %s: %v", k, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// pdfBytes returns a byte slice that filestorage sniffs as application/pdf.
func pdfBytes(seed string) []byte {
	return []byte("%PDF-1.4\n" + seed + "\n%%EOF")
}

// strVal returns the string value of key k in m ("" when absent or not a string).
func strVal(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// idsOf returns the "id" of each asset map, for readable failure messages.
func idsOf(assets []map[string]any) []string {
	out := make([]string, 0, len(assets))
	for _, a := range assets {
		out = append(out, strVal(a, "id"))
	}
	return out
}

// contains reports whether ss contains s.
func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }

// TestTenancyE2E drives the owner-model user journey end to end.
func TestTenancyE2E(t *testing.T) {
	dsn := os.Getenv("PROCRASTINATOR_E2E_DATABASE_URL")
	if dsn == "" {
		dsn = defaultDSN
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("PG unreachable (pgxpool.New): %v", err)
	}
	// Register the pool close FIRST so that, under t.Cleanup's LIFO ordering,
	// the row-cleanup (registered later) runs while the pool is still open.
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PG unreachable (ping): %v", err)
	}

	// The owner model needs migration 00004 (row-level security + households).
	// Skip if the schema is not at v4 rather than failing on a missing object.
	var maxVersion int
	if err := pool.QueryRow(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version`).Scan(&maxVersion); err != nil {
		t.Skipf("check goose_db_version: %v", err)
	}
	if maxVersion != 4 {
		t.Skipf("schema not at v4 (final set); max goose version is %d; run goose migrations to v4 first", maxVersion)
	}

	// Unique per-run prefix (matches ^[A-Za-z0-9_-]{1,64}$).
	var rb [4]byte
	if _, err := rand.Read(rb[:]); err != nil {
		t.Fatalf("read crypto/rand: %v", err)
	}
	runID := fmt.Sprintf("e2e-%d-%s", time.Now().Unix(), hex.EncodeToString(rb[:]))
	u1, u2, u3 := runID+"-u1", runID+"-u2", runID+"-u3"

	// Real server stack + real on-disk storage in a temp dir.
	storageDir := t.TempDir()
	factory := postgres.NewFactory(pool)
	handler := buildServer(t, storageDir, runID, pool, factory)

	if err := provisionUsers(ctx, pool, u1, u2, u3); err != nil {
		t.Fatalf("provision users: %v", err)
	}

	// IDs created in subtests a/b and consumed by c–g.
	var hhID, hhAssetID, personalAssetID, acctID string

	// Cleanup: remove every row this run created. The RLS-owned tables are
	// deleted while the session is bound to u1 (the owner) so RLS admits the
	// delete; the households/household_members rows are u1's to delete as the
	// owner; finally the three user rows are deleted unbound. The temp storage
	// dir is cleaned by t.TempDir().
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()

		tx, err := pool.Begin(cctx)
		if err != nil {
			t.Errorf("cleanup begin tx: %v", err)
			return
		}
		if _, err := tx.Exec(cctx, `SELECT set_config('app.user_id', $1, true)`, u1); err != nil {
			t.Errorf("cleanup set app.user_id: %v", err)
			return
		}
		// Owned rows first (u1 owns every asset/source/document/account this run
		// created, in both scopes). Children before parents for the FKs.
		if _, err := tx.Exec(cctx, "DELETE FROM documents WHERE owner_id = $1", u1); err != nil {
			t.Errorf("cleanup documents: %v", err)
		}
		if _, err := tx.Exec(cctx, "DELETE FROM assets WHERE owner_id = $1", u1); err != nil {
			t.Errorf("cleanup assets: %v", err)
		}
		if _, err := tx.Exec(cctx, "DELETE FROM sources WHERE owner_id = $1", u1); err != nil {
			t.Errorf("cleanup sources: %v", err)
		}
		if _, err := tx.Exec(cctx, "DELETE FROM financial_accounts WHERE owner_id = $1", u1); err != nil {
			t.Errorf("cleanup financial_accounts: %v", err)
		}
		// Then membership rows (u1 owns H1, so it may delete the co-member rows)
		// and the household itself.
		if hhID != "" {
			if _, err := tx.Exec(cctx, "DELETE FROM household_members WHERE household_id = $1", hhID); err != nil {
				t.Errorf("cleanup household_members: %v", err)
			}
		}
		if _, err := tx.Exec(cctx, "DELETE FROM households WHERE owner_id = $1", u1); err != nil {
			t.Errorf("cleanup households: %v", err)
		}
		if err := tx.Commit(cctx); err != nil {
			t.Errorf("cleanup commit: %v", err)
		}

		// Finally the three user rows (users has no RLS).
		for _, u := range []string{u1, u2, u3} {
			if _, err := pool.Exec(cctx, "DELETE FROM users WHERE id = $1", u); err != nil {
				t.Errorf("cleanup user %s: %v", u, err)
			}
		}
	})

	// (a) u1 creates household H1 via the HTTP endpoint; u1 (creator) is a
	// member automatically, u2 is added, and the membership takes effect.
	t.Run("a_household_create_and_member", func(t *testing.T) {
		status, body := httpPostJSON(t, handler, "/api/users/"+u1+"/households", map[string]string{"display_name": "E2E H1"})
		if status != http.StatusCreated {
			t.Fatalf("create household status = %d, want 201 (body: %s)", status, body)
		}
		var hh struct {
			ID          string `json:"id"`
			OwnerID     string `json:"owner_id"`
			DisplayName string `json:"display_name"`
			Members     []struct {
				UserID string `json:"user_id"`
			} `json:"members"`
		}
		if err := json.Unmarshal(body, &hh); err != nil {
			t.Fatalf("unmarshal household: %v (body: %s)", err, body)
		}
		hhID = hh.ID
		if hhID == "" {
			t.Fatal("household id is empty")
		}
		if hh.OwnerID != u1 {
			t.Errorf("household owner_id = %q, want %q (body: %s)", hh.OwnerID, u1, body)
		}
		var memberIDs []string
		for _, m := range hh.Members {
			memberIDs = append(memberIDs, m.UserID)
		}
		if !contains(memberIDs, u1) {
			t.Errorf("creator %s is not in the household's members (members: %v, body: %s)", u1, memberIDs, body)
		}

		// Add u2 as a member.
		status, body = httpPostJSON(t, handler, "/api/users/"+u1+"/households/"+hhID+"/members", map[string]string{"user_id": u2})
		if status != http.StatusNoContent {
			t.Fatalf("add member status = %d, want 204 (body: %s)", status, body)
		}

		// Verify membership took effect: u2's household list includes hhID.
		status, body = httpGet(t, handler, "/api/users/"+u2+"/households")
		if status != http.StatusOK {
			t.Fatalf("u2 GET households status = %d, want 200 (body: %s)", status, body)
		}
		var hhs []map[string]any
		if err := json.Unmarshal(body, &hhs); err != nil {
			t.Fatalf("unmarshal u2 households: %v (body: %s)", err, body)
		}
		var hhIDs []string
		for _, h := range hhs {
			hhIDs = append(hhIDs, strVal(h, "id"))
		}
		if !contains(hhIDs, hhID) {
			t.Errorf("u2's households do not include %s after being added (saw: %v, body: %s)", hhID, hhIDs, body)
		}
	})

	// (b) Household-scoped AND personal uploads, both via the real HTTP upload
	// endpoint (no direct repository calls).
	t.Run("b_household_and_personal_upload", func(t *testing.T) {
		if hhID == "" {
			t.Skipf("skipping: prerequisite (a) did not create the household")
		}
		// Household upload: file part + owner_household_id form field.
		status, body := httpPostFileForm(t, handler, "/api/users/"+u1+"/documents", "hh.pdf", pdfBytes("hh-"+runID), map[string]string{"owner_household_id": hhID})
		if status != http.StatusCreated {
			t.Fatalf("household upload status = %d, want 201 (body: %s)", status, body)
		}
		var hha map[string]any
		if err := json.Unmarshal(body, &hha); err != nil {
			t.Fatalf("unmarshal household asset: %v (body: %s)", err, body)
		}
		hhAssetID = strVal(hha, "id")
		if hhAssetID == "" {
			t.Fatalf("household asset id is empty (body: %s)", body)
		}
		if got := strVal(hha, "owner_household_id"); got != hhID {
			t.Errorf("household asset owner_household_id = %q, want %q (body: %s)", got, hhID, body)
		}

		// Personal upload: file part only, no owner_household_id field.
		status, body = httpPostFileForm(t, handler, "/api/users/"+u1+"/documents", "personal.pdf", pdfBytes("personal-"+runID), nil)
		if status != http.StatusCreated {
			t.Fatalf("personal upload status = %d, want 201 (body: %s)", status, body)
		}
		var pa map[string]any
		if err := json.Unmarshal(body, &pa); err != nil {
			t.Fatalf("unmarshal personal asset: %v (body: %s)", err, body)
		}
		personalAssetID = strVal(pa, "id")
		if personalAssetID == "" {
			t.Fatalf("personal asset id is empty (body: %s)", body)
		}
		if _, present := pa["owner_household_id"]; present {
			t.Errorf("personal asset has owner_household_id, want absent (body: %s)", body)
		}
	})

	// (c) u1 sees BOTH assets, each with the correct owner_household_id shape.
	t.Run("c_u1_sees_both_assets", func(t *testing.T) {
		if hhAssetID == "" || personalAssetID == "" {
			t.Skipf("skipping: prerequisite (b) did not create the assets")
		}
		status, body := httpGet(t, handler, "/api/users/"+u1+"/assets")
		if status != http.StatusOK {
			t.Fatalf("u1 GET assets status = %d, want 200 (body: %s)", status, body)
		}
		var assets []map[string]any
		if err := json.Unmarshal(body, &assets); err != nil {
			t.Fatalf("unmarshal assets: %v (body: %s)", err, body)
		}
		byID := map[string]map[string]any{}
		for _, a := range assets {
			byID[strVal(a, "id")] = a
		}
		a, ok := byID[hhAssetID]
		if !ok {
			t.Errorf("u1 does not see the household asset %s (saw: %v)", hhAssetID, idsOf(assets))
		} else if got := strVal(a, "owner_household_id"); got != hhID {
			t.Errorf("household asset owner_household_id = %q, want %q", got, hhID)
		}
		p, ok := byID[personalAssetID]
		if !ok {
			t.Errorf("u1 does not see the personal asset %s (saw: %v)", personalAssetID, idsOf(assets))
		} else if _, present := p["owner_household_id"]; present {
			t.Errorf("personal asset has owner_household_id, want absent")
		}
	})

	// (d) u2, a member of H1, sees ONLY the household asset (not u1's personal
	// asset) and can read the household asset's documents.
	t.Run("d_u2_member_sees_only_household", func(t *testing.T) {
		if hhAssetID == "" || personalAssetID == "" {
			t.Skipf("skipping: prerequisite (b) did not create the assets")
		}
		status, body := httpGet(t, handler, "/api/users/"+u2+"/assets")
		if status != http.StatusOK {
			t.Fatalf("u2 GET assets status = %d, want 200 (body: %s)", status, body)
		}
		var assets []map[string]any
		if err := json.Unmarshal(body, &assets); err != nil {
			t.Fatalf("unmarshal assets: %v (body: %s)", err, body)
		}
		byID := map[string]map[string]any{}
		for _, a := range assets {
			byID[strVal(a, "id")] = a
		}
		if _, ok := byID[hhAssetID]; !ok {
			t.Errorf("u2 (a member of H1) does NOT see the household asset %s (saw: %v)", hhAssetID, idsOf(assets))
		}
		if _, ok := byID[personalAssetID]; ok {
			t.Errorf("u2 sees u1's personal asset %s, which must stay hidden (saw: %v)", personalAssetID, idsOf(assets))
		}
		if len(assets) != 1 {
			t.Errorf("u2 sees %d asset(s), want exactly 1 (the household asset); saw %v", len(assets), idsOf(assets))
		}

		// A member should be able to read the household asset's documents.
		dStatus, dBody := httpGet(t, handler, "/api/users/"+u2+"/assets/"+hhAssetID+"/documents")
		if dStatus != http.StatusOK {
			t.Errorf("u2 GET household asset documents status = %d, want 200 (a member should access; body: %s)", dStatus, dBody)
		} else {
			var docs []map[string]any
			if err := json.Unmarshal(dBody, &docs); err != nil {
				t.Fatalf("unmarshal documents: %v (body: %s)", err, dBody)
			}
			if len(docs) < 1 {
				t.Errorf("u2 sees %d documents on the household asset, want >=1 (body: %s)", len(docs), dBody)
			}
		}
	})

	// (e) u3 (no household) sees an empty list and cannot read either of u1's
	// assets (404).
	t.Run("e_u3_nonmember_sees_nothing", func(t *testing.T) {
		if hhAssetID == "" || personalAssetID == "" {
			t.Skipf("skipping: prerequisite (b) did not create the assets")
		}
		status, body := httpGet(t, handler, "/api/users/"+u3+"/assets")
		if status != http.StatusOK {
			t.Fatalf("u3 GET assets status = %d, want 200 (body: %s)", status, body)
		}
		if trimmed := strings.TrimSpace(string(body)); trimmed != "[]" {
			t.Errorf("u3 assets = %q, want \"[]\" (empty list, not an error)", trimmed)
		}
		if s, b := httpGet(t, handler, "/api/users/"+u3+"/assets/"+hhAssetID); s != http.StatusNotFound {
			t.Errorf("u3 GET household asset detail = %d, want 404 (body: %s)", s, b)
		}
		if s, b := httpGet(t, handler, "/api/users/"+u3+"/assets/"+personalAssetID); s != http.StatusNotFound {
			t.Errorf("u3 GET personal asset detail = %d, want 404 (body: %s)", s, b)
		}
	})

	// (f) Cross-user file isolation: u1's uploaded bytes live under the u1/
	// prefix and there is no copy under u2/. The API exposes no file-download
	// endpoint, so cross-user fetch is impossible at the HTTP layer; this asserts
	// the on-disk per-user layout instead (spec: "{userId}/{fileID}").
	t.Run("f_cross_user_file_isolation", func(t *testing.T) {
		u1Files, err := filepath.Glob(filepath.Join(storageDir, u1, "*.pdf"))
		if err != nil {
			t.Fatalf("glob u1 dir: %v", err)
		}
		if len(u1Files) == 0 {
			t.Errorf("no .pdf files under %s, want >=1 (u1's uploads)", filepath.Join(storageDir, u1))
		}
		u2Files, err := filepath.Glob(filepath.Join(storageDir, u2, "*"))
		if err != nil {
			t.Fatalf("glob u2 dir: %v", err)
		}
		if len(u2Files) != 0 {
			t.Errorf("files found under %s — u2 must not hold any of u1's uploads: %v", filepath.Join(storageDir, u2), u2Files)
		}
	})

	// (g) RLS backstop: an unbound app-role session (NOBYPASSRLS) must see NONE
	// of the rows even after they exist; binding the session to a user reveals
	// exactly the rows that user may see under the owner-model visibility rule.
	t.Run("g_rls_backstop", func(t *testing.T) {
		if hhAssetID == "" || personalAssetID == "" {
			t.Skipf("skipping: prerequisite (b) did not create the assets")
		}

		// 1) Create a PERSONAL finance account for u1 over HTTP so the
		// financial_accounts table has an owner-model row to check.
		status, body := httpPostJSON(t, handler, "/api/users/"+u1+"/finance/accounts", map[string]string{
			"name": "E2E acct", "type": "bank", "currency": "USD",
		})
		if status != http.StatusCreated {
			t.Fatalf("create finance account status = %d, want 201 (body: %s)", status, body)
		}
		var acc map[string]any
		if err := json.Unmarshal(body, &acc); err != nil {
			t.Fatalf("unmarshal account: %v (body: %s)", err, body)
		}
		acctID = strVal(acc, "id")
		if acctID == "" {
			t.Fatalf("finance account id is empty (body: %s)", body)
		}

		// 2) Open a fresh direct connection as the app role (NOBYPASSRLS).
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatalf("connect as app role for RLS check: %v", err)
		}
		defer conn.Close(ctx)

		// 3) UNBOUND session: sees nothing across all three RLS tables.
		for _, table := range []string{"assets", "documents", "financial_accounts"} {
			var n int
			if err := conn.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
				t.Fatalf("unbound count %s: %v", table, err)
			}
			if n != 0 {
				t.Errorf("unbound app role sees %d row(s) in %s, want 0 (RLS must strip all rows)", n, table)
			}
		}

		// 4) BOUND to u2: sees the household asset but not u1's personal asset
		// or personal finance account.
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatalf("begin u2-bound RLS check: %v", err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, u2); err != nil {
			t.Fatalf("set_config app.user_id to u2: %v", err)
		}
		var u2HH, u2Personal, u2Acct int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE id = $1`, hhAssetID).Scan(&u2HH); err != nil {
			t.Fatalf("u2 count household asset: %v", err)
		}
		if u2HH != 1 {
			t.Errorf("u2 sees %d household asset row(s) by id %s, want 1 (a member sees the household asset)", u2HH, hhAssetID)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE id = $1`, personalAssetID).Scan(&u2Personal); err != nil {
			t.Fatalf("u2 count personal asset: %v", err)
		}
		if u2Personal != 0 {
			t.Errorf("u2 sees %d row(s) of u1's personal asset %s, want 0 (personal rows stay hidden)", u2Personal, personalAssetID)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM financial_accounts WHERE id = $1`, acctID).Scan(&u2Acct); err != nil {
			t.Fatalf("u2 count personal account: %v", err)
		}
		if u2Acct != 0 {
			t.Errorf("u2 sees %d row(s) of u1's personal finance account %s, want 0 (personal rows stay hidden)", u2Acct, acctID)
		}
		_ = tx.Rollback(ctx)

		// 5) BOUND to u3 (no household, owns nothing): sees nothing at all.
		tx, err = conn.Begin(ctx)
		if err != nil {
			t.Fatalf("begin u3-bound RLS check: %v", err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, u3); err != nil {
			t.Fatalf("set_config app.user_id to u3: %v", err)
		}
		var u3All int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets`).Scan(&u3All); err != nil {
			t.Fatalf("u3 count assets: %v", err)
		}
		if u3All != 0 {
			t.Errorf("u3 sees %d asset row(s), want 0 (u3 is in no household and owns nothing)", u3All)
		}
		_ = tx.Rollback(ctx)
	})
}
