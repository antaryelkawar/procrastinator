package api

import (
	"errors"
	"net/http"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/household"
	"procrastinator-backend/core/identity"
	"procrastinator-backend/core/ingest"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/statement"
	"procrastinator-backend/infra/filestorage"
)

// apiError is the typed error returned by handlers for all failure cases.
// The centralized ResponseErrorHandlerFunc unwraps it to write the
// {"error": msg} envelope with the given status.
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func newAPIError(status int, msg string) error {
	return &apiError{status: status, msg: msg}
}

// mapIngestError maps core/ingest and related sentinels to (status, msg).
func mapIngestError(err error) (int, string) {
	switch {
	case errors.Is(err, ingest.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, "upload exceeds size limit"
	case errors.Is(err, filestorage.ErrUnsupportedType):
		return http.StatusUnsupportedMediaType, "unsupported file type"
	case errors.Is(err, ingest.ErrExtraction):
		return http.StatusBadGateway, "extraction failed"
	case errors.Is(err, identity.ErrNoIdentity):
		return http.StatusUnprocessableEntity, "no usable identity in document"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// mapLedgerError maps core/ledger and persistence sentinels to (status, msg).
func mapLedgerError(err error) (int, string) {
	switch {
	case errors.Is(err, ledger.ErrInvalid):
		return http.StatusBadRequest, "invalid input"
	case errors.Is(err, ledger.ErrConflict):
		return http.StatusConflict, "conflicting state"
	case errors.Is(err, repo.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, user.ErrNoUser):
		return http.StatusUnauthorized, "missing or invalid user identity"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// mapStatementError maps core/statement and persistence sentinels to (status, msg).
func mapStatementError(err error) (int, string) {
	switch {
	case errors.Is(err, statement.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, "upload exceeds size limit"
	case errors.Is(err, statement.ErrUnsupportedType):
		return http.StatusUnsupportedMediaType, "unsupported statement type"
	case errors.Is(err, statement.ErrNoLines):
		return http.StatusUnprocessableEntity, "statement has no parseable lines"
	case errors.Is(err, statement.ErrTooManyLines):
		return http.StatusUnprocessableEntity, "statement exceeds line limit"
	case errors.Is(err, statement.ErrConflict):
		return http.StatusConflict, "conflicting batch state"
	case errors.Is(err, repo.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, user.ErrNoUser):
		return http.StatusUnauthorized, "missing or invalid user identity"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// mapHouseholdError maps core/household and persistence sentinels to (status, msg).
func mapHouseholdError(err error) (int, string) {
	switch {
	case errors.Is(err, household.ErrInvalid):
		return http.StatusBadRequest, "invalid input"
	case errors.Is(err, household.ErrNotMember):
		return http.StatusForbidden, "not a member of the household"
	case errors.Is(err, repo.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, user.ErrNoUser):
		return http.StatusUnauthorized, "missing or invalid user identity"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// writeErrorEnvelope is the single shared function that writes the
// {"error": msg} envelope with a status + Content-Type: application/json. It
// is the centralized error encoder used by the strict-handler error funcs and
// the UserMiddleware (via httpx.WriteErrorEnvelope).
func writeErrorEnvelope(w http.ResponseWriter, status int, msg string) {
	httpx.WriteErrorEnvelope(w, status, msg)
}
