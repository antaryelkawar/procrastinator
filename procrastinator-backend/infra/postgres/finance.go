package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// Compile-time guards: each concrete repository satisfies the generic
// repository interface for its entity.
var (
	_ repo.Repository[entity.FinancialAccount] = (*AccountRepository)(nil)
	_ repo.Repository[entity.MoneyMovement]    = (*MovementRepository)(nil)
	_ repo.Repository[entity.ImportBatch]      = (*ImportBatchRepository)(nil)
	_ repo.Repository[entity.ImportLine]       = (*ImportLineRepository)(nil)
)

// AccountRepository is the generic repository engine for entity.FinancialAccount.
type AccountRepository struct {
	*pgRepository[entity.FinancialAccount]
}

// MovementRepository is the generic repository engine for entity.MoneyMovement.
type MovementRepository struct {
	*pgRepository[entity.MoneyMovement]
}

// ImportBatchRepository is the generic repository engine for entity.ImportBatch.
type ImportBatchRepository struct {
	*pgRepository[entity.ImportBatch]
}

// ImportLineRepository is the generic repository engine for entity.ImportLine.
type ImportLineRepository struct {
	*pgRepository[entity.ImportLine]
}

// SearchAccounts returns the user's financial accounts whose name,
// account_type, or institution case-insensitively contain the (pre-escaped)
// ILIKE pattern, ordered by created_at DESC, id ASC. It runs in a scope-bound
// transaction (app.user_id RLS backstop), applies the D-8 visibility rule, and
// returns a non-nil empty slice when nothing matches.
func (r *AccountRepository) SearchAccounts(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.FinancialAccount, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}

	var result []entity.FinancialAccount
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		args = append(args, pattern)
		patN := len(args)
		stmt := fmt.Sprintf(
			"SELECT * FROM financial_accounts WHERE %s AND (name ILIKE $%d OR account_type ILIKE $%d OR institution ILIKE $%d) ORDER BY created_at DESC, id ASC",
			vis, patN, patN, patN)

		rows, err := q.Query(ctx, stmt, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.FinancialAccount, 0)
		for rows.Next() {
			item, err := scanFinancialAccount(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

// SearchMovements returns the user's money movements whose description or
// external_reference case-insensitively contain the (pre-escaped) ILIKE
// pattern, ordered by created_at DESC, id ASC. It runs in a scope-bound
// transaction (app.user_id RLS backstop), applies the D-8 visibility rule, and
// returns a non-nil empty slice when nothing matches.
func (r *MovementRepository) SearchMovements(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.MoneyMovement, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}

	var result []entity.MoneyMovement
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		args = append(args, pattern)
		patN := len(args)
		stmt := fmt.Sprintf(
			"SELECT * FROM money_movements WHERE %s AND (description ILIKE $%d OR external_reference ILIKE $%d) ORDER BY created_at DESC, id ASC",
			vis, patN, patN)

		rows, err := q.Query(ctx, stmt, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.MoneyMovement, 0)
		for rows.Next() {
			item, err := scanMoneyMovement(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

// SearchImportBatches returns the user's import batches whose filename
// case-insensitively contains the (pre-escaped) ILIKE pattern, ordered by
// created_at DESC, id ASC. It runs in a scope-bound transaction (app.user_id
// RLS backstop), applies the D-8 visibility rule, and returns a non-nil empty
// slice when nothing matches.
func (r *ImportBatchRepository) SearchImportBatches(ctx context.Context, pattern string, opts ...repo.Option) ([]entity.ImportBatch, error) {
	o := repo.ApplyOptions(opts...)

	tid, err := resolveOwner(ctx, o)
	if err != nil {
		return nil, err
	}

	var result []entity.ImportBatch
	err = r.scope.run(ctx, tid, func(q Querier) error {
		var args []any
		vis, err := visibilityCond(ctx, q, tid, true, "", &args)
		if err != nil {
			return err
		}
		args = append(args, pattern)
		patN := len(args)
		stmt := fmt.Sprintf(
			"SELECT * FROM import_batches WHERE %s AND filename ILIKE $%d ORDER BY created_at DESC, id ASC",
			vis, patN)

		rows, err := q.Query(ctx, stmt, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result = make([]entity.ImportBatch, 0)
		for rows.Next() {
			item, err := scanImportBatch(rows)
			if err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

// NewAccountRepository returns a repository for entity.FinancialAccount.
func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{
		pgRepository: &pgRepository[entity.FinancialAccount]{
			scope:     &poolScope{pool: pool},
			table:     "financial_accounts",
			scanRow:   scanFinancialAccount,
			toMap:     financialAccountToMap,
			filters:   accountFilters,
			shareable: true,
		},
	}
}

// NewMovementRepository returns a repository for entity.MoneyMovement.
func NewMovementRepository(pool *pgxpool.Pool) *MovementRepository {
	return &MovementRepository{
		pgRepository: &pgRepository[entity.MoneyMovement]{
			scope:     &poolScope{pool: pool},
			table:     "money_movements",
			scanRow:   scanMoneyMovement,
			toMap:     moneyMovementToMap,
			filters:   movementFilters,
			shareable: true,
		},
	}
}

// NewImportBatchRepository returns a repository for entity.ImportBatch.
func NewImportBatchRepository(pool *pgxpool.Pool) *ImportBatchRepository {
	return &ImportBatchRepository{
		pgRepository: &pgRepository[entity.ImportBatch]{
			scope:     &poolScope{pool: pool},
			table:     "import_batches",
			scanRow:   scanImportBatch,
			toMap:     importBatchToMap,
			filters:   importBatchFilters,
			shareable: true,
		},
	}
}

// NewImportLineRepository returns a repository for entity.ImportLine.
func NewImportLineRepository(pool *pgxpool.Pool) *ImportLineRepository {
	return &ImportLineRepository{
		pgRepository: &pgRepository[entity.ImportLine]{
			scope:     &poolScope{pool: pool},
			table:     "import_lines",
			scanRow:   scanImportLine,
			toMap:     importLineToMap,
			filters:   importLineFilters,
			shareable: true,
		},
	}
}

var accountFieldCols = map[string]string{
	"name":                "name",
	"account_type":        "account_type",
	"currency":            "currency",
	"institution":         "institution",
	"external_descriptor": "external_descriptor",
	"created_at":          "created_at",
	"updated_at":          "updated_at",
}

var accountOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"name":       "name",
}

var movementFieldCols = map[string]string{
	"kind":                   "kind",
	"amount":                 "amount",
	"currency":               "currency",
	"occurred_on":            "occurred_on",
	"recorded_at":            "recorded_at",
	"description":            "description",
	"norm_description":       "norm_description",
	"origin":                 "origin",
	"source_account_id":      "source_account_id",
	"destination_account_id": "destination_account_id",
	"import_batch_id":        "import_batch_id",
	"external_reference":     "external_reference",
	"linked_document_id":     "linked_document_id",
	"created_at":             "created_at",
	"updated_at":             "updated_at",
}

var movementOrderCols = map[string]string{
	"id":          "id",
	"created_at":  "created_at",
	"updated_at":  "updated_at",
	"occurred_on": "occurred_on",
	"kind":        "kind",
	"origin":      "origin",
}

var importBatchFieldCols = map[string]string{
	"state":                "state",
	"account_id":           "account_id",
	"source_id":            "source_id",
	"filename":             "filename",
	"format":               "format",
	"line_count_valid":     "line_count_valid",
	"line_count_duplicate": "line_count_duplicate",
	"created_at":           "created_at",
	"updated_at":           "updated_at",
}

var importBatchOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"state":      "state",
}

var importLineFieldCols = map[string]string{
	"batch_id":           "batch_id",
	"line_ref":           "line_ref",
	"raw_line":           "raw_line",
	"occurred_on":        "occurred_on",
	"amount":             "amount",
	"direction":          "direction",
	"description":        "description",
	"norm_description":   "norm_description",
	"external_reference": "external_reference",
	"status":             "status",
	"created_at":         "created_at",
}

var importLineOrderCols = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"batch_id":   "batch_id",
	"line_ref":   "line_ref",
	"status":     "status",
}

var (
	accountFilters     = filterConfig{fieldCols: accountFieldCols, orderCols: accountOrderCols}
	movementFilters    = filterConfig{fieldCols: movementFieldCols, orderCols: movementOrderCols}
	importBatchFilters = filterConfig{fieldCols: importBatchFieldCols, orderCols: importBatchOrderCols}
	importLineFilters  = filterConfig{fieldCols: importLineFieldCols, orderCols: importLineOrderCols}
)

// scanFinancialAccount scans a row into an entity.FinancialAccount,
// mapping pgx.ErrNoRows to repo.ErrNotFound. Column order matches the
// financial_accounts table (migration 00004_finance).
func scanFinancialAccount(row rowScanner) (entity.FinancialAccount, error) {
	var a entity.FinancialAccount
	var id string
	err := row.Scan(
		&id, &a.OwnerID, &a.Name, &a.Type, &a.Currency,
		&a.Institution, &a.ExternalDescriptor, &a.CreatedAt, &a.UpdatedAt,
		&a.OwnerHouseholdID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.FinancialAccount{}, repo.ErrNotFound
		}
		return entity.FinancialAccount{}, err
	}
	a.ID = id
	return a, nil
}

// scanMoneyMovement scans a row into an entity.MoneyMovement,
// mapping pgx.ErrNoRows to repo.ErrNotFound. Column order matches the
// money_movements table (migration 00004_finance).
func scanMoneyMovement(row rowScanner) (entity.MoneyMovement, error) {
	var m entity.MoneyMovement
	var id string
	err := row.Scan(
		&id, &m.OwnerID, &m.Kind, &m.Amount, &m.Currency,
		&m.OccurredOn, &m.RecordedAt, &m.Description, &m.NormDescription, &m.Origin,
		&m.SourceAccountID, &m.DestinationAccountID, &m.ImportBatchID, &m.ImportLine,
		&m.ExternalReference, &m.LinkedDocumentID, &m.LinkCreator, &m.LinkConflicting,
		&m.CreatedAt, &m.UpdatedAt, &m.OwnerHouseholdID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.MoneyMovement{}, repo.ErrNotFound
		}
		return entity.MoneyMovement{}, err
	}
	m.ID = id
	return m, nil
}

// scanImportBatch scans a row into an entity.ImportBatch,
// mapping pgx.ErrNoRows to repo.ErrNotFound. Column order matches the
// import_batches table (migration 00004_finance).
func scanImportBatch(row rowScanner) (entity.ImportBatch, error) {
	var b entity.ImportBatch
	var id string
	err := row.Scan(
		&id, &b.OwnerID, &b.State, &b.AccountID, &b.SourceID, &b.Filename, &b.Format,
		&b.LineCountValid, &b.LineCountDuplicate, &b.LineCountPossibleDup, &b.LineCountError,
		&b.CreatedAt, &b.UpdatedAt, &b.OwnerHouseholdID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.ImportBatch{}, repo.ErrNotFound
		}
		return entity.ImportBatch{}, err
	}
	b.ID = id
	return b, nil
}

// scanImportLine scans a row into an entity.ImportLine,
// mapping pgx.ErrNoRows to repo.ErrNotFound. Column order matches the
// import_lines table (migration 00004_finance).
func scanImportLine(row rowScanner) (entity.ImportLine, error) {
	var l entity.ImportLine
	var id string
	err := row.Scan(
		&id, &l.OwnerID, &l.BatchID, &l.LineRef, &l.RawLine,
		&l.OccurredOn, &l.Amount, &l.Direction, &l.Description, &l.NormDescription,
		&l.ExternalReference, &l.Status, &l.ErrorReason, &l.CreatedAt, &l.OwnerHouseholdID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.ImportLine{}, repo.ErrNotFound
		}
		return entity.ImportLine{}, err
	}
	l.ID = id
	return l, nil
}

func financialAccountToMap(a entity.FinancialAccount) map[string]any {
	m := make(map[string]any)
	if a.ID != "" {
		m["id"] = a.ID
	}
	if a.OwnerID != "" {
		m["owner_id"] = a.OwnerID
	}
	if a.Name != "" {
		m["name"] = a.Name
	}
	if a.Type != "" {
		m["account_type"] = a.Type
	}
	if a.Currency != "" {
		m["currency"] = a.Currency
	}
	if a.Institution != nil {
		m["institution"] = a.Institution
	}
	if a.ExternalDescriptor != nil {
		m["external_descriptor"] = a.ExternalDescriptor
	}
	if a.OwnerHouseholdID != nil {
		m["owner_household_id"] = a.OwnerHouseholdID
	}
	return m
}

func moneyMovementToMap(m entity.MoneyMovement) map[string]any {
	out := make(map[string]any)
	if m.ID != "" {
		out["id"] = m.ID
	}
	if m.OwnerID != "" {
		out["owner_id"] = m.OwnerID
	}
	if m.Kind != "" {
		out["kind"] = m.Kind
	}
	if m.Amount != "" {
		// Exact-decimal money string; passes through as-is, never a float.
		out["amount"] = m.Amount
	}
	if m.Currency != "" {
		out["currency"] = m.Currency
	}
	if !m.OccurredOn.IsZero() {
		out["occurred_on"] = m.OccurredOn
	}
	if !m.RecordedAt.IsZero() {
		out["recorded_at"] = m.RecordedAt
	}
	if m.Description != "" {
		out["description"] = m.Description
		if m.NormDescription != "" {
			out["norm_description"] = m.NormDescription
		} else {
			out["norm_description"] = commons.NormalizeDescription(m.Description)
		}
	}
	if m.Origin != "" {
		out["origin"] = m.Origin
	}
	if m.ID != "" {
		// Full fetched entity being Updated: include the import-provenance and
		// document-link columns AS-IS so that nil pointers map to SQL NULL and
		// a false LinkConflicting is written verbatim. The generic Update only
		// SETs columns present in this map, so without this an Unlink operation
		// (which nils LinkedDocumentID/LinkCreator and sets LinkConflicting false)
		// could not clear the link columns. The no-ID (Create) path keeps the
		// non-nil-only inclusion below.
		out["source_account_id"] = m.SourceAccountID
		out["destination_account_id"] = m.DestinationAccountID
		out["import_batch_id"] = m.ImportBatchID
		out["import_line"] = m.ImportLine
		out["external_reference"] = m.ExternalReference
		out["linked_document_id"] = m.LinkedDocumentID
		out["link_creator"] = m.LinkCreator
		out["link_conflicting"] = m.LinkConflicting
	} else {
		if m.SourceAccountID != nil {
			out["source_account_id"] = m.SourceAccountID
		}
		if m.DestinationAccountID != nil {
			out["destination_account_id"] = m.DestinationAccountID
		}
		if m.ImportBatchID != nil {
			out["import_batch_id"] = m.ImportBatchID
		}
		if m.ImportLine != nil {
			out["import_line"] = m.ImportLine
		}
		if m.ExternalReference != nil {
			out["external_reference"] = m.ExternalReference
		}
		if m.LinkedDocumentID != nil {
			out["linked_document_id"] = m.LinkedDocumentID
		}
		if m.LinkCreator != nil {
			out["link_creator"] = m.LinkCreator
		}
		if m.LinkConflicting {
			out["link_conflicting"] = m.LinkConflicting
		}
	}
	if m.OwnerHouseholdID != nil {
		out["owner_household_id"] = m.OwnerHouseholdID
	}
	return out
}

func importBatchToMap(b entity.ImportBatch) map[string]any {
	m := make(map[string]any)
	if b.ID != "" {
		m["id"] = b.ID
	}
	if b.OwnerID != "" {
		m["owner_id"] = b.OwnerID
	}
	if b.State != "" {
		m["state"] = b.State
	}
	if b.AccountID != "" {
		m["account_id"] = b.AccountID
	}
	if b.SourceID != "" {
		m["source_id"] = b.SourceID
	}
	if b.Filename != "" {
		m["filename"] = b.Filename
	}
	if b.Format != "" {
		m["format"] = b.Format
	}
	if b.LineCountValid > 0 {
		m["line_count_valid"] = b.LineCountValid
	}
	if b.LineCountDuplicate > 0 {
		m["line_count_duplicate"] = b.LineCountDuplicate
	}
	if b.LineCountPossibleDup > 0 {
		m["line_count_possible_dup"] = b.LineCountPossibleDup
	}
	if b.LineCountError > 0 {
		m["line_count_error"] = b.LineCountError
	}
	if b.OwnerHouseholdID != nil {
		m["owner_household_id"] = b.OwnerHouseholdID
	}
	return m
}

func importLineToMap(l entity.ImportLine) map[string]any {
	m := make(map[string]any)
	if l.ID != "" {
		m["id"] = l.ID
	}
	if l.OwnerID != "" {
		m["owner_id"] = l.OwnerID
	}
	if l.BatchID != "" {
		m["batch_id"] = l.BatchID
	}
	if l.LineRef != 0 {
		// Line refs are 1-based; 0 is the "unset" zero value.
		m["line_ref"] = l.LineRef
	}
	if l.RawLine != "" {
		m["raw_line"] = l.RawLine
	}
	if l.OccurredOn != nil {
		m["occurred_on"] = l.OccurredOn
	}
	if l.Amount != nil {
		// Exact-decimal money string; passes through as-is, never a float.
		m["amount"] = l.Amount
	}
	if l.Direction != nil && *l.Direction != "" {
		m["direction"] = *l.Direction
	}
	if l.Description != nil {
		m["description"] = l.Description
		if l.NormDescription != nil {
			m["norm_description"] = l.NormDescription
		} else {
			nd := commons.NormalizeDescription(*l.Description)
			m["norm_description"] = &nd
		}
	}
	if l.ExternalReference != nil {
		m["external_reference"] = l.ExternalReference
	}
	if l.Status != "" {
		m["status"] = l.Status
	}
	if l.ErrorReason != nil {
		m["error_reason"] = l.ErrorReason
	}
	if l.OwnerHouseholdID != nil {
		m["owner_household_id"] = l.OwnerHouseholdID
	}
	return m
}

// newAccountRepoForTx returns an account repository bound to an ambient transaction.
func newAccountRepoForTx(tx pgx.Tx) *AccountRepository {
	return &AccountRepository{
		pgRepository: &pgRepository[entity.FinancialAccount]{
			scope:     &txScopeImpl{tx: tx},
			table:     "financial_accounts",
			scanRow:   scanFinancialAccount,
			toMap:     financialAccountToMap,
			filters:   accountFilters,
			shareable: true,
		},
	}
}

// newMovementRepoForTx returns a movement repository bound to an ambient transaction.
func newMovementRepoForTx(tx pgx.Tx) *MovementRepository {
	return &MovementRepository{
		pgRepository: &pgRepository[entity.MoneyMovement]{
			scope:     &txScopeImpl{tx: tx},
			table:     "money_movements",
			scanRow:   scanMoneyMovement,
			toMap:     moneyMovementToMap,
			filters:   movementFilters,
			shareable: true,
		},
	}
}

// newImportBatchRepoForTx returns an import batch repository bound to an ambient transaction.
func newImportBatchRepoForTx(tx pgx.Tx) *ImportBatchRepository {
	return &ImportBatchRepository{
		pgRepository: &pgRepository[entity.ImportBatch]{
			scope:     &txScopeImpl{tx: tx},
			table:     "import_batches",
			scanRow:   scanImportBatch,
			toMap:     importBatchToMap,
			filters:   importBatchFilters,
			shareable: true,
		},
	}
}

// newImportLineRepoForTx returns an import line repository bound to an ambient transaction.
func newImportLineRepoForTx(tx pgx.Tx) *ImportLineRepository {
	return &ImportLineRepository{
		pgRepository: &pgRepository[entity.ImportLine]{
			scope:     &txScopeImpl{tx: tx},
			table:     "import_lines",
			scanRow:   scanImportLine,
			toMap:     importLineToMap,
			filters:   importLineFilters,
			shareable: true,
		},
	}
}
