package postgres_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/infra/postgres"
)

// search_test.go: DB integration tests for the search backend (task 7.5 of the
// proper-multi-tenant-search change). The four scenarios under test:
//
//  1. Cross-owner isolation — a user's search never returns another user's
//     rows, for every one of the five search methods.
//  2. Household member / non-member — a member (not the creator) of a
//     household sees the household's rows in search; a non-member does not.
//  3. Unbound RLS backstop — a raw SQL query with no app.user_id bound returns
//     nothing, bypassing the repository layer entirely.
//  4. Document-join — a document search matches on the joined sources.filename,
//     never on any document column.
//
// Topology mirrors rls_test.go: a dedicated p_search test schema, a shared
// pool (sync.Once lazy init), and fixture data seeded inside user-bound
// transactions (raw unbound writes are rejected by RLS, so seeding must bind
// the user first). All tests share the seeded data and never truncate between
// runs — openTestPool drops and recreates the schema on every test binary
// invocation, so the seed runs exactly once per binary.

const testSchemaSearch = "p_search"

const (
	searchUserA        = "search-a"
	searchUserB        = "search-b"
	searchUserMember   = "search-member"
	searchUserOutsider = "search-outsider"
)

// Marker serials / names / filenames. Every value is prefixed "search-" so the
// fixture is fully isolated from the other test schemas and is safe to
// delete-and-reinsert idempotently (openTestPool recreates the schema on each
// binary run, so a plain INSERT would hit unique indexes on re-runs of the
// same process).
const (
	// Assets
	serAPersonal  = "search-a-001"
	serBPersonal  = "search-b-002"
	serAHousehold = "search-a-h"

	// Accounts (names)
	nameAAccount = "Alpha checking"
	nameBAccount = "Beta savings"

	// Movements
	descAMove = "Alpha coffee purchase"
	extARef   = "REF-A-001"
	descBMove = "Beta grocery run"
	extBRef   = "REF-B-002"

	// Sources (filenames) + documents. Neither filename contains the word
	// "invoice" (the doc_type of both documents) so the document-join test
	// can verify that search matches the source filename, not a document
	// column.
	filA = "alpha-scan-001.pdf"
	filB = "beta-receipt-002.pdf"

	// Import batches (filenames)
	batchAFil = "alpha-statement.csv"
	batchBFil = "beta-statement.csv"

	// Household display name.
	hhDisplay = "Search Household"
)

var (
	searchOnce    sync.Once
	searchPool    *pgxpool.Pool
	searchAdmin   *pgxpool.Pool // superuser pool for test-helper lookups (bypasses RLS)
	searchFactory *repo.Factory
	searchInitErr error
)

// likePattern wraps a literal substring as the ILIKE pattern the SearchBackend
// expects. Per the search spec (proper-multi-tenant-search), the pattern is a
// case-insensitive LITERAL SUBSTRING: LIKE metacharacters in the query are
// escaped and the whole query is wrapped in % wildcards so it matches anywhere
// in the field. In production the core search service (task 8.1) performs this
// wrap + escape before calling SearchBackend; these tests are the DB layer and
// must pass a ready ILIKE pattern. All fixture values contain no % / _ / \, so
// a plain wrap is sufficient (no escaping needed).
func likePattern(sub string) string { return "%" + sub + "%" }

// searchRepos returns the search backend bound to the p_search test schema. It
// lazily initializes the pool (openTestPool drops + recreates the schema and
// runs the migrations), builds the repo.Factory, and seeds the fixture exactly
// once (sync.Once). It mirrors the genericRepos lazy-init pattern.
func searchRepos(t *testing.T) *repo.Factory {
	t.Helper()
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	searchOnce.Do(func() {
		searchPool, searchInitErr = openTestPool(t, testSchemaSearch)
		if searchInitErr != nil {
			return
		}
		// Admin pool (superuser) for test-helper lookups that must bypass RLS.
		// Derive it from the actual test DSN (same database) by swapping the
		// role to pgadmin, so the p_search schema resolves in the right DB.
		adminCfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			searchInitErr = err
			return
		}
		adminCfg.ConnConfig.User = "pgadmin"
		adminCfg.ConnConfig.Password = "pgadmin"
		if adminCfg.ConnConfig.RuntimeParams == nil {
			adminCfg.ConnConfig.RuntimeParams = make(map[string]string)
		}
		adminCfg.ConnConfig.RuntimeParams["search_path"] = testSchemaSearch + ",public"
		searchAdmin, err = pgxpool.NewWithConfig(context.Background(), adminCfg)
		if err != nil {
			searchInitErr = err
			return
		}

		searchFactory = postgres.NewFactory(searchPool)
		searchInitErr = seedSearchFixture(context.Background(), searchPool, searchFactory)
	})
	if searchInitErr != nil {
		t.Fatalf("init search test pool: %v", searchInitErr)
	}
	return searchFactory
}

