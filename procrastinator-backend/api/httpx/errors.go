package httpx

import (
	"errors"
	"net/http"

	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/household"
	"procrastinator-backend/core/identity"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/processing"
	"procrastinator-backend/core/statement"
	"procrastinator-backend/infra/filestorage"
)

// APIError is the typed error returned by handlers for all failure cases.
// The centralized ResponseErrorHandlerFunc unwraps it to write the
// {"error": msg} envelope with the given status.
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string { return e.Msg }

// NewAPIError builds an *APIError with the given status and message.
func NewAPIError(status int, msg string) error {
	return &APIError{Status: status, Msg: msg}
}

// IsMaxBytesErr reports whether err (or any error in its chain) is an
// *http.MaxBytesError.
func IsMaxBytesErr(err error) bool {
	var mbErr *http.MaxBytesError
	return errors.As(err, &mbErr)
}

// MapIngestError maps core/processing and related sentinels to (status, msg).
func MapIngestError(err error) (int, string) {
	switch {
	case errors.Is(err, processing.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, "upload exceeds size limit"
	case errors.Is(err, filestorage.ErrUnsupportedType):
		return http.StatusUnsupportedMediaType, "unsupported file type"
	case errors.Is(err, processing.ErrExtraction):
		return http.StatusBadGateway, "extraction failed"
	case errors.Is(err, identity.ErrNoIdentity):
		return http.StatusUnprocessableEntity, "no usable identity in document"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// MapLedgerError maps core/ledger and persistence sentinels to (status, msg).
func MapLedgerError(err error) (int, string) {
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

// MapStatementError maps core/statement and persistence sentinels to (status, msg).
func MapStatementError(err error) (int, string) {
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

// MapHouseholdError maps core/household and persistence sentinels to (status, msg).
func MapHouseholdError(err error) (int, string) {
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
