package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"procrastinator-backend/api/gen"
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

// CreateHousehold processes POST /api/users/{userId}/households: it consumes
// the generated JSON body and creates a household via the household service.
// The creator is auto-added as the first member. A blank or missing
// display_name is rejected with 400.
func (s *Server) CreateHousehold(ctx context.Context, request gen.CreateHouseholdRequestObject) (gen.CreateHouseholdResponseObject, error) {
	if request.Body == nil {
		return nil, newAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(request.Body.DisplayName) == "" {
		return nil, newAPIError(http.StatusBadRequest, "invalid JSON body")
	}

	created, err := s.household.CreateHousehold(ctx, request.Body.DisplayName)
	if err != nil {
		status, msg := mapHouseholdError(err)
		return nil, newAPIError(status, msg)
	}

	// Fetch the full household with members (the creator is the first member).
	full, err := s.household.GetHousehold(ctx, created.ID)
	if err != nil {
		status, msg := mapHouseholdError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.CreateHousehold201JSONResponse(toHousehold(full)), nil
}

// AddHouseholdMember processes POST /api/users/{userId}/households/{householdId}/members:
// it consumes the generated JSON body and adds a user to the household via the
// household service. The requester must already be a member of the household.
func (s *Server) AddHouseholdMember(ctx context.Context, request gen.AddHouseholdMemberRequestObject) (gen.AddHouseholdMemberResponseObject, error) {
	if request.Body == nil {
		return nil, newAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(request.Body.UserId) == "" {
		return nil, newAPIError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := s.household.AddMember(ctx, request.HouseholdId, request.Body.UserId); err != nil {
		status, msg := mapHouseholdError(err)
		return nil, newAPIError(status, msg)
	}
	return gen.AddHouseholdMember204Response{}, nil
}

// ListHouseholds processes GET /api/users/{userId}/households: it returns all
// households the user owns or is a member of, each with its members. The result
// is never nil.
func (s *Server) ListHouseholds(ctx context.Context, request gen.ListHouseholdsRequestObject) (gen.ListHouseholdsResponseObject, error) {
	list, err := s.household.ListMyHouseholds(ctx)
	if err != nil {
		status, msg := mapHouseholdError(err)
		return nil, newAPIError(status, msg)
	}
	out := make([]gen.Household, 0, len(list))
	for _, hh := range list {
		out = append(out, toHousehold(hh))
	}
	return gen.ListHouseholds200JSONResponse(out), nil
}

// GetHousehold processes GET /api/users/{userId}/households/{householdId}: it
// returns a single household with its members. A household the requester is not
// a member of yields 404 (membership is hidden, not rejected).
func (s *Server) GetHousehold(ctx context.Context, request gen.GetHouseholdRequestObject) (gen.GetHouseholdResponseObject, error) {
	full, err := s.household.GetHousehold(ctx, request.HouseholdId)
	if err != nil {
		// ErrNotMember and ErrNotFound both map to 404 here: the API does not
		// distinguish "not a member" from "doesn't exist" for visibility.
		switch {
		case errors.Is(err, household.ErrNotMember), errors.Is(err, repo.ErrNotFound):
			return nil, newAPIError(http.StatusNotFound, "not found")
		case errors.Is(err, user.ErrNoUser):
			return nil, newAPIError(http.StatusUnauthorized, "missing or invalid user identity")
		default:
			return nil, newAPIError(http.StatusInternalServerError, "internal error")
		}
	}
	return gen.GetHousehold200JSONResponse(toHousehold(full)), nil
}