// boundTx opens a transaction on pool and binds the given user via a
// transaction-scoped set_config. The caller owns Commit/Rollback.
func boundTx(ctx context.Context, pool *pgxpool.Pool, userID string) (pgx.Tx, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, userID); err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf("bind user %q: %w", userID, err)
	}
	return tx, nil
}

// seedSearchFixture registers the four fixture users, creates the household
// (search-a is the creator + member; search-member is a non-creator member;
// search-b and search-outsider are non-members), and seeds one row per owned
// type per user — plus the document/source pair for the document-join test —
// each inside a user-bound transaction.
func seedSearchFixture(ctx context.Context, pool *pgxpool.Pool, f *repo.Factory) error {
	if err := ensureUsers(ctx, pool, searchUserA, searchUserB, searchUserMember, searchUserOutsider); err != nil {
		return fmt.Errorf("ensureUsers: %w", err)
	}

	// Household: created by search-a; search-member is a non-creator member.
	// search-b and search-outsider are deliberately NOT members.
	hh, err := f.Households.Create(ctx, entity.Household{DisplayName: hhDisplay}, repo.Owner(searchUserA))
	if err != nil {
		return fmt.Errorf("create household: %w", err)
	}
	if err := f.Households.AddMember(ctx, hh.ID, searchUserMember, repo.Owner(searchUserA)); err != nil {
		return fmt.Errorf("AddMember(search-member): %w", err)
	}

	// search-a: personal asset + household asset (owner_household_id = H), an
	// account, a movement, a source, a document, and an import batch. All in one
	// bound tx so the account/source FKs resolve and the membership trigger
	// (search-a is the household owner) is satisfied for the household asset.
	if err := seedUserA(ctx, pool, hh.ID); err != nil {
		return err
	}

	// search-b: personal asset, account, movement, source, document, import
	// batch (all personal — search-b is in no household).
	if err := seedUserB(ctx, pool); err != nil {
		return err
	}

	// The two non-members need no rows of their own: the assertions that matter
	// are the negative ones (they must NOT see the household or other users'
	// rows). ensureUsers above already registered them in the user registry.
	return nil
}

