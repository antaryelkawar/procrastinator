package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/ledger"
)

// createAccountBody is the JSON body for POST /api/finance/accounts.
type createAccountBody struct {
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	Currency           string  `json:"currency"`
	Institution        *string `json:"institution"`
	ExternalDescriptor *string `json:"external_descriptor"`
}

// createAccount processes POST /api/finance/accounts: it decodes the JSON body
// and creates the account via the ledger service. A freshly created account
// has no movements, so its derived balance is exactly "0".
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var body createAccountBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	acc, err := s.ledger.CreateAccount(r.Context(), ledger.AccountInput{
		Name:               body.Name,
		Type:               body.Type,
		Currency:           body.Currency,
		Institution:        body.Institution,
		ExternalDescriptor: body.ExternalDescriptor,
	})
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAccountJSON(acc, "0"))
}

// listAccounts returns all of the requesting user's accounts with each
// account's derived balance. The result is never nil.
func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	tid, ok := userFromCtx(w, r.Context())
	if !ok {
		return
	}

	accs, err := s.ledger.ListAccounts(r.Context())
	if err != nil {
		writeFinanceError(w, err)
		return
	}

	out := make([]accountJSON, 0, len(accs))
	for _, a := range accs {
		balance, err := s.balancer.BalanceForAccount(r.Context(), a.ID, repo.Owner(tid))
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		out = append(out, toAccountJSON(a, balance))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// getAccount returns the requesting user's account by ID together with its
// derived balance. Unknown or another user's IDs yield 404. The ledger
// service resolves the user from the context; the check here is defensive.
func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	if _, ok := userFromCtx(w, r.Context()); !ok {
		return
	}
	id := chi.URLParam(r, "id")

	acc, balance, err := s.ledger.GetAccount(r.Context(), id)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAccountJSON(acc, balance))
}

// writeFinanceError maps core/ledger and persistence sentinels onto the HTTP
// status contract (D11): ErrInvalid -> 400, ErrConflict -> 409,
// repo.ErrNotFound -> 404, user.ErrNoUser -> 401 (defensive), else 500.
func writeFinanceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, "invalid input")
	case errors.Is(err, ledger.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflicting state")
	case errors.Is(err, repo.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, user.ErrNoUser):
		httpx.WriteError(w, http.StatusUnauthorized, "missing or invalid user identity")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
