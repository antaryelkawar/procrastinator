package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/household"
)

// toHousehold converts a household.HouseholdWithMembers into the generated
// gen.Household DTO. The Members slice is never nil.
func toHousehold(hh household.HouseholdWithMembers) gen.Household {
	members := make([]gen.HouseholdMember, 0, len(hh.Members))
	for _, m := range hh.Members {
		members = append(members, gen.HouseholdMember{
			UserId:    m.UserID,
			CreatedAt: m.CreatedAt,
		})
	}
	return gen.Household{
		Id:          hh.Household.ID,
		DisplayName: hh.Household.DisplayName,
		OwnerId:     hh.Household.OwnerID,
		CreatedAt:   hh.Household.CreatedAt,
		Members:     members,
	}
}

// CreateHousehold processes POST /api/users/{userId}/households: it decodes the
// JSON body and creates a household via the household service. The creator is
// auto-added as the first member. A blank or missing display_name is rejected
// with 400.
func (s *Server) CreateHousehold(w http.ResponseWriter, r *http.Request, userId string) {
	var body gen.CreateHouseholdRequest
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
	httpx.WriteJSON(w, http.StatusCreated, toHousehold(full))
}

// AddHouseholdMember processes POST /api/users/{userId}/households/{householdId}/members:
// it decodes the JSON body and adds a user to the household via the household
// service. The requester must already be a member of the household.
func (s *Server) AddHouseholdMember(w http.ResponseWriter, r *http.Request, userId string, householdId string) {
	var body gen.AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.UserId) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := s.household.AddMember(r.Context(), householdId, body.UserId); err != nil {
		writeHouseholdError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListHouseholds processes GET /api/users/{userId}/households: it returns all
// households the user owns or is a member of, each with its members. The result
// is never nil.
func (s *Server) ListHouseholds(w http.ResponseWriter, r *http.Request, userId string) {
	list, err := s.household.ListMyHouseholds(r.Context())
	if err != nil {
		writeHouseholdError(w, err)
		return
	}
	out := make([]gen.Household, 0, len(list))
	for _, hh := range list {
		out = append(out, toHousehold(hh))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// GetHousehold processes GET /api/users/{userId}/households/{householdId}: it
// returns a single household with its members. A household the requester is not
// a member of yields 404 (membership is hidden, not rejected).
func (s *Server) GetHousehold(w http.ResponseWriter, r *http.Request, userId string, householdId string) {
	full, err := s.household.GetHousehold(r.Context(), householdId)
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
	httpx.WriteJSON(w, http.StatusOK, toHousehold(full))
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
