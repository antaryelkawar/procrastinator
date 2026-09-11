package finance

import (
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/statement"
)

// Service serves the finance endpoints (accounts, movements, import-batches).
type Service struct {
	ledger    *ledger.Service
	balancer  ledger.BalanceQuerier
	statement *statement.Service
	factory   *repo.Factory
}

// New constructs a Service.
func New(ledger *ledger.Service, balancer ledger.BalanceQuerier, statement *statement.Service, factory *repo.Factory) *Service {
	return &Service{ledger: ledger, balancer: balancer, statement: statement, factory: factory}
}
