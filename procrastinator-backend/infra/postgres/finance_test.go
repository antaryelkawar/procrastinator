package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/infra/postgres"
)

const testSchemaFinance = "p_finance"

const financeMissingID = "00000000-0000-0000-0000-000000000000"

var (
	finOnce      sync.Once
	finPool      *pgxpool.Pool
	finAccounts  *postgres.AccountRepository
	finMovements *postgres.MovementRepository
	finBatches   *postgres.ImportBatchRepository
	finLines     *postgres.ImportLineRepository
	finDocs      *postgres.DocumentRepository
	finSources   repo.Repository[entity.Source]
	finAssets    repo.Repository[entity.Asset]
	finFactory   *repo.Factory
	finInitErr   error
)

// finRepos returns the finance repositories bound to the p_finance test
// schema. Uses the sync.Once lazy init pattern (see genericRepos): the pool,
// repositories, and factory are opened and initialized exactly once per test
// binary run.
func finRepos(t *testing.T) (*postgres.AccountRepository, *postgres.MovementRepository, *postgres.ImportBatchRepository, *postgres.ImportLineRepository) {
	t.Helper()
	if os.Getenv("TESTPG_SKIP") == "1" {
		t.Skip("no test database configured")
	}
	finOnce.Do(func() {
		finPool, finInitErr = openTestPool(t, testSchemaFinance)
		if finInitErr != nil {
			return
		}
		finAccounts = postgres.NewAccountRepository(finPool)
		finMovements = postgres.NewMovementRepository(finPool)
		finBatches = postgres.NewImportBatchRepository(finPool)
		finLines = postgres.NewImportLineRepository(finPool)
		finDocs = postgres.NewDocumentRepository(finPool)
		finSources = postgres.NewSourceRepository(finPool)
		finAssets = postgres.NewAssetRepository(finPool)
		finFactory = postgres.NewFactory(finPool)
	})
	if finInitErr != nil {
		t.Fatalf("init finance test pool: %v", finInitErr)
	}
	return finAccounts, finMovements, finBatches, finLines
}

// truncateFinance clears all finance (and dependent) test data on the pool.
// Call at the top of each write test.
func truncateFinance(t *testing.T) {
	t.Helper()
	finRepos(t) // ensure the pool is initialized
	if _, err := finPool.Exec(context.Background(),
		`TRUNCATE import_lines, import_batches, money_movements, financial_accounts, documents, sources, assets CASCADE`); err != nil {
		t.Fatalf("truncate finance: %v", err)
	}
}

// seedFinanceDocument creates a source, an asset (no serial number, to keep
// the partial unique index on norm_serial unconcerned), and a document
// linking them, all under the given user. Returns the created document.
func seedFinanceDocument(t *testing.T, OwnerID, filename, price, currency string) entity.Document {
	t.Helper()
	truncateFinance(t) // subtests share the schema; keep each case in isolation
	ctx := context.Background()
	src, err := finSources.Create(ctx, entity.Source{
		Filename:    filename,
		ContentType: "application/pdf",
		Size:        1024,
		Path:        "storage/" + filename,
		SHA256:      "aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999",
	}, repo.Owner(OwnerID))
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	asset, err := finAssets.Create(ctx, entity.Asset{
		Brand:   strPtr("Samsung"),
		DocType: entity.DocTypeInvoice,
	}, repo.Owner(OwnerID))
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	doc, err := finDocs.Create(ctx, entity.Document{
		SourceID:        src.ID,
		AssetID:         asset.ID,
		DocType:         entity.DocTypeInvoice,
		ExtractedFields: map[string]any{"price": price, "currency": currency},
		RawExtraction:   "raw fixture",
	}, repo.Owner(OwnerID))
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}
	return doc
}

// seedAccount creates a bank account under the given user.
func seedAccount(t *testing.T, OwnerID, name string) entity.FinancialAccount {
	t.Helper()
	acc, err := finAccounts.Create(context.Background(), entity.FinancialAccount{
		Name:     name,
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(OwnerID))
	if err != nil {
		t.Fatalf("seed account %s: %v", name, err)
	}
	return acc
}

func TestFinanceAccountRoundTrip(t *testing.T) {
	accounts, _, _, _ := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()

	created, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:               "Salary Account",
		Type:               entity.AccountTypeBank,
		Currency:           "EUR",
		Institution:        strPtr("Bank V"),
		ExternalDescriptor: strPtr("DE89 3704 0044 0532 0130 00"),
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Error("Create returned empty ID, want DB-generated uuid")
	}
	if created.OwnerID != userA {
		t.Errorf("OwnerID = %q, want %q", created.OwnerID, userA)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want DB default now()")
	}
	if created.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want DB default now()")
	}

	got, err := accounts.Get(ctx, created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.OwnerID != userA {
		t.Errorf("OwnerID = %q, want %q", got.OwnerID, userA)
	}
	if got.Name != "Salary Account" {
		t.Errorf("Name = %q, want %q", got.Name, "Salary Account")
	}
	if got.Type != entity.AccountTypeBank {
		t.Errorf("Type = %q, want %q", got.Type, entity.AccountTypeBank)
	}
	if got.Currency != "EUR" {
		t.Errorf("Currency = %q, want exact %q", got.Currency, "EUR")
	}
	assertPtrEqual(t, "Institution", got.Institution, strPtr("Bank V"))
	assertPtrEqual(t, "ExternalDescriptor", got.ExternalDescriptor, strPtr("DE89 3704 0044 0532 0130 00"))
	if !got.CreatedAt.IsZero() && !created.CreatedAt.IsZero() && !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created.CreatedAt)
	}
	if !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, created.UpdatedAt)
	}
}

