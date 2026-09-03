package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/core/ledger"
)

// CreateMovement processes POST /api/users/{userId}/finance/movements: it decodes
// the JSON body, validates the occurred_on date, and creates the movement via
// the ledger service.
func (s *Server) CreateMovement(w http.ResponseWriter, r *http.Request, userId string) {
	var body gen.CreateMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	occurredOn := body.OccurredOn.Time
	if occurredOn.IsZero() {
		httpx.WriteError(w, http.StatusBadRequest, "invalid occurred_on")
		return
	}

	mv, err := s.ledger.CreateManualMovement(r.Context(), ledger.MovementInput{
		Kind:                 body.Kind,
		Amount:               body.Amount,
		Currency:             body.Currency,
		OccurredOn:           occurredOn,
		Description:          body.Description,
		SourceAccountID:      body.SourceAccountId,
		DestinationAccountID: body.DestinationAccountId,
	})
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toMovement(mv))
}

// ListMovements returns the requesting user's movements filtered by the
// optional account_id (source OR destination) and inclusive from/to
// occurred-on bounds. The result is never nil.
func (s *Server) ListMovements(w http.ResponseWriter, r *http.Request, userId string, params gen.ListMovementsParams) {
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

	mvs, err := s.ledger.ListMovements(r.Context(), ledger.MovementListFilter{
		AccountID:    accountId,
		OccurredFrom: from,
		OccurredTo:   to,
	})
	if err != nil {
		writeFinanceError(w, err)
		return
	}

	out := make([]gen.Movement, 0, len(mvs))
	for _, m := range mvs {
		out = append(out, toMovement(m))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// GetMovement returns the requesting user's movement by ID. Unknown or
// another user's IDs yield 404.
func (s *Server) GetMovement(w http.ResponseWriter, r *http.Request, userId string, id string) {
	mv, err := s.ledger.GetMovement(r.Context(), id)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMovement(mv))
}

// PatchMovement replaces a movement's description. Only the description is
// editable; a request carrying any core field is rejected before any
// repository call, as are blank descriptions.
func (s *Server) PatchMovement(w http.ResponseWriter, r *http.Request, userId string, id string) {
	var body gen.PatchMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	core := []string{body.Amount, body.Currency, body.OccurredOn, body.Kind, body.SourceAccountId, body.DestinationAccountId}
	for _, v := range core {
		if v != "" {
			httpx.WriteError(w, http.StatusBadRequest, "core movement fields are immutable")
			return
		}
	}
	if strings.TrimSpace(body.Description) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid description")
		return
	}

	mv, err := s.ledger.PatchDescription(r.Context(), id, body.Description)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMovement(mv))
}

// DeleteMovement deletes a manual movement (204 with an empty body). Imported
// movements are protected: the service returns ErrConflict (409).
func (s *Server) DeleteMovement(w http.ResponseWriter, r *http.Request, userId string, id string) {
	if err := s.ledger.DeleteMovement(r.Context(), id); err != nil {
		writeFinanceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LinkMovement links a movement to a document, recording the link creator and
// any price/currency disagreement (flagged, never overwritten). Re-linking the
// same pair is an idempotent no-op; conflicting pairs yield 409.
func (s *Server) LinkMovement(w http.ResponseWriter, r *http.Request, userId string, id string) {
	var body gen.LinkMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.DocumentId == "" {
		httpx.WriteError(w, http.StatusBadRequest, "document_id is required")
		return
	}

	if err := s.ledger.Link(r.Context(), id, body.DocumentId, entity.LinkCreatorManual); err != nil {
		writeFinanceError(w, err)
		return
	}

	mv, err := s.ledger.GetMovement(r.Context(), id)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMovement(mv))
}

// UnlinkMovement clears a movement's document link (204 with an empty body).
// Unlinking a movement that has no link is a no-op.
func (s *Server) UnlinkMovement(w http.ResponseWriter, r *http.Request, userId string, id string) {
	if err := s.ledger.Unlink(r.Context(), id); err != nil {
		writeFinanceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
