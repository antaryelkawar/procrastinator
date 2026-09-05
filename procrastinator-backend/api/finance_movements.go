package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/core/ledger"
)

// CreateMovement processes POST /api/users/{userId}/finance/movements: it
// consumes the generated JSON body, validates the occurred_on date, and
// creates the movement via the ledger service.
func (s *Server) CreateMovement(ctx context.Context, request gen.CreateMovementRequestObject) (gen.CreateMovementResponseObject, error) {
	if request.Body == nil {
		return nil, newAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	body := *request.Body
	occurredOn := body.OccurredOn.Time
	if occurredOn.IsZero() {
		return nil, newAPIError(http.StatusBadRequest, "invalid occurred_on")
	}
	mv, err := s.ledger.CreateManualMovement(ctx, ledger.MovementInput{
		Kind:                 body.Kind,
		Amount:               body.Amount,
		Currency:             body.Currency,
		OccurredOn:           occurredOn,
		Description:          body.Description,
		SourceAccountID:      body.SourceAccountId,
		DestinationAccountID: body.DestinationAccountId,
	})
	if err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.CreateMovement201JSONResponse(toMovement(mv)), nil
}

// ListMovements returns the requesting user's movements filtered by the
// optional account_id (source OR destination) and inclusive from/to
// occurred-on bounds. The result is never nil.
func (s *Server) ListMovements(ctx context.Context, request gen.ListMovementsRequestObject) (gen.ListMovementsResponseObject, error) {
	params := request.Params
	var from, to *time.Time
	if params.From != nil {
		t := (*params.From).Time
		from = &t
	}
	if params.To != nil {
		t := (*params.To).Time
		to = &t
	}
	var accountId string
	if params.AccountId != nil {
		accountId = *params.AccountId
	}
	mvs, err := s.ledger.ListMovements(ctx, ledger.MovementListFilter{
		AccountID:    accountId,
		OccurredFrom: from,
		OccurredTo:   to,
	})
	if err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	out := make([]gen.Movement, 0, len(mvs))
	for _, m := range mvs {
		out = append(out, toMovement(m))
	}
	return gen.ListMovements200JSONResponse(out), nil
}

// GetMovement returns the requesting user's movement by ID. Unknown or another
// user's IDs yield 404.
func (s *Server) GetMovement(ctx context.Context, request gen.GetMovementRequestObject) (gen.GetMovementResponseObject, error) {
	mv, err := s.ledger.GetMovement(ctx, request.Id)
	if err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.GetMovement200JSONResponse(toMovement(mv)), nil
}

// PatchMovement replaces a movement's description. Only the description is
// editable; a request carrying any core field is rejected before any
// repository call, as are blank descriptions.
func (s *Server) PatchMovement(ctx context.Context, request gen.PatchMovementRequestObject) (gen.PatchMovementResponseObject, error) {
	if request.Body == nil {
		return nil, newAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	body := *request.Body
	core := []string{body.Amount, body.Currency, body.OccurredOn, body.Kind, body.SourceAccountId, body.DestinationAccountId}
	for _, v := range core {
		if v != "" {
			return nil, newAPIError(http.StatusBadRequest, "core movement fields are immutable")
		}
	}
	if strings.TrimSpace(body.Description) == "" {
		return nil, newAPIError(http.StatusBadRequest, "invalid description")
	}
	mv, err := s.ledger.PatchDescription(ctx, request.Id, body.Description)
	if err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.PatchMovement200JSONResponse(toMovement(mv)), nil
}

// DeleteMovement deletes a manual movement (204 with an empty body). Imported
// movements are protected: the service returns ErrConflict (409).
func (s *Server) DeleteMovement(ctx context.Context, request gen.DeleteMovementRequestObject) (gen.DeleteMovementResponseObject, error) {
	if err := s.ledger.DeleteMovement(ctx, request.Id); err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.DeleteMovement204Response{}, nil
}

// LinkMovement links a movement to a document, recording the link creator and
// any price/currency disagreement (flagged, never overwritten). Re-linking the
// same pair is an idempotent no-op; conflicting pairs yield 409.
func (s *Server) LinkMovement(ctx context.Context, request gen.LinkMovementRequestObject) (gen.LinkMovementResponseObject, error) {
	if request.Body == nil {
		return nil, newAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	body := *request.Body
	if body.DocumentId == "" {
		return nil, newAPIError(http.StatusBadRequest, "document_id is required")
	}
	if err := s.ledger.Link(ctx, request.Id, body.DocumentId, entity.LinkCreatorManual); err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	mv, err := s.ledger.GetMovement(ctx, request.Id)
	if err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.LinkMovement200JSONResponse(toMovement(mv)), nil
}

// UnlinkMovement clears a movement's document link (204 with an empty body).
// Unlinking a movement that has no link is a no-op.
func (s *Server) UnlinkMovement(ctx context.Context, request gen.UnlinkMovementRequestObject) (gen.UnlinkMovementResponseObject, error) {
	if err := s.ledger.Unlink(ctx, request.Id); err != nil {
		status, msg := mapLedgerError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.UnlinkMovement204Response{}, nil
}
