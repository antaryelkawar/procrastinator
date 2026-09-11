package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
			"SELECT %s FROM financial_accounts WHERE %s AND ((payload #>> '{data,name}') ILIKE $%d OR (payload #>> '{data,account_type}') ILIKE $%d OR (payload #>> '{data,institution}') ILIKE $%d) ORDER BY created_at DESC, id ASC",
			r.selectList(), vis, patN, patN, patN)

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
			"SELECT %s FROM money_movements WHERE %s AND ((payload #>> '{data,description}') ILIKE $%d OR (payload #>> '{data,external_reference}') ILIKE $%d) ORDER BY created_at DESC, id ASC",
			r.selectList(), vis, patN, patN)

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
			"SELECT %s FROM import_batches WHERE %s AND (payload #>> '{data,filename}') ILIKE $%d ORDER BY created_at DESC, id ASC",
			r.selectList(), vis, patN)

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
			codec:     accountCodec,
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
			codec:     movementCodec,
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
			codec:     importBatchCodec,
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
			codec:     importLineCodec,
			shareable: true,
		},
	}
}

// newAccountRepoForTx returns an account repository bound to an ambient transaction.
func newAccountRepoForTx(tx pgx.Tx) *AccountRepository {
	return &AccountRepository{
		pgRepository: &pgRepository[entity.FinancialAccount]{
			scope:     &txScopeImpl{tx: tx},
			table:     "financial_accounts",
			scanRow:   scanFinancialAccount,
			codec:     accountCodec,
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
			codec:     movementCodec,
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
			codec:     importBatchCodec,
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
			codec:     importLineCodec,
			shareable: true,
		},
	}
}
