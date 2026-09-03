// Package household implements the owner-model household bounded context:
// household creation, membership management, and household visibility.
// It is user-scoped: every method resolves the bound user from the context
// first and fails closed (user.ErrNoUser) without touching any repository
// when it is absent.
package household

import (
	"context"
	"errors"
	"sort"
	"strings"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ErrInvalid is returned for validation failures: a blank display name.
var ErrInvalid = errors.New("household: invalid input")

// ErrNotMember is returned when the requester is not a member (or owner) of
// the target household. The API layer maps this sentinel to HTTP 403.
var ErrNotMember = errors.New("household: not a member of the household")

// HouseholdWithMembers pairs a household with its full member list.
type HouseholdWithMembers struct {
	Household entity.Household
	Members   []entity.HouseholdMember
}

// Service is the household application service. It is user-scoped: every
// method resolves the user from the context first and fails closed
// (user.ErrNoUser) without touching any repository when it is absent.
type Service struct {
	factory *repo.Factory
}

// New constructs a Service over the given repository factory.
func New(factory *repo.Factory) *Service {
	return &Service{factory: factory}
}

// CreateHousehold persists a new household owned by the bound user and
// auto-adds the creator as the first member. Blank (after TrimSpace)
// display names are rejected with ErrInvalid.
func (s *Service) CreateHousehold(ctx context.Context, displayName string) (entity.Household, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Household{}, err
	}
	if strings.TrimSpace(displayName) == "" {
		return entity.Household{}, ErrInvalid
	}
	var created entity.Household
	err = s.factory.InTx(ctx, func(ctx context.Context, r *repo.Repos) error {
		h, err := r.Households.Create(ctx, entity.Household{OwnerID: tid, DisplayName: displayName}, repo.Owner(tid))
		if err != nil {
			return err
		}
		created = h
		// The creator is auto-added as the first member.
		return r.Households.AddMember(ctx, h.ID, tid, repo.Owner(tid))
	})
	if err != nil {
		return entity.Household{}, err
	}
	return created, nil
}

// AddMember adds a registered user to a household. The check order is
// existence → membership → user-registered → add:
//
//   - the household must exist (Households.Exists, an RLS-bypassing probe);
//     an unknown household yields repo.ErrNotFound (HTTP 404).
//   - the requester must already be a member (or owner) of the household
//     (ErrNotMember otherwise, HTTP 403). This is checked before
//     user-registered so a non-member never learns whether the target user
//     exists.
//   - the target user must be registered; an unregistered target yields
//     repo.ErrNotFound (HTTP 404).
//   - the membership is then added idempotently.
func (s *Service) AddMember(ctx context.Context, householdID, userID string) error {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return err
	}
	// 1. Existence (RLS-bypassing): unknown household is a 404, distinct from
	// the 403 a non-member would otherwise get.
	exists, err := s.factory.Households.Exists(ctx, householdID, repo.Owner(tid))
	if err != nil {
		return err
	}
	if !exists {
		return repo.ErrNotFound
	}
	// 2. Membership: the requester must already belong to the household.
	ok, err := s.isMember(ctx, tid, householdID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotMember
	}
	// 3. User-registered: the target must be a known user.
	usrExists, err := s.factory.Users.Has(ctx, userID)
	if err != nil {
		return err
	}
	if !usrExists {
		return repo.ErrNotFound
	}
	// 4. Add (idempotent).
	return s.factory.Households.AddMember(ctx, householdID, userID, repo.Owner(tid))
}

// IsMember reports whether the bound user is a member of the given household.
func (s *Service) IsMember(ctx context.Context, householdID string) (bool, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return false, err
	}
	return s.isMember(ctx, tid, householdID)
}

// isMember reports whether userID is a member of householdID. It delegates to
// HouseholdsForUser and checks membership in the returned id set.
func (s *Service) isMember(ctx context.Context, userID, householdID string) (bool, error) {
	ids, err := s.factory.Households.HouseholdsForUser(ctx, userID, repo.Owner(userID))
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == householdID {
			return true, nil
		}
	}
	return false, nil
}

// GetHousehold returns the household together with its members. If the
// household is not visible to the requester (not owner, not member) the
// service returns ErrNotMember. If the requester has some household
// relationship (member of at least one household) and the target is unknown,
// ErrNotFound is returned.
func (s *Service) GetHousehold(ctx context.Context, householdID string) (HouseholdWithMembers, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return HouseholdWithMembers{}, err
	}
	// Determine if the requester belongs to any household.
	myHouseholds, err := s.factory.Households.HouseholdsForUser(ctx, tid, repo.Owner(tid))
	if err != nil {
		return HouseholdWithMembers{}, err
	}
	isMemberOfAny := len(myHouseholds) > 0

	h, err := s.factory.Households.GetHouseholdVisible(ctx, householdID, repo.Owner(tid))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			if !isMemberOfAny {
				// Requester has no household access at all: return ErrNotMember
				// (do not leak whether the household exists).
				return HouseholdWithMembers{}, ErrNotMember
			}
			// Requester has household access but this specific household is
			// not visible: ErrNotFound (they can see their own, this is a miss).
			return HouseholdWithMembers{}, repo.ErrNotFound
		}
		return HouseholdWithMembers{}, err
	}

	members, err := s.factory.Households.ListMembers(ctx, householdID, repo.Owner(tid))
	if err != nil {
		return HouseholdWithMembers{}, err
	}
	return HouseholdWithMembers{Household: h, Members: members}, nil
}

// ListMyHouseholds returns every household the bound user owns or is a member
// of (owned ∪ membered), ordered owned-first (households the user owns come
// before the rest). Each item carries its members. The result is never nil.
func (s *Service) ListMyHouseholds(ctx context.Context) ([]HouseholdWithMembers, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return nil, err
	}
	// Fetch the full visible set (owned + membered) atomically via the
	// member-aware visibility predicate.
	visible, err := s.factory.Households.ListHouseholdsVisible(ctx, repo.Owner(tid))
	if err != nil {
		return nil, err
	}

	// Order owned-first: households the user owns come before the rest.
	// stable sort preserves the repository's relative order within each group.
	sort.SliceStable(visible, func(i, j int) bool {
		ownI := visible[i].OwnerID == tid
		ownJ := visible[j].OwnerID == tid
		return ownI && !ownJ
	})

	out := make([]HouseholdWithMembers, 0, len(visible))
	for _, h := range visible {
		members, err := s.factory.Households.ListMembers(ctx, h.ID, repo.Owner(tid))
		if err != nil {
			return nil, err
		}
		out = append(out, HouseholdWithMembers{Household: h, Members: members})
	}
	return out, nil
}
