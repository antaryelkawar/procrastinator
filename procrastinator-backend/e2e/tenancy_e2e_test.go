// Package e2e contains a whole-system, end-to-end tenancy test. It drives the
// ACTUAL stack — the real api.Server over chi, a real Postgres pool connected
// as the app role (procrastinator, NOBYPASSRLS), and real on-disk file storage
// in a temp dir — and walks the owner-model user journey (household-scoped vs
// personal assets, member visibility, non-member isolation, and the RLS
// backstop on the shared tables).
//
// Precondition: the compose `postgres` service is running and migrations are
// applied to v1 (00001_schema, the fresh uniform-jsonb set). When the database
// is unreachable or not migrated, the test skips with a reason rather than failing.
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
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/lifecycle"
	"procrastinator-backend/core/processing"
	"procrastinator-backend/core/review"
	"procrastinator-backend/core/search"
	"procrastinator-backend/core/statement"
	"procrastinator-backend/infra/filestorage"
	"procrastinator-backend/infra/pdftext"
	"procrastinator-backend/infra/postgres"
)

// defaultDSN is the app-role DSN for the compose postgres service (role
// procrastinator / password procrastinator, database procrastinator). It can be
// overridden with PROCRASTINATOR_E2E_DATABASE_URL.
const defaultDSN = "postgres://procrastinator:procrastinator@localhost:5432/procrastinator?sslmode=disable"

// fakeChatter is a deterministic processing.Chatter that returns a fixed
// extraction payload as the LLM content. The external LLM is the only stubbed
// collaborator (the established pattern in the api package tests); everything
// else — HTTP, PG, file storage — is real.
type fakeChatter struct{ payload string }

// Chat implements processing.Chatter.
func (e fakeChatter) Chat(_ context.Context, _ string, _ []processing.ContentPart) (string, error) {
	return e.payload, nil
}

var _ processing.Chatter = fakeChatter{}

// invoicePayload returns a valid invoice extraction whose serial is unique per
// run so identity resolution creates a distinct personal asset.
func invoicePayload(runID string) string {
	return fmt.Sprintf(
		`{"classification":"invoice","brand":"E2E","model":"Model-%s","serial_number":"E2E-%s-P","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"100.00","currency":"INR","confidence":0.95,"metadata":{}}`,
		runID, runID,
	)
}