// seedUserA seeds search-a's rows inside one bound transaction.
func seedUserA(ctx context.Context, pool *pgxpool.Pool, hhID string) error {
	tx, err := boundTx(ctx, pool, searchUserA)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Reset any leftover fixture rows (idempotent re-seed).
	for _, table := range []string{"import_batches", "documents", "money_movements", "financial_accounts", "sources", "assets"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE owner_id = $1`, searchUserA); err != nil {
			return fmt.Errorf("reset %s for %q: %w", table, searchUserA, err)
		}
	}

	// Assets: personal (S-A-001) and household (S-A-H, owner_household_id = H).
	// Data fields (brand/model/serial + norms) live in payload.data.*; the raw
	// and norm keys are both present since the search ILIKEs the raw fields and
	// the identity indexes key on the norm fields.
	var assetAID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO assets (owner_id, payload)
		 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			'brand', 'AlphaCorp', 'norm_brand', 'alphacorp',
			'model', 'X1', 'norm_model', 'x1',
			'serial', $2::text, 'norm_serial', 'SEARCH-A-001'))) RETURNING id`,
		searchUserA, serAPersonal).Scan(&assetAID); err != nil {
		return fmt.Errorf("seed personal asset: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO assets (owner_id, owner_household_id, payload)
		 VALUES ($1, $3, jsonb_build_object('data', jsonb_build_object(
			'brand', 'AlphaCorp', 'norm_brand', 'alphacorp',
			'model', 'XH', 'norm_model', 'xh',
			'serial', $2::text, 'norm_serial', 'SEARCH-A-H')))`,
		searchUserA, serAHousehold, hhID); err != nil {
		return fmt.Errorf("seed household asset: %w", err)
	}

	// Account.
	var accountAID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO financial_accounts (owner_id, payload)
		 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			'name', $2::text, 'account_type', 'bank', 'currency', 'USD', 'institution', 'FirstBank'))) RETURNING id`,
		searchUserA, nameAAccount).Scan(&accountAID); err != nil {
		return fmt.Errorf("seed account: %w", err)
	}

	// Movement (expense from the account so the chk_kind_accounts constraint holds).
	if _, err := tx.Exec(ctx,
		`INSERT INTO money_movements (owner_id, source_account_id, payload)
		 VALUES ($1, $3, jsonb_build_object('data', jsonb_build_object(
			'kind', 'expense', 'amount', '4.20', 'currency', 'USD',
			'occurred_on', '2025-01-15',
			'description', $2::text, 'norm_description', 'alpha coffee purchase',
			'origin', 'manual', 'external_reference', $4::text)))`,
		searchUserA, descAMove, accountAID, extARef); err != nil {
		return fmt.Errorf("seed movement: %w", err)
	}

	// Source + document (the document-join fixture).
	var sourceAID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO sources (owner_id, payload)
		 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			'filename', $2::text, 'content_type', 'application/pdf',
			'byte_size', 1024, 'storage_path', 'storage/a.pdf', 'sha256', 'sha-a'))) RETURNING id`,
		searchUserA, filA).Scan(&sourceAID); err != nil {
		return fmt.Errorf("seed source: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO documents (owner_id, asset_id, source_id, payload)
		 VALUES ($1, $2, $3, jsonb_build_object('data', jsonb_build_object(
			'doc_type', 'invoice', 'extracted_fields', '{}'::jsonb, 'raw_extraction', '"raw-a"')))`,
		searchUserA, assetAID, sourceAID); err != nil {
		return fmt.Errorf("seed document: %w", err)
	}

	// Import batch (references the account and the source).
	if _, err := tx.Exec(ctx,
		`INSERT INTO import_batches (owner_id, account_id, source_id, payload)
		 VALUES ($1, $2, $3, jsonb_build_object('data', jsonb_build_object(
			'state', 'preview', 'filename', $4::text, 'format', 'csv')))`,
		searchUserA, accountAID, sourceAID, batchAFil); err != nil {
		return fmt.Errorf("seed import batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit search-a seed: %w", err)
	}
	return nil
}

