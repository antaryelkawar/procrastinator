package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/core/ledger"
)

// createMovementBody is the JSON body for POST /api/finance/movements. Amount
// is an exact-decimal string; OccurredOn is a "2006-01-02" date string.
type createMovementBody struct {
	Kind                 string `json:"kind"`
	Amount               string `json:"amount"`
	Currency             string `json:"currency"`
	OccurredOn           string `json:"occurred_on"`
	Description          string `json:"description"`
	SourceAccountID      string `json:"source_account_id"`
	DestinationAccountID string `json:"destination_account_id"`
}

// patchMovementBody carries the only editable field plus every core field, so
// that a request attempting to edit a core field is detectable and rejected.
type patchMovementBody struct {
	Description          string `json:"description"`
	Amount               string `json:"amount"`
	Currency             string `json:"currency"`
	OccurredOn           string `json:"occurred_on"`
	Kind                 string `json:"kind"`
	SourceAccountID      string `json:"source_account_id"`
	DestinationAccountID string `json:"destination_account_id"`
}

// linkBody is the JSON body for POST /api/finance/movements/{id}/link.
type linkBody struct {
	DocumentID string `json:"document_id"`
}

// parseOccurredOn parses the occurred_on request value. commons.ParseDate
// returns a zero time with a nil error for empty input, so the empty case is
// checked explicitly; both are 400.
func parseOccurredOn(w http.ResponseWriter, s string) (time.Time, bool) {
	if strings.TrimSpace(s) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid occurred_on")
		return time.Time{}, false
	}
	t, err := commons.ParseDate(s)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid occurred_on")
		return time.Time{}, false
	}
	return t, true
}

// createMovement processes POST /api/finance/movements: it decodes the JSON
// body, validates the occurred_on date, and creates the movement via the
// ledger service.
func (s *Server) createMovement(w http.ResponseWriter, r *http.Request) {
	var body createMovementBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	occurredOn, ok := parseOccurredOn(w, body.OccurredOn)
	if !ok {
		return
	}

	mv, err := s.ledger.CreateManualMovement(r.Context(), ledger.MovementInput{
		Kind:                 body.Kind,
		Amount:               body.Amount,
		Currency:             body.Currency,
		OccurredOn:           occurredOn,
		Description:          body.Description,
		SourceAccountID:      body.SourceAccountID,
		DestinationAccountID: body.DestinationAccountID,
	})
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toMovementJSON(mv))
}

// listMovements returns the requesting user's movements filtered by the
// optional account_id (source OR destination) and inclusive from/to
// occurred-on bounds. The result is never nil.
func (s *Server) listMovements(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		t, err := commons.ParseDate(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid from")
			return
		}
		from = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := commons.ParseDate(v)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid to")
			return
		}
		to = &t
	}

	mvs, err := s.ledger.ListMovements(r.Context(), ledger.MovementListFilter{
		AccountID:    q.Get("account_id"),
		OccurredFrom: from,
		OccurredTo:   to,
	})
	if err != nil {
		writeFinanceError(w, err)
		return
	}

	out := make([]movementJSON, 0, len(mvs))
	for _, m := range mvs {
		out = append(out, toMovementJSON(m))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// getMovement returns the requesting user's movement by ID. Unknown or
// another user's IDs yield 404.
func (s *Server) getMovement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	mv, err := s.ledger.GetMovement(r.Context(), id)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMovementJSON(mv))
}

// patchMovement replaces a movement's description. Only the description is
// editable; a request carrying any core field is rejected before any
// repository call, as are blank descriptions.
func (s *Server) patchMovement(w http.ResponseWriter, r *http.Request) {
	var body patchMovementBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	core := []string{body.Amount, body.Currency, body.OccurredOn, body.Kind, body.SourceAccountID, body.DestinationAccountID}
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

	id := chi.URLParam(r, "id")
	mv, err := s.ledger.PatchDescription(r.Context(), id, body.Description)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMovementJSON(mv))
}

// deleteMovement deletes a manual movement (204 with an empty body). Imported
// movements are protected: the service returns ErrConflict (409).
func (s *Server) deleteMovement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.ledger.DeleteMovement(r.Context(), id); err != nil {
		writeFinanceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// linkMovement links a movement to a document, recording the link creator and
// any price/currency disagreement (flagged, never overwritten). Re-linking the
// same pair is an idempotent no-op; conflicting pairs yield 409.
func (s *Server) linkMovement(w http.ResponseWriter, r *http.Request) {
	var body linkBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.DocumentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "document_id is required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.ledger.Link(r.Context(), id, body.DocumentID, entity.LinkCreatorManual); err != nil {
		writeFinanceError(w, err)
		return
	}

	mv, err := s.ledger.GetMovement(r.Context(), id)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMovementJSON(mv))
}

// unlinkMovement clears a movement's document link (204 with an empty body).
// Unlinking a movement that has no link is a no-op.
func (s *Server) unlinkMovement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.ledger.Unlink(r.Context(), id); err != nil {
		writeFinanceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