func TestFinanceMovementRoundTrip(t *testing.T) {
	accounts, movements, _, _ := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()

	srcAcc, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:     "Source A",
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed source account: %v", err)
	}
	dstAcc, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:     "Dest B",
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed destination account: %v", err)
	}

	// Full manual transfer: NormDescription left EMPTY, must be derived on write.
	occurredOn := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	created, err := movements.Create(ctx, entity.MoneyMovement{
		Kind:                 entity.KindTransfer,
		Amount:               "19999.99",
		Currency:             "EUR",
		OccurredOn:           occurredOn,
		Description:          "  Coffee   Shop  ",
		Origin:               entity.OriginManual,
		SourceAccountID:      &srcAcc.ID,
		DestinationAccountID: &dstAcc.ID,
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := movements.Get(ctx, created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.OwnerID != userA {
		t.Errorf("OwnerID = %q, want %q", got.OwnerID, userA)
	}
	if got.Kind != entity.KindTransfer {
		t.Errorf("Kind = %q, want %q", got.Kind, entity.KindTransfer)
	}
	if got.Amount != "19999.99" {
		t.Errorf("Amount = %q, want exact %q", got.Amount, "19999.99")
	}
	if got.Currency != "EUR" {
		t.Errorf("Currency = %q, want %q", got.Currency, "EUR")
	}
	if !got.OccurredOn.Equal(occurredOn) {
		t.Errorf("OccurredOn = %v, want %v", got.OccurredOn, occurredOn)
	}
	if got.Description != "  Coffee   Shop  " {
		t.Errorf("Description = %q, want verbatim %q", got.Description, "  Coffee   Shop  ")
	}
	if got.NormDescription != "coffee shop" {
		t.Errorf("NormDescription = %q, want derived %q", got.NormDescription, "coffee shop")
	}
	if got.Origin != entity.OriginManual {
		t.Errorf("Origin = %q, want %q", got.Origin, entity.OriginManual)
	}
	assertPtrEqual(t, "SourceAccountID", got.SourceAccountID, &srcAcc.ID)
	assertPtrEqual(t, "DestinationAccountID", got.DestinationAccountID, &dstAcc.ID)
	assertPtrEqual(t, "ImportBatchID", got.ImportBatchID, nil)
	if got.ImportLine != nil {
		t.Errorf("ImportLine = %v, want nil", *got.ImportLine)
	}
	assertPtrEqual(t, "ExternalReference", got.ExternalReference, nil)
	assertPtrEqual(t, "LinkedDocumentID", got.LinkedDocumentID, nil)
	assertPtrEqual(t, "LinkCreator", got.LinkCreator, nil)
	if got.LinkConflicting {
		t.Error("LinkConflicting = true, want false")
	}

	// Second movement with all link fields set.
	importBatchID := "11111111-1111-1111-1111-111111111111"
	linkedDocID := "22222222-2222-2222-2222-222222222222"
	importLine := 3
	created2, err := movements.Create(ctx, entity.MoneyMovement{
		Kind:              entity.KindExpense,
		Amount:            "500.50",
		Currency:          "EUR",
		OccurredOn:        time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		Description:       "Linked Expense",
		NormDescription:   "linked expense",
		Origin:            entity.OriginImport,
		SourceAccountID:   &srcAcc.ID,
		ImportBatchID:     &importBatchID,
		ImportLine:        &importLine,
		ExternalReference: strPtr("REF-77"),
		LinkedDocumentID:  &linkedDocID,
		LinkCreator:       strPtr(entity.LinkCreatorManual),
		LinkConflicting:   true,
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create linked movement: %v", err)
	}

	got2, err := movements.Get(ctx, created2.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get linked movement: %v", err)
	}
	if got2.Kind != entity.KindExpense {
		t.Errorf("Kind = %q, want %q", got2.Kind, entity.KindExpense)
	}
	if got2.Amount != "500.50" {
		t.Errorf("Amount = %q, want %q", got2.Amount, "500.50")
	}
	if got2.NormDescription != "linked expense" {
		t.Errorf("NormDescription = %q, want %q", got2.NormDescription, "linked expense")
	}
	assertPtrEqual(t, "linked.SourceAccountID", got2.SourceAccountID, &srcAcc.ID)
	assertPtrEqual(t, "linked.DestinationAccountID", got2.DestinationAccountID, nil)
	assertPtrEqual(t, "linked.ImportBatchID", got2.ImportBatchID, &importBatchID)
	if got2.ImportLine == nil || *got2.ImportLine != 3 {
		t.Errorf("linked.ImportLine = %v, want 3", got2.ImportLine)
	}
	assertPtrEqual(t, "linked.ExternalReference", got2.ExternalReference, strPtr("REF-77"))
	assertPtrEqual(t, "linked.LinkedDocumentID", got2.LinkedDocumentID, &linkedDocID)
	assertPtrEqual(t, "linked.LinkCreator", got2.LinkCreator, strPtr(entity.LinkCreatorManual))
	if !got2.LinkConflicting {
		t.Error("linked.LinkConflicting = false, want true")
	}
}

func TestFinanceImportBatchRoundTrip(t *testing.T) {
	accounts, _, batches, _ := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()

	acc, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:     "Batch Account",
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	sources := postgres.NewSourceRepository(finPool)
	src, err := sources.Create(ctx, entity.Source{
		Filename:    "stmt.pdf",
		ContentType: "application/pdf",
		Size:        1024,
		Path:        "storage/batch-src.pdf",
		SHA256:      "aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}

	created, err := batches.Create(ctx, entity.ImportBatch{
		State:                entity.BatchStatePreview,
		AccountID:            acc.ID,
		SourceID:             src.ID,
		Filename:             "stmt.csv",
		Format:               "csv",
		LineCountValid:       2,
		LineCountDuplicate:   1,
		LineCountPossibleDup: 1,
		LineCountError:       1,
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Error("Create returned empty ID")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("timestamps zero, want DB defaults")
	}

	got, err := batches.Get(ctx, created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.OwnerID != userA {
		t.Errorf("OwnerID = %q, want %q", got.OwnerID, userA)
	}
	if got.State != entity.BatchStatePreview {
		t.Errorf("State = %q, want %q", got.State, entity.BatchStatePreview)
	}
	if got.AccountID != acc.ID {
		t.Errorf("AccountID = %q, want %q", got.AccountID, acc.ID)
	}
	if got.SourceID != src.ID {
		t.Errorf("SourceID = %q, want %q", got.SourceID, src.ID)
	}
	if got.Filename != "stmt.csv" {
		t.Errorf("Filename = %q, want %q", got.Filename, "stmt.csv")
	}
	if got.Format != "csv" {
		t.Errorf("Format = %q, want %q", got.Format, "csv")
	}
	if got.LineCountValid != 2 {
		t.Errorf("LineCountValid = %d, want 2", got.LineCountValid)
	}
	if got.LineCountDuplicate != 1 {
		t.Errorf("LineCountDuplicate = %d, want 1", got.LineCountDuplicate)
	}
	if got.LineCountPossibleDup != 1 {
		t.Errorf("LineCountPossibleDup = %d, want 1", got.LineCountPossibleDup)
	}
	if got.LineCountError != 1 {
		t.Errorf("LineCountError = %d, want 1", got.LineCountError)
	}
}

func TestFinanceImportLineRoundTrip(t *testing.T) {
	accounts, _, batches, lines := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()

	acc, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:     "Line Account",
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	sources := postgres.NewSourceRepository(finPool)
	src, err := sources.Create(ctx, entity.Source{
		Filename:    "lines.pdf",
		ContentType: "application/pdf",
		Size:        1024,
		Path:        "storage/lines-src.pdf",
		SHA256:      "aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	batch, err := batches.Create(ctx, entity.ImportBatch{
		State:     entity.BatchStatePreview,
		AccountID: acc.ID,
		SourceID:  src.ID,
		Filename:  "stmt.csv",
		Format:    "csv",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	// First line: all fields set, NormDescription left nil (derived on write).
	occurredOn := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	created, err := lines.Create(ctx, entity.ImportLine{
		BatchID:           batch.ID,
		LineRef:           3,
		RawLine:           "2026-08-20,-1250.50,Reliance Digital,REF-1",
		OccurredOn:        &occurredOn,
		Amount:            strPtr("1250.50"),
		Direction:         strPtr("out"),
		Description:       strPtr("  Reliance   Digital "),
		ExternalReference: strPtr("REF-1"),
		Status:            entity.LineStatusValid,
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := lines.Get(ctx, created.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.OwnerID != userA {
		t.Errorf("OwnerID = %q, want %q", got.OwnerID, userA)
	}
	if got.BatchID != batch.ID {
		t.Errorf("BatchID = %q, want %q", got.BatchID, batch.ID)
	}
	if got.LineRef != 3 {
		t.Errorf("LineRef = %d, want 3", got.LineRef)
	}
	if got.RawLine != "2026-08-20,-1250.50,Reliance Digital,REF-1" {
		t.Errorf("RawLine = %q, want %q", got.RawLine, "2026-08-20,-1250.50,Reliance Digital,REF-1")
	}
	assertTimePtrEqual(t, "OccurredOn", got.OccurredOn, &occurredOn)
	assertPtrEqual(t, "Amount", got.Amount, strPtr("1250.50"))
	if got.Direction == nil || *got.Direction != "out" {
		t.Errorf("Direction = %v, want %q", got.Direction, "out")
	}
	assertPtrEqual(t, "Description", got.Description, strPtr("  Reliance   Digital "))
	assertPtrEqual(t, "NormDescription", got.NormDescription, strPtr("reliance digital"))
	assertPtrEqual(t, "ExternalReference", got.ExternalReference, strPtr("REF-1"))
	if got.Status != entity.LineStatusValid {
		t.Errorf("Status = %q, want %q", got.Status, entity.LineStatusValid)
	}
	if got.ErrorReason != nil {
		t.Errorf("ErrorReason = %q, want nil", *got.ErrorReason)
	}

	// Second line: error status, nullable fields stay nil.
	created2, err := lines.Create(ctx, entity.ImportLine{
		BatchID:     batch.ID,
		LineRef:     4,
		RawLine:     "2026-08-21,?,unknown",
		Direction:   strPtr("in"),
		Status:      entity.LineStatusError,
		ErrorReason: strPtr("missing amount"),
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create error line: %v", err)
	}
	got2, err := lines.Get(ctx, created2.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Get error line: %v", err)
	}
	if got2.Status != entity.LineStatusError {
		t.Errorf("Status = %q, want %q", got2.Status, entity.LineStatusError)
	}
	assertPtrEqual(t, "err.ErrorReason", got2.ErrorReason, strPtr("missing amount"))
	if got2.Description != nil {
		t.Errorf("Description = %q, want nil", *got2.Description)
	}
	if got2.NormDescription != nil {
		t.Errorf("NormDescription = %q, want nil", *got2.NormDescription)
	}
	if got2.OccurredOn != nil {
		t.Errorf("OccurredOn = %v, want nil", got2.OccurredOn)
	}
	if got2.Amount != nil {
		t.Errorf("Amount = %q, want nil", *got2.Amount)
	}
	if got2.ExternalReference != nil {
		t.Errorf("ExternalReference = %q, want nil", *got2.ExternalReference)
	}
}

func TestFinanceGetDeleteNotFound(t *testing.T) {
	accounts, movements, batches, lines := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()

	cases := []struct {
		name string
		get  func(id string) error
	}{
		{"account", func(id string) error {
			_, err := accounts.Get(ctx, id, repo.Owner(userA))
			return err
		}},
		{"movement", func(id string) error {
			_, err := movements.Get(ctx, id, repo.Owner(userA))
			return err
		}},
		{"batch", func(id string) error {
			_, err := batches.Get(ctx, id, repo.Owner(userA))
			return err
		}},
		{"line", func(id string) error {
			_, err := lines.Get(ctx, id, repo.Owner(userA))
			return err
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.get(financeMissingID); !errors.Is(err, repo.ErrNotFound) {
				t.Errorf("Get(missing id): err = %v, want ErrNotFound", err)
			}
		})
	}

	t.Run("delete account", func(t *testing.T) {
		created, err := accounts.Create(ctx, entity.FinancialAccount{
			Name:     "Doomed",
			Type:     entity.AccountTypeWallet,
			Currency: "EUR",
		}, repo.Owner(userA))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := accounts.Delete(ctx, created.ID, repo.Owner(userA)); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := accounts.Get(ctx, created.ID, repo.Owner(userA)); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
		}
	})
}

func TestFinanceListStableOrderNonNil(t *testing.T) {
	accounts, movements, batches, lines := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()

	// Empty schema: List returns a non-nil, empty slice.
	t.Run("empty account", func(t *testing.T) {
		l, err := accounts.List(ctx, repo.Owner(userA))
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if l == nil {
			t.Error("List on empty schema = nil, want non-nil empty slice")
		}
		if len(l) != 0 {
			t.Errorf("List = %d rows, want 0", len(l))
		}
	})
	t.Run("empty movement", func(t *testing.T) {
		l, err := movements.List(ctx, repo.Owner(userA))
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if l == nil {
			t.Error("List on empty schema = nil, want non-nil empty slice")
		}
		if len(l) != 0 {
			t.Errorf("List = %d rows, want 0", len(l))
		}
	})
	t.Run("empty batch", func(t *testing.T) {
		l, err := batches.List(ctx, repo.Owner(userA))
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if l == nil {
			t.Error("List on empty schema = nil, want non-nil empty slice")
		}
		if len(l) != 0 {
			t.Errorf("List = %d rows, want 0", len(l))
		}
	})
	t.Run("empty line", func(t *testing.T) {
		l, err := lines.List(ctx, repo.Owner(userA))
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if l == nil {
			t.Error("List on empty schema = nil, want non-nil empty slice")
		}
		if len(l) != 0 {
			t.Errorf("List = %d rows, want 0", len(l))
		}
	})

	for i := 0; i < 3; i++ {
		if _, err := accounts.Create(ctx, entity.FinancialAccount{
			Name:     "Order Account",
			Type:     entity.AccountTypeBank,
			Currency: "EUR",
		}, repo.Owner(userA)); err != nil {
			t.Fatalf("Create account[%d]: %v", i, err)
		}
		time.Sleep(time.Millisecond) // distinct created_at values
	}

	got, err := accounts.List(ctx, repo.Owner(userA), repo.OrderBy("created_at, id"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List = %d rows, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].CreatedAt.After(got[i].CreatedAt) ||
			(got[i-1].CreatedAt.Equal(got[i].CreatedAt) && got[i-1].ID > got[i].ID) {
			t.Errorf("order violated at %d: (%v, %q) after (%v, %q)",
				i, got[i-1].CreatedAt, got[i-1].ID, got[i].CreatedAt, got[i].ID)
		}
	}
}

func TestFinanceUserScoping(t *testing.T) {
	accounts, movements, _, _ := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()

	// Account scoping.
	acc, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:     "Scoped Account",
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create account: %v", err)
	}
	if _, err := accounts.Get(ctx, acc.ID, repo.Owner(userB)); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("account Get(other user): err = %v, want ErrNotFound", err)
	}
	list, err := accounts.List(ctx, repo.Owner(userB))
	if err != nil {
		t.Fatalf("account List(other user): %v", err)
	}
	if len(list) != 0 {
		t.Errorf("account List(other user) = %d rows, want 0", len(list))
	}

	// Movement scoping: seed an expense movement under userA.
	occurredOn := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	mov, err := movements.Create(ctx, entity.MoneyMovement{
		Kind:            entity.KindExpense,
		Amount:          "10.00",
		Currency:        "EUR",
		OccurredOn:      occurredOn,
		Description:     "Scoped",
		Origin:          entity.OriginManual,
		SourceAccountID: &acc.ID,
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("Create movement: %v", err)
	}
	if _, err := movements.Get(ctx, mov.ID, repo.Owner(userB)); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("movement Get(other user): err = %v, want ErrNotFound", err)
	}
	movList, err := movements.List(ctx, repo.Owner(userB))
	if err != nil {
		t.Fatalf("movement List(other user): %v", err)
	}
	if len(movList) != 0 {
		t.Errorf("movement List(other user) = %d rows, want 0", len(movList))
	}
}

func TestFinanceNoUserErr(t *testing.T) {
	accounts, movements, batches, lines := finRepos(t)
	truncateFinance(t)
	ctx := context.Background() // no user

	if _, err := accounts.Create(ctx, entity.FinancialAccount{
		Name: "X", Type: entity.AccountTypeBank, Currency: "EUR",
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("account Create: err = %v, want ErrNoUser", err)
	}
	if _, err := accounts.Get(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("account Get: err = %v, want ErrNoUser", err)
	}
	if _, err := accounts.List(ctx); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("account List: err = %v, want ErrNoUser", err)
	}
	if _, err := accounts.Update(ctx, entity.FinancialAccount{
		ID: financeMissingID, Name: "X", Type: entity.AccountTypeBank, Currency: "EUR",
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("account Update: err = %v, want ErrNoUser", err)
	}
	if err := accounts.Delete(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("account Delete: err = %v, want ErrNoUser", err)
	}

	if _, err := movements.Create(ctx, entity.MoneyMovement{
		Kind: entity.KindExpense, Amount: "1", Currency: "EUR",
		OccurredOn: time.Now(), Description: "d", Origin: entity.OriginManual,
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("movement Create: err = %v, want ErrNoUser", err)
	}
	if _, err := movements.Get(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("movement Get: err = %v, want ErrNoUser", err)
	}
	if _, err := movements.List(ctx); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("movement List: err = %v, want ErrNoUser", err)
	}
	if _, err := movements.Update(ctx, entity.MoneyMovement{
		ID: financeMissingID, Kind: entity.KindExpense, Amount: "1", Currency: "EUR",
		OccurredOn: time.Now(), Description: "d", Origin: entity.OriginManual,
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("movement Update: err = %v, want ErrNoUser", err)
	}
	if err := movements.Delete(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("movement Delete: err = %v, want ErrNoUser", err)
	}

	if _, err := batches.Create(ctx, entity.ImportBatch{
		State: entity.BatchStatePreview, AccountID: financeMissingID,
		SourceID: financeMissingID, Filename: "f", Format: "csv",
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("batch Create: err = %v, want ErrNoUser", err)
	}
	if _, err := batches.Get(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("batch Get: err = %v, want ErrNoUser", err)
	}
	if _, err := batches.List(ctx); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("batch List: err = %v, want ErrNoUser", err)
	}
	if _, err := batches.Update(ctx, entity.ImportBatch{
		ID: financeMissingID, State: entity.BatchStatePreview,
		AccountID: financeMissingID, SourceID: financeMissingID, Filename: "f", Format: "csv",
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("batch Update: err = %v, want ErrNoUser", err)
	}
	if err := batches.Delete(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("batch Delete: err = %v, want ErrNoUser", err)
	}

	if _, err := lines.Create(ctx, entity.ImportLine{
		BatchID: financeMissingID, LineRef: 1, RawLine: "raw", Status: entity.LineStatusValid,
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("line Create: err = %v, want ErrNoUser", err)
	}
	if _, err := lines.Get(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("line Get: err = %v, want ErrNoUser", err)
	}
	if _, err := lines.List(ctx); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("line List: err = %v, want ErrNoUser", err)
	}
	if _, err := lines.Update(ctx, entity.ImportLine{
		ID: financeMissingID, BatchID: financeMissingID, LineRef: 1,
		RawLine: "raw", Status: entity.LineStatusValid,
	}); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("line Update: err = %v, want ErrNoUser", err)
	}
	if err := lines.Delete(ctx, financeMissingID); !errors.Is(err, user.ErrNoUser) {
		t.Errorf("line Delete: err = %v, want ErrNoUser", err)
	}
}

func TestFinanceBalanceForAccount(t *testing.T) {
	_, movements, _, _ := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()
	occurredOn := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

	type movementSeed struct {
		userID      string
		kind        string
		amount      string
		source      *string
		destination *string
	}
	addMovement := func(s movementSeed) {
		t.Helper()
		if _, err := movements.Create(ctx, entity.MoneyMovement{
			Kind:                 s.kind,
			Amount:               s.amount,
			Currency:             "EUR",
			OccurredOn:           occurredOn,
			Description:          "Balance fixture",
			Origin:               entity.OriginManual,
			SourceAccountID:      s.source,
			DestinationAccountID: s.destination,
		}, repo.Owner(s.userID)); err != nil {
			t.Fatalf("seed movement (%s): %v", s.kind, err)
		}
	}

	t.Run("no movements", func(t *testing.T) {
		acc := seedAccount(t, userA, "Empty")
		got, err := movements.BalanceForAccount(ctx, acc.ID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("BalanceForAccount: %v", err)
		}
		if got != "0" {
			t.Errorf("balance = %q, want %q", got, "0")
		}
	})

	t.Run("income adds", func(t *testing.T) {
		acc := seedAccount(t, userA, "Income")
		addMovement(movementSeed{userID: userA, kind: entity.KindIncome, amount: "19999.99", destination: &acc.ID})
		got, err := movements.BalanceForAccount(ctx, acc.ID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("BalanceForAccount: %v", err)
		}
		if got != "19999.99" {
			t.Errorf("balance = %q, want exact %q", got, "19999.99")
		}
	})

	t.Run("expense subtracts", func(t *testing.T) {
		acc := seedAccount(t, userA, "Expense")
		addMovement(movementSeed{userID: userA, kind: entity.KindExpense, amount: "500.50", source: &acc.ID})
		got, err := movements.BalanceForAccount(ctx, acc.ID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("BalanceForAccount: %v", err)
		}
		if got != "-500.50" {
			t.Errorf("balance = %q, want exact %q", got, "-500.50")
		}
	})

	t.Run("transfer in adds / transfer out subtracts", func(t *testing.T) {
		a := seedAccount(t, userA, "Transfer A")
		b := seedAccount(t, userA, "Transfer B")
		// A's balance = in - out, where "in" means the movement that has A as
		// destination and "out" the one that has A as source.
		addMovement(movementSeed{userID: userA, kind: entity.KindTransfer, amount: "100.01", source: &b.ID, destination: &a.ID}) // in to A
		addMovement(movementSeed{userID: userA, kind: entity.KindTransfer, amount: "200.00", source: &a.ID, destination: &b.ID}) // out of A
		got, err := movements.BalanceForAccount(ctx, a.ID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("BalanceForAccount: %v", err)
		}
		if got != "-99.99" {
			t.Errorf("balance = %q, want exact %q", got, "-99.99")
		}
	})

	t.Run("combined", func(t *testing.T) {
		a := seedAccount(t, userA, "Combined A")
		b := seedAccount(t, userA, "Combined B")
		addMovement(movementSeed{userID: userA, kind: entity.KindIncome, amount: "19999.99", destination: &a.ID})
		addMovement(movementSeed{userID: userA, kind: entity.KindExpense, amount: "500.50", source: &a.ID})
		addMovement(movementSeed{userID: userA, kind: entity.KindTransfer, amount: "100.01", source: &b.ID, destination: &a.ID})
		addMovement(movementSeed{userID: userA, kind: entity.KindTransfer, amount: "200.00", source: &a.ID, destination: &b.ID})
		got, err := movements.BalanceForAccount(ctx, a.ID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("BalanceForAccount: %v", err)
		}
		if got != "19399.50" {
			t.Errorf("balance = %q, want exact %q", got, "19399.50")
		}
	})

	t.Run("foreign user excluded", func(t *testing.T) {
		accA := seedAccount(t, userA, "Foreign A")
		accB := seedAccount(t, userB, "Foreign B")
		addMovement(movementSeed{userID: userB, kind: entity.KindIncome, amount: "7777.77", destination: &accB.ID})
		got, err := movements.BalanceForAccount(ctx, accA.ID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("BalanceForAccount: %v", err)
		}
		if got != "0" {
			t.Errorf("balance = %q, want %q (userB movement must not count)", got, "0")
		}
	})
}

func TestFinanceMovementsForAccount(t *testing.T) {
	accounts, movements, _, _ := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()
	occurredOn := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

	accA, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:     "MFA A",
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed account A: %v", err)
	}
	accB, err := accounts.Create(ctx, entity.FinancialAccount{
		Name:     "MFA B",
		Type:     entity.AccountTypeBank,
		Currency: "EUR",
	}, repo.Owner(userA))
	if err != nil {
		t.Fatalf("seed account B: %v", err)
	}

	mk := func(kind string, source, destination *string) entity.MoneyMovement {
		created, err := movements.Create(ctx, entity.MoneyMovement{
			Kind:                 kind,
			Amount:               "10.00",
			Currency:             "EUR",
			OccurredOn:           occurredOn,
			Description:          "MFA fixture",
			Origin:               entity.OriginManual,
			SourceAccountID:      source,
			DestinationAccountID: destination,
		}, repo.Owner(userA))
		if err != nil {
			t.Fatalf("seed movement %s: %v", kind, err)
		}
		return created
	}

	expA := mk(entity.KindExpense, &accA.ID, nil) // in scope for A
	incA := mk(entity.KindIncome, nil, &accA.ID)  // in scope for A
	expB := mk(entity.KindExpense, &accB.ID, nil) // out of scope for A, in scope for B

	assertIDs := func(t *testing.T, name string, got []entity.MoneyMovement, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %d movements, want %d", name, len(got), len(want))
		}
		seen := map[string]bool{}
		for _, m := range got {
			seen[m.ID] = true
		}
		for _, w := range want {
			if !seen[w] {
				t.Errorf("%s: missing movement %q", name, w)
			}
		}
	}

	got, err := movements.MovementsForAccount(ctx, accA.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("MovementsForAccount(A): %v", err)
	}
	assertIDs(t, "A", got, expA.ID, incA.ID)

	gotB, err := movements.MovementsForAccount(ctx, accB.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("MovementsForAccount(B): %v", err)
	}
	assertIDs(t, "B", gotB, expB.ID)

	emptyAcc := seedAccount(t, userA, "MFA Empty")
	gotEmpty, err := movements.MovementsForAccount(ctx, emptyAcc.ID, repo.Owner(userA))
	if err != nil {
		t.Fatalf("MovementsForAccount(empty): %v", err)
	}
	if gotEmpty == nil {
		t.Error("MovementsForAccount(empty) = nil, want non-nil empty slice")
	}
	if len(gotEmpty) != 0 {
		t.Errorf("MovementsForAccount(empty) = %d rows, want 0", len(gotEmpty))
	}

	gotOther, err := movements.MovementsForAccount(ctx, accA.ID, repo.Owner(userB))
	if err != nil {
		t.Fatalf("MovementsForAccount(A, userB): %v", err)
	}
	if gotOther == nil {
		t.Error("MovementsForAccount(A, userB) = nil, want non-nil empty slice")
	}
	if len(gotOther) != 0 {
		t.Errorf("MovementsForAccount(A, userB) = %d rows, want 0 (user-scoped)", len(gotOther))
	}
}

func TestFinanceLinkCandidates(t *testing.T) {
	_, movements, _, _ := finRepos(t)
	truncateFinance(t)
	ctx := context.Background()
	occurredOn := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

	t.Run("exact match returns the doc", func(t *testing.T) {
		doc := seedFinanceDocument(t, userA, "match.pdf", "19999.99", "EUR")
		got, err := finDocs.LinkCandidates(ctx, "19999.99", "EUR", repo.Owner(userA))
		if err != nil {
			t.Fatalf("LinkCandidates: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("LinkCandidates = %d docs, want 1", len(got))
		}
		if got[0].ID != doc.ID {
			t.Errorf("doc ID = %q, want %q", got[0].ID, doc.ID)
		}
	})

	t.Run("no match is empty", func(t *testing.T) {
		doc := seedFinanceDocument(t, userA, "nomatch.pdf", "19999.99", "EUR")
		_ = doc

		got, err := finDocs.LinkCandidates(ctx, "1.00", "EUR", repo.Owner(userA))
		if err != nil {
			t.Fatalf("LinkCandidates(price mismatch): %v", err)
		}
		if got == nil {
			t.Error("LinkCandidates(price mismatch) = nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("LinkCandidates(price mismatch) = %d docs, want 0", len(got))
		}

		got, err = finDocs.LinkCandidates(ctx, "19999.99", "USD", repo.Owner(userA))
		if err != nil {
			t.Fatalf("LinkCandidates(currency mismatch): %v", err)
		}
		if got == nil {
			t.Error("LinkCandidates(currency mismatch) = nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("LinkCandidates(currency mismatch) = %d docs, want 0", len(got))
		}
	})

	t.Run("already-linked doc excluded", func(t *testing.T) {
		doc := seedFinanceDocument(t, userA, "linked.pdf", "19999.99", "EUR")
		acc := seedAccount(t, userA, "Linked")
		if _, err := movements.Create(ctx, entity.MoneyMovement{
			Kind:             entity.KindExpense,
			Amount:           "19999.99",
			Currency:         "EUR",
			OccurredOn:       occurredOn,
			Description:      "Linked expense",
			Origin:           entity.OriginManual,
			SourceAccountID:  &acc.ID,
			LinkedDocumentID: &doc.ID,
		}, repo.Owner(userA)); err != nil {
			t.Fatalf("seed linked movement: %v", err)
		}
		got, err := finDocs.LinkCandidates(ctx, "19999.99", "EUR", repo.Owner(userA))
		if err != nil {
			t.Fatalf("LinkCandidates: %v", err)
		}
		if got == nil {
			t.Error("LinkCandidates = nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("LinkCandidates = %d docs, want 0 (already linked)", len(got))
		}
	})

	t.Run("foreign user invisible", func(t *testing.T) {
		// Distinct price so this subtest's query cannot see docs left behind
		// by the earlier subtests (the schema is only truncated once, at the
		// top of the test).
		seedFinanceDocument(t, userB, "foreign.pdf", "8888.88", "EUR")
		got, err := finDocs.LinkCandidates(ctx, "19999.99", "EUR", repo.Owner(userA))
		if err != nil {
			t.Fatalf("LinkCandidates: %v", err)
		}
		if got == nil {
			t.Error("LinkCandidates = nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("LinkCandidates = %d docs, want 0 (foreign user must not appear)", len(got))
		}
	})
}

func TestFinanceFactoryInTx(t *testing.T) {
	finRepos(t) // ensure the pool and factory are initialized
	truncateFinance(t)
	ctx := context.Background()
	occurredOn := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

	t.Run("tx-bound finance repos see uncommitted writes", func(t *testing.T) {
		var accountID string
		var txMovements *postgres.MovementRepository
		err := finFactory.InTx(ctx, func(ctx context.Context, repos *repo.Repos) (retErr error) {
			account, err := repos.Accounts.Create(ctx, entity.FinancialAccount{
				Name:     "Tx Account",
				Type:     entity.AccountTypeBank,
				Currency: "EUR",
			}, repo.Owner(userA))
			if err != nil {
				return err
			}
			accountID = account.ID
			var ok bool
			txMovements, ok = repos.Movements.(*postgres.MovementRepository)
			if !ok {
				return fmt.Errorf("repos.Movements = %T, want *postgres.MovementRepository", repos.Movements)
			}
			if _, err := repos.Movements.Create(ctx, entity.MoneyMovement{
				Kind:                 entity.KindIncome,
				Amount:               "100.00",
				Currency:             "EUR",
				OccurredOn:           occurredOn,
				Description:          "Tx income",
				Origin:               entity.OriginManual,
				DestinationAccountID: &account.ID,
			}, repo.Owner(userA)); err != nil {
				return err
			}

			// The tx sees its own uncommitted writes...
			txBalance, err := txMovements.BalanceForAccount(ctx, account.ID, repo.Owner(userA))
			if err != nil {
				return err
			}
			if txBalance != "100.00" {
				t.Errorf("tx-bound BalanceForAccount = %q, want %q (uncommitted write visible in tx)", txBalance, "100.00")
			}

			// ...but the separate pool connection must NOT see them.
			poolBalance, err := finMovements.BalanceForAccount(ctx, account.ID, repo.Owner(userA))
			if err != nil {
				return err
			}
			if poolBalance != "0" {
				t.Errorf("pool-bound BalanceForAccount = %q, want %q (uncommitted tx writes must not leak to the pool)", poolBalance, "0")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("InTx: %v", err)
		}

		// After commit, the pool-bound repo sees the committed movement.
		committed, err := finMovements.BalanceForAccount(ctx, accountID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("BalanceForAccount after commit: %v", err)
		}
		if committed != "100.00" {
			t.Errorf("BalanceForAccount after commit = %q, want %q", committed, "100.00")
		}
	})

	t.Run("commits on success", func(t *testing.T) {
		truncateFinance(t)

		var createdID string
		err := finFactory.InTx(ctx, func(ctx context.Context, repos *repo.Repos) error {
			var err error
			var created entity.FinancialAccount
			created, err = repos.Accounts.Create(ctx, entity.FinancialAccount{
				Name:     "Committed Account",
				Type:     entity.AccountTypeBank,
				Currency: "EUR",
			}, repo.Owner(userA))
			if err != nil {
				return err
			}
			createdID = created.ID
			return nil
		})
		if err != nil {
			t.Fatalf("InTx: %v", err)
		}

		// Committed: visible after the transaction via the pool-bound repo.
		got, err := finAccounts.Get(ctx, createdID, repo.Owner(userA))
		if err != nil {
			t.Fatalf("Get after commit: %v, want visible row", err)
		}
		if got.ID != createdID {
			t.Errorf("Get ID = %q, want %q", got.ID, createdID)
		}
	})

	t.Run("rolls back on error", func(t *testing.T) {
		truncateFinance(t)

		forcedErr := errors.New("forced rollback")
		err := finFactory.InTx(ctx, func(ctx context.Context, repos *repo.Repos) error {
			account, err := repos.Accounts.Create(ctx, entity.FinancialAccount{
				Name:     "Doomed Account",
				Type:     entity.AccountTypeBank,
				Currency: "EUR",
			}, repo.Owner(userA))
			if err != nil {
				return err
			}
			if _, err := repos.Movements.Create(ctx, entity.MoneyMovement{
				Kind:                 entity.KindIncome,
				Amount:               "10.00",
				Currency:             "EUR",
				OccurredOn:           occurredOn,
				Description:          "Doomed income",
				Origin:               entity.OriginManual,
				DestinationAccountID: &account.ID,
			}, repo.Owner(userA)); err != nil {
				return err
			}
			return forcedErr
		})
		if !errors.Is(err, forcedErr) {
			t.Fatalf("InTx: err = %v, want %q", err, forcedErr)
		}

		// Rolled back: no rows visible after the transaction.
		accounts, err := finAccounts.List(ctx, repo.Owner(userA))
		if err != nil {
			t.Fatalf("List accounts after rollback: %v", err)
		}
		if len(accounts) != 0 {
			t.Errorf("account List after rollback = %d rows, want 0 (rolled back)", len(accounts))
		}
		movs, err := finMovements.List(ctx, repo.Owner(userA))
		if err != nil {
			t.Fatalf("List movements after rollback: %v", err)
		}
		if len(movs) != 0 {
			t.Errorf("movement List after rollback = %d rows, want 0 (rolled back)", len(movs))
		}
	})
}
