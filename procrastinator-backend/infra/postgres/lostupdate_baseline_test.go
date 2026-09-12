package postgres_test

// BASELINE for change #3 `payload-link-model-guardrails`.
//
// The generic pgRepository.Update in infra/postgres/repository.go performs a
// fetch-merge-write across SEPARATE transactions with NO optimistic
// concurrency control: it calls fetchExistingData (repository.go:439) inside
// one scope.run transaction, merges the stored payload in Go, then issues the
// UPDATE inside a second, separate transaction (repository.go:478-489). The
// UPDATE's WHERE clause is "id = $N AND <visibility>" (repository.go:483-485)
// — there is NO version/OCC column, and the merge does "merged[k] = v"
// (repository.go:450,453), which replaces a whole JSONB object rather than
// merging at the sub-key level.
//
// This test documents the PRESENT (unguarded) behavior deterministically: the
// interleaving "A fetches → B fetches → A writes → B writes" ends with B's
// stale write silently overwriting A's write (a lost update) and NO error is
// returned. These assertions PIN the present no-OCC behavior.
//
// When change #3 adds mandatory OCC, this same interleaving must surface a
// conflict (e.g. repo.ErrConflict) and/or preserve both writes; the assertions
// in both subtests below flip. Until then this file must NOT be changed to
// pass — it is a regression guard.
//
// No production code is modified by this file.
import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/infra/postgres"
)

const lostUpdateSchema = "p_lostupdate"

var (
	lostupdateOnce sync.Once
	lostupdatePool *pgxpool.Pool
	lostupdateErr  error
	lostupdateDocs repo.Repository[entity.Document]
	lostupdateMvts repo.Repository[entity.MoneyMovement]
	lostupdateSrcs repo.Repository[entity.Source]
)

// lostUpdateRepos returns the repositories bound to the p_lostupdate test
// schema, lazily initialized once per test binary run.
func lostUpdateRepos(t *testing.T) (repo.Repository[entity.Source], repo.Repository[entity.Document], repo.Repository[entity.MoneyMovement]) {
	t.Helper()
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	lostupdateOnce.Do(func() {
		lostupdatePool, lostupdateErr = openTestPool(t, lostUpdateSchema)
		if lostupdateErr != nil {
			return
		}
		lostupdateSrcs = postgres.NewSourceRepository(lostupdatePool)
		lostupdateDocs = postgres.NewDocumentRepository(lostupdatePool)
		lostupdateMvts = postgres.NewMovementRepository(lostupdatePool)
	})
	if lostupdateErr != nil {
		t.Fatalf("init lost-update test pool: %v", lostupdateErr)
	}
	return lostupdateSrcs, lostupdateDocs, lostupdateMvts
}

// truncateLostUpdate clears all test data on the p_lostupdate pool. Call at the
// top of each subtest.
func truncateLostUpdate(t *testing.T) {
	t.Helper()
	lostUpdateRepos(t) // ensure the pool is initialized
	if _, err := lostupdatePool.Exec(context.Background(), `TRUNCATE money_movements, documents, sources CASCADE`); err != nil {
		t.Fatalf("truncate lost-update: %v", err)
	}
}

// TestLostUpdateBaseline documents the PRESENT no-OCC lost-update behavior
// across two write surfaces: a JSONB payload object (Document.ExtractedFields)
// and a nullable real column (MoneyMovement.LinkedDocumentID). For each, it
// reproduces deterministically (no goroutines) the interleaving
// A-fetch → B-fetch → A-write → B-write and asserts that B's stale write
// silently overwrites A's (A's change is LOST) and NO error is returned.
//
// These assertions hold only because the repository has NO optimistic
// concurrency control. When change #3 `payload-link-model-guardrails` adds
// mandatory OCC, the interleaving must surface a conflict and/or preserve
// both writes, and BOTH subtests below must be updated to assert the new
// guarded behavior.
func TestLostUpdateBaseline(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{name: "JSONB payload (Document.ExtractedFields)", run: testLostUpdateJSONB},
		{name: "link column (MoneyMovement.LinkedDocumentID)", run: testLostUpdateLink},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, tc.run)
	}
}