// seedUserB seeds search-b's rows inside one bound transaction.
func seedUserB(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := boundTx(ctx, pool, searchUserB)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Reset any leftover fixture rows (idempotent re-seed).
	for _, table := range []string{"import_batches", "documents", "money_movements", "financial_accounts", "sources", "assets"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE owner_id = $1`, searchUserB); err != nil {
			return fmt.Errorf("reset %s for %q: %w", table, searchUserB, err)
		}
	}

	// Personal asset (S-B-002).
	var assetBID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO assets (owner_id, payload)
		 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			'brand', 'BetaCorp', 'norm_brand', 'betacorp',
			'model', 'Y2', 'norm_model', 'y2',
			'serial', $2::text, 'norm_serial', 'SEARCH-B-002'))) RETURNING id`,
		searchUserB, serBPersonal).Scan(&assetBID); err != nil {
		return fmt.Errorf("seed personal asset: %w", err)
	}

	// Account.
	var accountBID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO financial_accounts (owner_id, payload)
		 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			'name', $2::text, 'account_type', 'bank', 'currency', 'USD', 'institution', 'SecondBank'))) RETURNING id`,
		searchUserB, nameBAccount).Scan(&accountBID); err != nil {
		return fmt.Errorf("seed account: %w", err)
	}

	// Movement (income into the account so the chk_kind_accounts constraint holds).
	if _, err := tx.Exec(ctx,
		`INSERT INTO money_movements (owner_id, destination_account_id, payload)
		 VALUES ($1, $3, jsonb_build_object('data', jsonb_build_object(
			'kind', 'income', 'amount', '12.00', 'currency', 'USD',
			'occurred_on', '2025-01-16',
			'description', $2::text, 'norm_description', 'beta grocery run',
			'origin', 'manual', 'external_reference', $4::text)))`,
		searchUserB, descBMove, accountBID, extBRef); err != nil {
		return fmt.Errorf("seed movement: %w", err)
	}

	// Source + document (the document-join fixture).
	var sourceBID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO sources (owner_id, payload)
		 VALUES ($1, jsonb_build_object('data', jsonb_build_object(
			'filename', $2::text, 'content_type', 'application/pdf',
			'byte_size', 2048, 'storage_path', 'storage/b.pdf', 'sha256', 'sha-b'))) RETURNING id`,
		searchUserB, filB).Scan(&sourceBID); err != nil {
		return fmt.Errorf("seed source: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO documents (owner_id, asset_id, source_id, payload)
		 VALUES ($1, $2, $3, jsonb_build_object('data', jsonb_build_object(
			'doc_type', 'invoice', 'extracted_fields', '{}'::jsonb, 'raw_extraction', '"raw-b"')))`,
		searchUserB, assetBID, sourceBID); err != nil {
		return fmt.Errorf("seed document: %w", err)
	}

	// Import batch (references the account and the source).
	if _, err := tx.Exec(ctx,
		`INSERT INTO import_batches (owner_id, account_id, source_id, payload)
		 VALUES ($1, $2, $3, jsonb_build_object('data', jsonb_build_object(
			'state', 'preview', 'filename', $4::text, 'format', 'csv')))`,
		searchUserB, accountBID, sourceBID, batchBFil); err != nil {
		return fmt.Errorf("seed import batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit search-b seed: %w", err)
	}
	return nil
}

// ---- assertion helpers over search result slices ----

// assetSerial reports whether any asset in got has the given serial_number.
func assetSerial(got []entity.Asset, serial string) bool {
	for _, a := range got {
		if a.SerialNumber != nil && *a.SerialNumber == serial {
			return true
		}
	}
	return false
}

// accountNameSet maps the returned accounts by name.
func accountNameSet(got []entity.FinancialAccount) map[string]bool {
	m := make(map[string]bool)
	for _, a := range got {
		m[a.Name] = true
	}
	return m
}

// movementDescSet maps the returned movements by description.
func movementDescSet(got []entity.MoneyMovement) map[string]bool {
	m := make(map[string]bool)
	for _, mv := range got {
		m[mv.Description] = true
	}
	return m
}

// docFilenameSet resolves each returned document's joined source filename. The
// Document entity does not carry the source filename, so it is looked up by the
// document's source_id. The lookup uses searchAdmin (superuser pool) because the
// app role has RLS on sources and an unbound query would see nothing.
func docFilenameSet(t *testing.T, pool *pgxpool.Pool, got []entity.Document) map[string]bool {
	t.Helper()
	m := make(map[string]bool)
	for _, d := range got {
		if d.SourceID == "" {
			continue
		}
		var fn string
		if err := searchAdmin.QueryRow(context.Background(), `SELECT payload #>> '{data,filename}' FROM sources WHERE id = $1`, d.SourceID).Scan(&fn); err != nil {
			t.Fatalf("resolve source filename for doc %s: %v", d.ID, err)
		}
		m[fn] = true
	}
	return m
}

// batchFilenameSet maps the returned import batches by filename.
func batchFilenameSet(got []entity.ImportBatch) map[string]bool {
	m := make(map[string]bool)
	for _, b := range got {
		m[b.Filename] = true
	}
	return m
}

