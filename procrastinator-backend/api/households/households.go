package households

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
	"procrastinator-backend/core/household"
)

// Service serves the household endpoints (create, add-member, list, get).
type Service struct {
	household *household.Service
}

// New constructs a Service.
func New(household *household.Service) *Service {
	return &Service{household: household}
}

// ToHousehold converts a household.HouseholdWithMembers into the generated
// gen.Household DTO. The Members slice is never nil.
func ToHousehold(hh household.HouseholdWithMembers) gen.Household {
	members := make([]gen.HouseholdMember, 0, len(hh.Members))
	for _, m := range hh.Members {
		members = append(members, gen.HouseholdMember{
			UserId:    m.UserID,
			CreatedAt: m.CreatedAt,
		})
	}
	return gen.Household{
		Id:        hh.Household.ID,
		OwnerId:   hh.Household.OwnerID,
		Members:   members,
		CreatedAt: hh.Household.CreatedAt,
		UpdatedAt: hh.Household.UpdatedAt,
		Data: struct {
			DisplayName string `json:"display_name"`
		}{
			DisplayName: hh.Household.DisplayName,
		},
	}
}

// CreateHousehold processes POST /api/users/{userId}/households: it consumes
// the generated JSON body and creates a household via the household service.
// The creator is auto-added as the first member. A blank or missing
// display_name is rejected with 400.
func (s *Service) CreateHousehold(ctx context.Context, request gen.CreateHouseholdRequestObject) (gen.CreateHouseholdResponseObject, error) {
	if request.Body == nil {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(request.Body.DisplayName) == "" {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "invalid JSON body")
	}

	created, err := s.household.CreateHousehold(ctx, request.Body.DisplayName)
	if err != nil {
		status, msg := httpx.MapHouseholdError(err)
		return nil, httpx.NewAPIError(status, msg)
	}

	// Fetch the full household with members (the creator is the first member).
	full, err := s.household.GetHousehold(ctx, created.ID)
	if err != nil {
		status, msg := httpx.MapHouseholdError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	return gen.CreateHousehold201JSONResponse(ToHousehold(full)), nil
}

// AddHouseholdMember processes POST /api/users/{userId}/households/{householdId}/members:
// it consumes the generated JSON body and adds a user to the household via the
// household service. The requester must already be a member of the household.
func (s *Service) AddHouseholdMember(ctx context.Context, request gen.AddHouseholdMemberRequestObject) (gen.AddHouseholdMemberResponseObject, error) {
	if request.Body == nil {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(request.Body.UserId) == "" {
		return nil, httpx.NewAPIError(http.StatusBadRequest, "invalid JSON body")
	}

	if err := s.household.AddMember(ctx, request.HouseholdId, request.Body.UserId); err != nil {
		status, msg := httpx.MapHouseholdError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	return gen.AddHouseholdMember204Response{}, nil
}

// ListHouseholds processes GET /api/users/{userId}/households: it returns all
// households the user owns or is a member of, each with its members. The result
// is never nil.
func (s *Service) ListHouseholds(ctx context.Context, request gen.ListHouseholdsRequestObject) (gen.ListHouseholdsResponseObject, error) {
	list, err := s.household.ListMyHouseholds(ctx)
	if err != nil {
		status, msg := httpx.MapHouseholdError(err)
		return nil, httpx.NewAPIError(status, msg)
	}
	out := make([]gen.Household, 0, len(list))
	for _, hh := range list {
		out = append(out, ToHousehold(hh))
	}
	return gen.ListHouseholds200JSONResponse(out), nil
}

// GetHousehold processes GET /api/users/{userId}/households/{householdId}: it
// returns a single household with its members. A household the requester is not
// a member of yields 404 (membership is hidden, not rejected).
func (s *Service) GetHousehold(ctx context.Context, request gen.GetHouseholdRequestObject) (gen.GetHouseholdResponseObject, error) {
	full, err := s.household.GetHousehold(ctx, request.HouseholdId)
	if err != nil {
		// ErrNotMember and ErrNotFound both map to 404 here: the API does not
		// distinguish "not a member" from "doesn't exist" for visibility.
		switch {
		case errors.Is(err, household.ErrNotMember), errors.Is(err, repo.ErrNotFound):
			return nil, httpx.NewAPIError(http.StatusNotFound, "not found")
		case errors.Is(err, user.ErrNoUser):
			return nil, httpx.NewAPIError(http.StatusUnauthorized, "missing or invalid user identity")
		default:
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
		}
	}
	return gen.GetHousehold200JSONResponse(ToHousehold(full)), nil
}