func testLostUpdateJSONB(t *testing.T) {
	truncateLostUpdate(t)
	ctx := context.Background()

	sources, docs, _ := lostUpdateRepos(t)

	// A document requires a source_id (documents.source_id NOT NULL UNIQUE).
	src, err := sources.Create(ctx, testSource(), repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}

	created, err := docs.Create(ctx, entity.Document{
		SourceID:        src.ID,
		DocType:         entity.DocTypeInvoice,
		ExtractedFields: map[string]any{"base": "v0"},
		// RawExtraction is payload-carried (documents has no raw_extraction
		// column — it lands in payload.data.raw_extraction); set it
		// defensively so the JSONB merge has a non-empty data field.
		RawExtraction: "raw extraction fixture",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create document: %v", err)
	}
	id := created.ID

	// Two independent in-memory fetches (as two concurrent clients would
	// each do before either writes).
	entA, err := docs.Get(ctx, id, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get A: %v", err)
	}
	entB, err := docs.Get(ctx, id, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}

	// Each client adds a DIFFERENT new sub-key to its own copy.
	if entA.ExtractedFields == nil {
		entA.ExtractedFields = map[string]any{}
	}
	if entB.ExtractedFields == nil {
		entB.ExtractedFields = map[string]any{}
	}
	entA.ExtractedFields["from_A"] = "A"
	entB.ExtractedFields["from_B"] = "B"

	// A writes, then B writes. With no OCC, B's stale write wins.
	if _, err := docs.Update(ctx, entA, repo.Owner(userA)); err != nil {
		t.Fatalf("Update A: %v", err)
	}
	if _, err := docs.Update(ctx, entB, repo.Owner(userA)); err != nil {
		// Honest baseline: the no-OCC baseline requires this to SILENTLY
		// overwrite. If a conflict is now surfaced, the baseline no longer
		// holds and this test must be updated for payload-link-model-guardrails.
		t.Fatalf("Update B (expected no error under present no-OCC baseline): %v", err)
	}

	final, err := docs.Get(ctx, id, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get final: %v", err)
	}

	// --- PRESENT-behavior baseline assertions (no OCC) ---
	// Last writer (B) wins: B's sub-key survives.
	if final.ExtractedFields["from_B"] != "B" {
		t.Errorf("JSONB: final.ExtractedFields[from_B] = %v, want \"B\" (last writer B wins)", final.ExtractedFields["from_B"])
	}
	// A's sub-key is LOST: B's stale in-memory copy replaced the whole JSONB object.
	if _, ok := final.ExtractedFields["from_A"]; ok {
		t.Errorf("JSONB: baseline no longer holds — from_A present = %v (OCC now in place; update this baseline for payload-link-model-guardrails)", final.ExtractedFields)
	}
	// The pre-existing key both clients carried survives.
	if final.ExtractedFields["base"] != "v0" {
		t.Errorf("JSONB: final.ExtractedFields[base] = %v, want \"v0\"", final.ExtractedFields["base"])
	}
}

func testLostUpdateLink(t *testing.T) {
	truncateLostUpdate(t)
	ctx := context.Background()

	_, _, mvts := lostUpdateRepos(t)

	// Only owner_id is NOT NULL; kind/amount/currency live in the payload.
	// LinkedDocumentID is nil at create and has NO FK (migrations/00001_schema.sql:194).
	created, err := mvts.Create(ctx, entity.MoneyMovement{
		Kind:             entity.KindExpense,
		Amount:           "10.00",
		Currency:         "USD",
		OccurredOn:       time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		LinkedDocumentID: nil,
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create movement: %v", err)
	}
	id := created.ID

	// Two DISTINCT valid UUIDs. No FK on linked_document_id, so they need
	// not reference real documents.
	docA := "11111111-1111-4111-8111-111111111111"
	docB := "22222222-2222-4222-8222-222222222222"

	entA, err := mvts.Get(ctx, id, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get A: %v", err)
	}
	entB, err := mvts.Get(ctx, id, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}

	// Each client re-points the SAME link column to a DIFFERENT document.
	entA.LinkedDocumentID = &docA
	entB.LinkedDocumentID = &docB

	if _, err := mvts.Update(ctx, entA, repo.Owner(userA)); err != nil {
		t.Fatalf("Update A: %v", err)
	}
	if _, err := mvts.Update(ctx, entB, repo.Owner(userA)); err != nil {
		// Honest baseline: same as JSONB case — no error under present no-OCC.
		t.Fatalf("Update B (expected no error under present no-OCC baseline): %v", err)
	}

	final, err := mvts.Get(ctx, id, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get final: %v", err)
	}

	// --- PRESENT-behavior baseline assertions (no OCC) ---
	// Last writer (B) wins: final points at docB.
	if final.LinkedDocumentID == nil || *final.LinkedDocumentID != docB {
		var got string
		if final.LinkedDocumentID != nil {
			got = *final.LinkedDocumentID
		}
		t.Errorf("link: final.LinkedDocumentID = %v, want %s (last writer B wins)", got, docB)
	}
	// A's intended link (docA) is LOST — B's stale write overwrote it.
	if final.LinkedDocumentID != nil && *final.LinkedDocumentID == docA {
		t.Errorf("link: baseline no longer holds — final link = docA (OCC now in place; update this baseline for payload-link-model-guardrails)")
	}
}
