package finance

import (
	"context"
	"net/http"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/ledger"
)

// CreateAccount processes POST /api/users/{userId}/finance/accounts: it consumes
// the generated JSON body and creates the account via the ledger service. A
// freshly created account has no movements, so its derived balance is exactly
// "0".
func (s *Service) CreateAccount(ctx context.Context, request gen.CreateAccountRequestObject) (gen.CreateAccountResponseObject, error) {
	if request.Body == nil {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	body := *request.Body
	acc, err := s.ledger.CreateAccount(ctx, ledger.AccountInput{
		Name:               body.Name,
		Type:               body.Type,
		Currency:           body.Currency,
		Institution:        body.Institution,
		ExternalDescriptor: body.ExternalDescriptor,
	})
	if err != nil {
		status, msg := httpx.MapLedgerError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	return gen.CreateAccount201JSONResponse(ToAccount(acc, "0")), nil
}

// ListAccounts returns all of the requesting user's accounts with each
// account's derived balance. The result is never nil.
func (s *Service) ListAccounts(ctx context.Context, request gen.ListAccountsRequestObject) (gen.ListAccountsResponseObject, error) {
	accs, err := s.ledger.ListAccounts(ctx)
	if err != nil {
		status, msg := httpx.MapLedgerError(err)
		return nil, httpx.NewAPIError(status, msg)
	}

	out := make([]gen.Account, 0, len(accs))
	for _, a := range accs {
		balance, err := s.balancer.BalanceForAccount(ctx, a.ID, repo.Owner(request.UserId))
		if err != nil {
			status, msg := httpx.MapLedgerError(err)
			return nil, httpx.NewAPIError(status, msg)
		}
		out = append(out, ToAccount(a, balance))
	}
	return gen.ListAccounts200JSONResponse(out), nil
}

// GetAccount returns the requesting user's account by ID together with its
// derived balance. Unknown or another user's IDs yield 404. The ledger service
// resolves the user from the context; the check here is defensive.
func (s *Service) GetAccount(ctx context.Context, request gen.GetAccountRequestObject) (gen.GetAccountResponseObject, error) {
	acc, balance, err := s.ledger.GetAccount(ctx, request.Id)
	if err != nil {
		status, msg := httpx.MapLedgerError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	return gen.GetAccount200JSONResponse(ToAccount(acc, balance)), nil
}