// TestSearchCrossOwnerIsolation verifies that, for each of the five search
// methods, searching as search-a with a pattern that would match BOTH users'
// rows returns only search-a's rows — never search-b's.
func TestSearchCrossOwnerIsolation(t *testing.T) {
	t.Parallel()
	f := searchRepos(t)
	ctx := context.Background()
	s := f.Search

	t.Run("assets", func(t *testing.T) {
		got, err := s.SearchAssets(ctx, likePattern("Corp"), repo.Filters{}, repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchAssets: %v", err)
		}
		// "Corp" matches the brand of BOTH users' personal assets (AlphaCorp /
		// BetaCorp) and both of search-a's assets. search-a sees exactly her two
		// AlphaCorp assets and never search-b's BetaCorp asset.
		if len(got) != 2 {
			t.Errorf("assets: search-a got %d assets, want 2 (her personal + household AlphaCorp)", len(got))
		}
		if !assetSerial(got, serAPersonal) {
			t.Errorf("assets: search-a missing her personal asset (serial %s)", serAPersonal)
		}
		if !assetSerial(got, serAHousehold) {
			t.Errorf("assets: search-a missing her household asset (serial %s)", serAHousehold)
		}
		if assetSerial(got, serBPersonal) {
			t.Errorf("assets: search-a saw search-b's asset (serial %s) (cross-owner leak)", serBPersonal)
		}
	})

	t.Run("accounts", func(t *testing.T) {
		got, err := s.SearchAccounts(ctx, likePattern("Bank"), repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchAccounts: %v", err)
		}
		names := accountNameSet(got)
		if !names[nameAAccount] {
			t.Errorf("accounts: search-a missing her account %q", nameAAccount)
		}
		if names[nameBAccount] {
			t.Errorf("accounts: search-a saw search-b's account %q (cross-owner leak)", nameBAccount)
		}
	})

	t.Run("movements", func(t *testing.T) {
		// "purchase" appears only in search-a's movement; the negative check is
		// that search-b's movement never leaks in.
		got, err := s.SearchMovements(ctx, likePattern("purchase"), repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchMovements: %v", err)
		}
		descs := movementDescSet(got)
		if !descs[descAMove] {
			t.Errorf("movements: search-a missing her movement %q", descAMove)
		}
		if descs[descBMove] {
			t.Errorf("movements: search-a saw search-b's movement %q (cross-owner leak)", descBMove)
		}
	})

	t.Run("documents", func(t *testing.T) {
		// "alpha" matches only search-a's source filename.
		got, err := s.SearchDocuments(ctx, likePattern("alpha"), repo.Filters{}, repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchDocuments: %v", err)
		}
		fns := docFilenameSet(t, searchPool, got)
		if !fns[filA] {
			t.Errorf("documents: search-a missing her document (source %q)", filA)
		}
		if fns[filB] {
			t.Errorf("documents: search-a saw search-b's document (source %q) (cross-owner leak)", filB)
		}
	})

	t.Run("import_batches", func(t *testing.T) {
		// "statement" appears in BOTH users' batch filenames (alpha-statement.csv
		// / beta-statement.csv) — the strongest cross-owner signal: search-a must
		// get her own and never search-b's.
		got, err := s.SearchImportBatches(ctx, likePattern("statement"), repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchImportBatches: %v", err)
		}
		fns := batchFilenameSet(got)
		if !fns[batchAFil] {
			t.Errorf("import_batches: search-a missing her batch %q", batchAFil)
		}
		if fns[batchBFil] {
			t.Errorf("import_batches: search-a saw search-b's batch %q (cross-owner leak)", batchBFil)
		}
	})
}

// TestSearchHouseholdMember verifies the D-8 household rule end to end for
// search: a household member (not the creator) sees the household's rows in
// search; a non-member does not. The member does NOT see the creator's personal
// rows.
func TestSearchHouseholdMember(t *testing.T) {
	t.Parallel()
	f := searchRepos(t)
	ctx := context.Background()
	s := f.Search

	t.Run("member sees household asset, not personal", func(t *testing.T) {
		got, err := s.SearchAssets(ctx, likePattern("AlphaCorp"), repo.Filters{}, repo.Owner(searchUserMember))
		if err != nil {
			t.Fatalf("SearchAssets(member): %v", err)
		}
		// The household asset (owner_household_id = H) is shared to members, so
		// search-member sees exactly one AlphaCorp asset. The creator's personal
		// asset (owner_household_id IS NULL) is NOT shared to members.
		if len(got) != 1 {
			t.Errorf("member: got %d assets for 'AlphaCorp', want exactly 1 (the household asset)", len(got))
		}
		if !assetSerial(got, serAHousehold) {
			t.Errorf("member: missing the household asset (serial %s)", serAHousehold)
		}
		if assetSerial(got, serAPersonal) {
			t.Errorf("member: saw the creator's personal asset (serial %s) — not shared to members", serAPersonal)
		}
	})

	t.Run("non-member sees nothing", func(t *testing.T) {
		got, err := s.SearchAssets(ctx, likePattern("AlphaCorp"), repo.Filters{}, repo.Owner(searchUserOutsider))
		if err != nil {
			t.Fatalf("SearchAssets(outsider): %v", err)
		}
		if len(got) != 0 {
			t.Errorf("non-member: got %d assets for 'AlphaCorp', want 0 (not a household member)", len(got))
		}
	})
}

