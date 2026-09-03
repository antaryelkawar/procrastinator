package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/household"
)

// householdMemberJSON is the JSON representation of a household member.
type householdMemberJSON struct {
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// householdJSON is the JSON representation of a household returned by the API.
// Members is never nil: it marshals as [] rather than null.
type householdJSON struct {
	ID          string                `json:"id"`
	DisplayName string                `json:"display_name"`
	OwnerID     string                `json:"owner_id"`
	CreatedAt   time.Time             `json:"created_at"`
	Members     []householdMemberJSON `json:"members"`
}

// toHouseholdJSON converts a household.HouseholdWithMembers into its JSON DTO.
// The Members slice is never nil.
func toHouseholdJSON(hh household.HouseholdWithMembers) householdJSON {
	members := make([]householdMemberJSON, 0, len(hh.Members))
	for _, m := range hh.Members {
		members = append(members, householdMemberJSON{
			UserID:    m.UserID,
			CreatedAt: m.CreatedAt,
		})
	}
	return householdJSON{
		ID:          hh.Household.ID,
		DisplayName: hh.Household.DisplayName,
		OwnerID:     hh.Household.OwnerID,
		CreatedAt:   hh.Household.CreatedAt,
		Members:     members,
	}
}

// createHouseholdBody is the JSON body for POST /api/users/{userId}/households.
type createHouseholdBody struct {
	DisplayName string `json:"display_name"`
}

// addMemberBody is the JSON body for POST /api/users/{userId}/households/{householdId}/members.
type addMemberBody struct {
	UserID string `json:"user_id"`
}

// createHousehold processes POST /api/users/{userId}/households: it decodes the
// JSON body and creates a household via the household service. The creator is
// auto-added as the first member. A blank or missing display_name is rejected
// with 400.
func (s *Server) createHousehold(w http.ResponseWriter, r *http.Request) {
	var body createHouseholdBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.DisplayName) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	created, err := s.household.CreateHousehold(r.Context(), body.DisplayName)
	if err != nil {
		writeHouseholdError(w, err)
		return
	}

	// Fetch the full household with members (the creator is the first member).
	full, err := s.household.GetHousehold(r.Context(), created.ID)
	if err != nil {
		writeHouseholdError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toHouseholdJSON(full))
}

// addHouseholdMember processes POST /api/users/{userId}/households/{householdId}/members:
// it decodes the JSON body and adds a user to the household via the household
// service. The requester must already be a member of the household.
func (s *Server) addHouseholdMember(w http.ResponseWriter, r *http.Request) {
	hhID := chi.URLParam(r, "householdId")

	var body addMemberBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.UserID) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := s.household.AddMember(r.Context(), hhID, body.UserID); err != nil {
		writeHouseholdError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listHouseholds processes GET /api/users/{userId}/households: it returns all
// households the user owns or is a member of, each with its members. The result
// is never nil.
func (s *Server) listHouseholds(w http.ResponseWriter, r *http.Request) {
	list, err := s.household.ListMyHouseholds(r.Context())
	if err != nil {
		writeHouseholdError(w, err)
		return
	}
	out := make([]householdJSON, 0, len(list))
	for _, hh := range list {
		out = append(out, toHouseholdJSON(hh))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// getHousehold processes GET /api/users/{userId}/households/{householdId}: it
// returns a single household with its members. A household the requester is not
// a member of yields 404 (membership is hidden, not rejected).
func (s *Server) getHousehold(w http.ResponseWriter, r *http.Request) {
	hhID := chi.URLParam(r, "householdId")

	full, err := s.household.GetHousehold(r.Context(), hhID)
	if err != nil {
		// ErrNotMember and ErrNotFound both map to 404 here: the API does not
		// distinguish "not a member" from "doesn't exist" for visibility.
		switch {
		case errors.Is(err, household.ErrNotMember), errors.Is(err, repo.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "not found")
		case errors.Is(err, user.ErrNoUser):
			httpx.WriteError(w, http.StatusUnauthorized, "missing or invalid user identity")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toHouseholdJSON(full))
}

// writeHouseholdError maps core/household and persistence sentinels onto the
// HTTP status contract: ErrInvalid -> 400, ErrNotMember -> 403 (or 404 for
// getHousehold where the API hides membership), repo.ErrNotFound -> 404,
// user.ErrNoUser -> 401 (defensive), else 500.
func writeHouseholdError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, household.ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, "invalid input")
	case errors.Is(err, household.ErrNotMember):
		httpx.WriteError(w, http.StatusForbidden, "not a member of the household")
	case errors.Is(err, repo.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, user.ErrNoUser):
		httpx.WriteError(w, http.StatusUnauthorized, "missing or invalid user identity")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