// buildServer wires the real api.Server (processing + lifecycle + ledger +
// statement) over the given factory/storage with a deterministic fake LLM and
// the requested number of extraction workers, and returns its chi router.
// Two agreeing workers yield consensus confidence 0.9 (auto-commit at the 0.7
// threshold -> 201); a single worker caps confidence at 0.6 (held for review
// -> 202).
func buildServer(t *testing.T, storageDir string, pool *pgxpool.Pool, factory *repo.Factory, payload string, workerCount int) http.Handler {
	t.Helper()
	const maxBytes = int64(20 * 1024 * 1024)

	movRepo := postgres.NewMovementRepository(pool)
	ledgerSvc := ledger.New(factory, movRepo)
	reviewSvc := review.New(factory)
	searchSvc := search.New(factory.Search)

	workers := make([]processing.Worker, 0, workerCount)
	for i := 0; i < workerCount; i++ {
		workers = append(workers, processing.Worker{
			Model:    "e2e-worker",
			BaseURL:  "",
			Strategy: processing.Strategy{Name: "extract"},
		})
	}
	extractor := processing.NewExtractor(fakeChatter{payload: payload}, workers, 10*time.Second, "")
	svc := processing.New(factory, extractor, filestorage.New(storageDir), maxBytes, 0.7, reviewSvc, 30, 10, 10*time.Second)
	svc.SetTextStorage(filestorage.NewText(t.TempDir()))
	lifecycleSvc := lifecycle.New(factory, 30)
	stmtSvc := statement.New(
		factory,
		filestorage.NewStatement(t.TempDir()),
		movRepo,
		postgres.NewDocumentRepository(pool),
		pdftext.New(),
		maxBytes,
		100000,
	)
	srv := api.New(svc, factory, ledgerSvc, movRepo, maxBytes, stmtSvc, maxBytes, household.New(factory), searchSvc, reviewSvc, lifecycleSvc)
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

// dataMap returns the nested "data" object of an IngestReview JSON body, or an
// empty map when absent (the review payload fields live under data.*).
func dataMap(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return map[string]any{}
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

	// The owner model needs the fresh uniform-jsonb schema (00001).
	// Skip if the schema is not at v1 rather than failing on a missing object.
	var maxVersion int
	if err := pool.QueryRow(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version`).Scan(&maxVersion); err != nil {
		t.Skipf("check goose_db_version: %v", err)
	}
	if maxVersion != 1 {
		t.Skipf("schema not at v1 (fresh uniform-jsonb set); max goose version is %d; run goose migrations to v1 first", maxVersion)
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
	handler := buildServer(t, storageDir, pool, factory, invoicePayload(runID), 2)

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
		if _, err := tx.Exec(cctx, "DELETE FROM ingest_reviews WHERE owner_id = $1", u1); err != nil {
			t.Errorf("cleanup ingest_reviews: %v", err)
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

// TestConfidenceReviewE2E drives the confidence gate + review lifecycle
// end-to-end: low-confidence upload → 202 (held) → list → approve → asset+doc
// created; separate low-confidence upload → 202 → reject → no asset, source
// retained.
func TestConfidenceReviewE2E(t *testing.T) {
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
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PG unreachable (ping): %v", err)
	}

	var maxVersion int
	if err := pool.QueryRow(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version`).Scan(&maxVersion); err != nil {
		t.Skipf("check goose_db_version: %v", err)
	}
	if maxVersion != 1 {
		t.Skipf("schema not at v1 (fresh uniform-jsonb set); max goose version is %d", maxVersion)
	}

	var rb [4]byte
	if _, err := rand.Read(rb[:]); err != nil {
		t.Fatalf("read crypto/rand: %v", err)
	}
	runID := fmt.Sprintf("e2e-r-%d-%s", time.Now().Unix(), hex.EncodeToString(rb[:]))
	u1, u2 := runID+"-owner", runID+"-other"

	// Build server with a single worker (confidence capped at 0.6 < 0.7
	// threshold, so uploads are held for review).
	storageDir := t.TempDir()
	factory := postgres.NewFactory(pool)
	handler := buildServer(t, storageDir, pool, factory, invoicePayload(runID), 1)

	if err := provisionUsers(ctx, pool, u1, u2); err != nil {
		t.Fatalf("provision users: %v", err)
	}

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx, err := pool.Begin(cctx)
		if err != nil {
			return
		}
		if _, err := tx.Exec(cctx, `SELECT set_config('app.user_id', $1, true)`, u1); err != nil {
			return
		}
		_, _ = tx.Exec(cctx, "DELETE FROM documents WHERE owner_id = $1", u1)
		_, _ = tx.Exec(cctx, "DELETE FROM ingest_reviews WHERE owner_id = $1", u1)
		_, _ = tx.Exec(cctx, "DELETE FROM assets WHERE owner_id = $1", u1)
		_, _ = tx.Exec(cctx, "DELETE FROM sources WHERE owner_id = $1", u1)
		_ = tx.Commit(cctx)
		for _, u := range []string{u1, u2} {
			_, _ = pool.Exec(cctx, "DELETE FROM users WHERE id = $1", u)
		}
	})

	// (a) Upload with low confidence → 202 (held for review).
	var reviewID string
	t.Run("a_low_confidence_upload_held", func(t *testing.T) {
		status, body := httpPostFileForm(t, handler, "/api/users/"+u1+"/documents", "review-approve.pdf", pdfBytes("ra-"+runID), nil)
		if status != http.StatusAccepted {
			t.Fatalf("low-conf upload status = %d, want 202 (body: %s)", status, body)
		}
		var rev map[string]any
		if err := json.Unmarshal(body, &rev); err != nil {
			t.Fatalf("unmarshal 202 response: %v (body: %s)", err, body)
		}
		reviewID = strVal(rev, "id")
		if reviewID == "" {
			t.Fatalf("review id is empty (body: %s)", body)
		}
		if state := strVal(dataMap(rev), "state"); state != "pending" {
			t.Errorf("review state = %q, want pending (body: %s)", state, body)
		}
		if conf, ok := dataMap(rev)["confidence"].(float64); !ok || conf < 0.6 || conf > 0.61 {
			t.Errorf("review confidence = %v, want ~0.6 (body: %s)", dataMap(rev)["confidence"], body)
		}
	})

	// (b) List reviews → the pending review appears.
	t.Run("b_list_shows_pending_review", func(t *testing.T) {
		status, body := httpGet(t, handler, "/api/users/"+u1+"/ingest/reviews")
		if status != http.StatusOK {
			t.Fatalf("list reviews status = %d, want 200 (body: %s)", status, body)
		}
		var reviews []map[string]any
		if err := json.Unmarshal(body, &reviews); err != nil {
			t.Fatalf("unmarshal reviews: %v (body: %s)", err, body)
		}
		if len(reviews) != 1 {
			t.Fatalf("list returned %d reviews, want 1 (body: %s)", len(reviews), body)
		}
		if got := strVal(reviews[0], "id"); got != reviewID {
			t.Errorf("listed review id = %q, want %q", got, reviewID)
		}
	})

	// (c) Approve the review → 200 with the resulting asset.
	var approvedAssetID string
	t.Run("c_approve_creates_asset", func(t *testing.T) {
		status, body := httpPostJSON(t, handler, "/api/users/"+u1+"/ingest/reviews/"+reviewID+"/approve", nil)
		if status != http.StatusOK {
			t.Fatalf("approve status = %d, want 200 (body: %s)", status, body)
		}
		var resp struct {
			Asset  map[string]any `json:"asset"`
			Review map[string]any `json:"review"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal approve response: %v (body: %s)", err, body)
		}
		approvedAssetID = strVal(resp.Asset, "id")
		if approvedAssetID == "" {
			t.Fatalf("approved asset id is empty (body: %s)", body)
		}
		if state := strVal(dataMap(resp.Review), "state"); state != "approved" {
			t.Errorf("review state after approve = %q, want approved (body: %s)", state, body)
		}
		if dataMap(resp.Review)["decided_at"] == nil {
			t.Error("decided_at is nil after approve, want set")
		}
	})

	// (d) The approved asset is retrievable and has a document.
	t.Run("d_approved_asset_exists", func(t *testing.T) {
		if approvedAssetID == "" {
			t.Skipf("prerequisite (c) did not produce an asset id")
		}
		status, body := httpGet(t, handler, "/api/users/"+u1+"/assets/"+approvedAssetID)
		if status != http.StatusOK {
			t.Fatalf("GET approved asset status = %d, want 200 (body: %s)", status, body)
		}
		var asset map[string]any
		if err := json.Unmarshal(body, &asset); err != nil {
			t.Fatalf("unmarshal asset: %v (body: %s)", err, body)
		}
		if strVal(asset, "id") != approvedAssetID {
			t.Errorf("asset id = %q, want %q", strVal(asset, "id"), approvedAssetID)
		}

		// The asset should have a document linked.
		dStatus, dBody := httpGet(t, handler, "/api/users/"+u1+"/assets/"+approvedAssetID+"/documents")
		if dStatus != http.StatusOK {
			t.Fatalf("GET asset documents status = %d, want 200 (body: %s)", dStatus, dBody)
		}
		var docs []map[string]any
		if err := json.Unmarshal(dBody, &docs); err != nil {
			t.Fatalf("unmarshal documents: %v (body: %s)", err, dBody)
		}
		if len(docs) < 1 {
			t.Errorf("approved asset has %d documents, want >=1 (body: %s)", len(docs), dBody)
		}
	})

	// (e) Upload a second document → 202, then reject → no asset created,
	// source retained.
	var rejectReviewID string
	t.Run("e_second_upload_and_reject", func(t *testing.T) {
		// Use a different serial to avoid identity merge.
		payload := fmt.Sprintf(
			`{"classification":"invoice","brand":"E2E","model":"Reject-%s","serial_number":"E2E-%s-RJ","purchase_date":"2024-01-12","warranty_end":"2027-01-12","price":"50.00","currency":"INR","confidence":0.3,"metadata":{}}`,
			runID, runID,
		)
		// Rebuild with different payload for this upload — use a second server.
		factory2 := postgres.NewFactory(pool)
		handler2 := buildServer(t, storageDir, pool, factory2, payload, 1)

		status, body := httpPostFileForm(t, handler2, "/api/users/"+u1+"/documents", "review-reject.pdf", pdfBytes("rr-"+runID), nil)
		if status != http.StatusAccepted {
			t.Fatalf("second low-conf upload status = %d, want 202 (body: %s)", status, body)
		}
		var rev map[string]any
		if err := json.Unmarshal(body, &rev); err != nil {
			t.Fatalf("unmarshal second 202 response: %v (body: %s)", err, body)
		}
		rejectReviewID = strVal(rev, "id")
		if rejectReviewID == "" {
			t.Fatalf("second review id is empty (body: %s)", body)
		}

		// Record the source count (bound to u1 for RLS) for later verification.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin source count tx: %v", err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, u1); err != nil {
			t.Fatalf("set_config for source count: %v", err)
		}
		var srcCount int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM sources WHERE owner_id = $1", u1).Scan(&srcCount); err != nil {
			t.Fatalf("count sources: %v", err)
		}
		_ = tx.Rollback(ctx)
		if srcCount < 2 {
			t.Errorf("source count = %d, want >=2 (both uploads should retain sources)", srcCount)
		}

		// Reject the second review.
		status, body = httpPostJSON(t, handler2, "/api/users/"+u1+"/ingest/reviews/"+rejectReviewID+"/reject", nil)
		if status != http.StatusOK {
			t.Fatalf("reject status = %d, want 200 (body: %s)", status, body)
		}
		var updated map[string]any
		if err := json.Unmarshal(body, &updated); err != nil {
			t.Fatalf("unmarshal reject response: %v (body: %s)", err, body)
		}
		if state := strVal(dataMap(updated), "state"); state != "rejected" {
			t.Errorf("review state after reject = %q, want rejected (body: %s)", state, body)
		}
		if dataMap(updated)["decided_at"] == nil {
			t.Error("decided_at is nil after reject, want set")
		}
	})

	// (f) After reject: no new asset was created for the rejected candidate,
	// but the source is still retained.
	t.Run("f_rejected_no_asset_source_retained", func(t *testing.T) {
		if rejectReviewID == "" {
			t.Skipf("prerequisite (e) did not produce a review id")
		}
		// The rejected review should not have produced an asset. Verify by
		// listing assets: only the one from (c) should exist.
		status, body := httpGet(t, handler, "/api/users/"+u1+"/assets")
		if status != http.StatusOK {
			t.Fatalf("list assets status = %d, want 200 (body: %s)", status, body)
		}
		var assets []map[string]any
		if err := json.Unmarshal(body, &assets); err != nil {
			t.Fatalf("unmarshal assets: %v (body: %s)", err, body)
		}
		if len(assets) != 1 {
			t.Errorf("asset count = %d, want 1 (only the approved one; rejected candidate should not create an asset)", len(assets))
		}

		// Source is retained: bound DB check.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin source-retain tx: %v", err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, u1); err != nil {
			t.Fatalf("set_config for source retain check: %v", err)
		}
		var srcCount int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM sources WHERE owner_id = $1", u1).Scan(&srcCount); err != nil {
			t.Fatalf("count sources: %v", err)
		}
		_ = tx.Rollback(ctx)
		if srcCount < 2 {
			t.Errorf("source count = %d, want >=2 (rejected upload's source must be retained)", srcCount)
		}
	})

	// (g) Another owner (u2) cannot see u1's reviews.
	t.Run("g_cross_owner_review_invisible", func(t *testing.T) {
		status, body := httpGet(t, handler, "/api/users/"+u2+"/ingest/reviews")
		if status != http.StatusOK {
			t.Fatalf("u2 list reviews status = %d, want 200 (body: %s)", status, body)
		}
		var reviews []map[string]any
		if err := json.Unmarshal(body, &reviews); err != nil {
			t.Fatalf("unmarshal u2 reviews: %v (body: %s)", err, body)
		}
		if len(reviews) != 0 {
			t.Errorf("u2 sees %d reviews, want 0 (cross-owner isolation)", len(reviews))
		}

		// u2 cannot get u1's review by id.
		if reviewID != "" {
			if s, b := httpGet(t, handler, "/api/users/"+u2+"/ingest/reviews/"+reviewID); s != http.StatusNotFound {
				t.Errorf("u2 GET u1's review status = %d, want 404 (body: %s)", s, b)
			}
		}
	})
}

// TestSearchIsolationE2E drives the search isolation scenarios end-to-end:
// cross-owner isolation, household member sees household asset, non-member sees
// nothing, unbound RLS session sees nothing.
func TestSearchIsolationE2E(t *testing.T) {
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
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PG unreachable (ping): %v", err)
	}

	var maxVersion int
	if err := pool.QueryRow(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version`).Scan(&maxVersion); err != nil {
		t.Skipf("check goose_db_version: %v", err)
	}
	if maxVersion != 1 {
		t.Skipf("schema not at v1 (fresh uniform-jsonb set); max goose version is %d", maxVersion)
	}

	var rb [4]byte
	if _, err := rand.Read(rb[:]); err != nil {
		t.Fatalf("read crypto/rand: %v", err)
	}
	runID := fmt.Sprintf("e2e-s-%d-%s", time.Now().Unix(), hex.EncodeToString(rb[:]))
	u1, u2, u3 := runID+"-u1", runID+"-u2", runID+"-u3"

	// Build server with two agreeing workers (consensus confidence 0.9 ≥ 0.7)
	// so uploads auto-commit.
	storageDir := t.TempDir()
	factory := postgres.NewFactory(pool)
	handler := buildServer(t, storageDir, pool, factory, invoicePayload(runID), 2)

	if err := provisionUsers(ctx, pool, u1, u2, u3); err != nil {
		t.Fatalf("provision users: %v", err)
	}

	var hhID, hhAssetID, personalAssetID string

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		tx, err := pool.Begin(cctx)
		if err != nil {
			return
		}
		if _, err := tx.Exec(cctx, `SELECT set_config('app.user_id', $1, true)`, u1); err != nil {
			return
		}
		_, _ = tx.Exec(cctx, "DELETE FROM documents WHERE owner_id = $1", u1)
		_, _ = tx.Exec(cctx, "DELETE FROM ingest_reviews WHERE owner_id = $1", u1)
		_, _ = tx.Exec(cctx, "DELETE FROM assets WHERE owner_id = $1", u1)
		_, _ = tx.Exec(cctx, "DELETE FROM sources WHERE owner_id = $1", u1)
		if hhID != "" {
			_, _ = tx.Exec(cctx, "DELETE FROM household_members WHERE household_id = $1", hhID)
		}
		_, _ = tx.Exec(cctx, "DELETE FROM households WHERE owner_id = $1", u1)
		_ = tx.Commit(cctx)
		for _, u := range []string{u1, u2, u3} {
			_, _ = pool.Exec(cctx, "DELETE FROM users WHERE id = $1", u)
		}
	})

	// (a) Create household, add u2 as member.
	t.Run("a_household_setup", func(t *testing.T) {
		status, body := httpPostJSON(t, handler, "/api/users/"+u1+"/households", map[string]string{"display_name": "E2E Search HH"})
		if status != http.StatusCreated {
			t.Fatalf("create household status = %d, want 201 (body: %s)", status, body)
		}
		var hh struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &hh); err != nil {
			t.Fatalf("unmarshal household: %v (body: %s)", err, body)
		}
		hhID = hh.ID
		if hhID == "" {
			t.Fatal("household id is empty")
		}
		if s, b := httpPostJSON(t, handler, "/api/users/"+u1+"/households/"+hhID+"/members", map[string]string{"user_id": u2}); s != http.StatusNoContent {
			t.Fatalf("add member status = %d, want 204 (body: %s)", s, b)
		}
	})

	// (b) Upload a household-scoped asset and a personal asset (both auto-commit
	// at confidence 0.95).
	t.Run("b_upload_household_and_personal", func(t *testing.T) {
		if hhID == "" {
			t.Skipf("prerequisite (a) did not create household")
		}
		// Household upload.
		status, body := httpPostFileForm(t, handler, "/api/users/"+u1+"/documents", "search-hh.pdf", pdfBytes("sh-"+runID), map[string]string{"owner_household_id": hhID})
		if status != http.StatusCreated {
			t.Fatalf("household upload status = %d, want 201 (body: %s)", status, body)
		}
		var hha map[string]any
		if err := json.Unmarshal(body, &hha); err != nil {
			t.Fatalf("unmarshal household asset: %v (body: %s)", err, body)
		}
		hhAssetID = strVal(hha, "id")
		if hhAssetID == "" {
			t.Fatal("household asset id is empty")
		}

		// Personal upload.
		status, body = httpPostFileForm(t, handler, "/api/users/"+u1+"/documents", "search-personal.pdf", pdfBytes("sp-"+runID), nil)
		if status != http.StatusCreated {
			t.Fatalf("personal upload status = %d, want 201 (body: %s)", status, body)
		}
		var pa map[string]any
		if err := json.Unmarshal(body, &pa); err != nil {
			t.Fatalf("unmarshal personal asset: %v (body: %s)", err, body)
		}
		personalAssetID = strVal(pa, "id")
		if personalAssetID == "" {
			t.Fatal("personal asset id is empty")
		}
	})

	// (c) u1 searches → sees both assets (the brand "E2E" matches both).
	t.Run("c_u1_searches_sees_both", func(t *testing.T) {
		if hhAssetID == "" || personalAssetID == "" {
			t.Skipf("prerequisite (b) did not create assets")
		}
		status, body := httpGet(t, handler, "/api/users/"+u1+"/search/quick?q=E2E")
		if status != http.StatusOK {
			t.Fatalf("u1 quick search status = %d, want 200 (body: %s)", status, body)
		}
		var resp struct {
			Results []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal search response: %v (body: %s)", err, body)
		}
		ids := make([]string, 0, len(resp.Results))
		for _, r := range resp.Results {
			ids = append(ids, r.ID)
		}
		if !contains(ids, hhAssetID) {
			t.Errorf("u1 search does not include household asset %s (saw: %v)", hhAssetID, ids)
		}
		if !contains(ids, personalAssetID) {
			t.Errorf("u1 search does not include personal asset %s (saw: %v)", personalAssetID, ids)
		}
	})

	// (d) u2 (member of H1) searches → sees the household asset but NOT u1's
	// personal asset.
	t.Run("d_u2_member_sees_only_household", func(t *testing.T) {
		if hhAssetID == "" || personalAssetID == "" {
			t.Skipf("prerequisite (b) did not create assets")
		}
		status, body := httpGet(t, handler, "/api/users/"+u2+"/search/quick?q=E2E")
		if status != http.StatusOK {
			t.Fatalf("u2 quick search status = %d, want 200 (body: %s)", status, body)
		}
		var resp struct {
			Results []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal search response: %v (body: %s)", err, body)
		}
		ids := make([]string, 0, len(resp.Results))
		for _, r := range resp.Results {
			ids = append(ids, r.ID)
		}
		if !contains(ids, hhAssetID) {
			t.Errorf("u2 (member) search does not include household asset %s (saw: %v)", hhAssetID, ids)
		}
		if contains(ids, personalAssetID) {
			t.Errorf("u2 (member) search includes u1's personal asset %s, which must stay hidden (saw: %v)", personalAssetID, ids)
		}
	})

	// (e) u3 (non-member) searches → sees nothing.
	t.Run("e_u3_nonmember_sees_nothing", func(t *testing.T) {
		if hhAssetID == "" {
			t.Skipf("prerequisite (b) did not create assets")
		}
		status, body := httpGet(t, handler, "/api/users/"+u3+"/search/quick?q=E2E")
		if status != http.StatusOK {
			t.Fatalf("u3 quick search status = %d, want 200 (body: %s)", status, body)
		}
		var resp struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal search response: %v (body: %s)", err, body)
		}
		if len(resp.Results) != 0 {
			var ids []string
			for _, r := range resp.Results {
				ids = append(ids, r.ID)
			}
			t.Errorf("u3 (non-member) search returned %d hits, want 0 (saw: %v)", len(resp.Results), ids)
		}
	})

	// (f) Unbound RLS backstop: a direct DB session with no bound app.user_id
	// sees nothing in the searched tables.
	t.Run("f_unbound_rls_sees_nothing", func(t *testing.T) {
		if hhAssetID == "" {
			t.Skipf("prerequisite (b) did not create assets")
		}
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatalf("connect for RLS check: %v", err)
		}
		defer conn.Close(ctx)

		// Unbound: no app.user_id set.
		for _, table := range []string{"assets", "sources", "ingest_reviews"} {
			var n int
			if err := conn.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
				t.Fatalf("unbound count %s: %v", table, err)
			}
			if n != 0 {
				t.Errorf("unbound app role sees %d row(s) in %s, want 0 (RLS backstop)", n, table)
			}
		}
	})
}