// TestSearchUnboundRLSBackstop verifies the RLS backstop holds when the
// repository layer is bypassed entirely: a raw SQL query executed on a plain
// pool connection with no app.user_id bound returns nothing. This mirrors the
// unbound-session test in rls_test.go and runs across every RLS-protected
// owned table (not just assets).
func TestSearchUnboundRLSBackstop(t *testing.T) {
	t.Parallel()
	searchRepos(t) // ensure schema, migrations, and seed rows exist
	ctx := context.Background()

	// A plain pool transaction, NO set_config('app.user_id', ...).
	tx, err := searchPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unbound tx: %v", err)
	}
	defer tx.Rollback(ctx)

	for _, table := range []string{
		"assets",
		"financial_accounts",
		"money_movements",
		"documents",
		"import_batches",
		"sources",
	} {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("unbound count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("unbound SELECT count(%s) = %d, want 0 (RLS must hide everything with no bound user)", table, count)
		}
	}
	tx.Rollback(ctx)
}

// TestSearchDocumentJoin verifies the document search matches on the joined
// sources.filename, never on any document column. search-a's document is
// reachable by the source filename "alpha-invoice" but not by a pattern that
// would only match a document column (doc_type "invoice"). search-b's document
// is reachable by "beta-receipt" and hidden from search-a.
func TestSearchDocumentJoin(t *testing.T) {
	t.Parallel()
	f := searchRepos(t)
	ctx := context.Background()
	s := f.Search

	t.Run("matches joined source filename, not a document column", func(t *testing.T) {
		// "invoice" appears in the doc_type column of BOTH documents but in
		// NEITHER source filename. If search matched a document column, this
		// would return rows; because it matches only the joined source filename,
		// it must return nothing for search-a.
		got, err := s.SearchDocuments(ctx, likePattern("invoice"), repo.Filters{}, repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchDocuments('invoice'): %v", err)
		}
		if len(got) != 0 {
			t.Errorf("documents: search for a document-column value 'invoice' returned %d rows, want 0 (search must match the source filename, not a document column)", len(got))
		}
	})

	t.Run("search-a sees alpha-scan via source filename", func(t *testing.T) {
		got, err := s.SearchDocuments(ctx, likePattern("alpha-scan"), repo.Filters{}, repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchDocuments('alpha-scan'): %v", err)
		}
		fns := docFilenameSet(t, searchPool, got)
		if !fns[filA] {
			t.Errorf("search-a: missing her document (source %q) via the joined filename", filA)
		}
		if fns[filB] {
			t.Errorf("search-a: saw search-b's document (source %q) (cross-owner leak)", filB)
		}
	})

	t.Run("search-a does not see beta-receipt", func(t *testing.T) {
		got, err := s.SearchDocuments(ctx, likePattern("beta-receipt"), repo.Filters{}, repo.Owner(searchUserA))
		if err != nil {
			t.Fatalf("SearchDocuments('beta-receipt'): %v", err)
		}
		if len(got) != 0 {
			t.Errorf("search-a: %d rows for 'beta-receipt', want 0 (search-b's document is invisible)", len(got))
		}
	})

	t.Run("search-b sees beta-receipt", func(t *testing.T) {
		got, err := s.SearchDocuments(ctx, likePattern("beta-receipt"), repo.Filters{}, repo.Owner(searchUserB))
		if err != nil {
			t.Fatalf("SearchDocuments('beta-receipt') as search-b: %v", err)
		}
		fns := docFilenameSet(t, searchPool, got)
		if !fns[filB] {
			t.Errorf("search-b: missing her document (source %q) via the joined filename", filB)
		}
	})
}
