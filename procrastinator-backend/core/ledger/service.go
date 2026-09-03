// Package ledger implements the financial-ledger bounded context: user-scoped
// financial accounts, manual money movements, derived balances, and
// movement-document links. Money is exact-decimal strings; no floats.
package ledger

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ErrInvalid is returned for validation failures: bad field values, blank
// descriptions, invalid kind/account shapes, currency mismatches.
var ErrInvalid = errors.New("invalid input")

// ErrConflict is returned when an operation conflicts with existing state:
// deleting an imported movement, or re-linking an already-linked movement or
// document to a different counterpart.
var ErrConflict = errors.New("conflicting state")

// BalanceQuerier reports the derived balance of one account as an
// exact-decimal string. Implementations derive the balance from the
// account's movements; the ledger service never persists balances itself.
type BalanceQuerier interface {
	BalanceForAccount(ctx context.Context, accountID string, opts ...repo.Option) (string, error)
}

// Service is the financial-ledger application service. It is user-scoped:
// every method resolves the user from the context first and fails closed
// (user.ErrNoUser) without touching any repository when it is absent.
// Money is handled as exact-decimal strings throughout; no floats.
type Service struct {
	factory  *repo.Factory
	balancer BalanceQuerier
}

// New constructs a Service over the given repository factory and balancer.
func New(factory *repo.Factory, balancer BalanceQuerier) *Service {
	return &Service{factory: factory, balancer: balancer}
}

// AccountInput carries the caller-supplied fields for account creation.
// The repository assigns the ID, user ID, and timestamps.
type AccountInput struct {
	Name               string
	Type               string
	Currency           string
	Institution        *string
	ExternalDescriptor *string
}

// MovementInput carries the caller-supplied fields for manual movement
// creation. SourceAccountID / DestinationAccountID are "" when absent; the
// kind determines which one must be present.
type MovementInput struct {
	Kind                 string
	Amount               string
	Currency             string
	OccurredOn           time.Time
	Description          string
	SourceAccountID      string // "" when absent
	DestinationAccountID string // "" when absent
}

// MovementListFilter filters ListMovements results. AccountID matches the
// source OR destination; the date bounds are inclusive when non-nil.
type MovementListFilter struct {
	AccountID    string     // "" = no filter; matches source OR destination
	OccurredFrom *time.Time // nil = unbounded; inclusive
	OccurredTo   *time.Time // nil = unbounded; inclusive
}

// CreateAccount validates the input (name non-blank, valid type, valid
// currency) and persists a new account. There is deliberately no account
// update method: currency is immutable by construction.
func (s *Service) CreateAccount(ctx context.Context, in AccountInput) (entity.FinancialAccount, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.FinancialAccount{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return entity.FinancialAccount{}, ErrInvalid
	}
	if !entity.ValidAccountType(in.Type) {
		return entity.FinancialAccount{}, ErrInvalid
	}
	if !commons.IsValidCurrency(in.Currency) {
		return entity.FinancialAccount{}, ErrInvalid
	}
	acc := entity.FinancialAccount{
		Name:               in.Name,
		Type:               in.Type,
		Currency:           in.Currency,
		Institution:        in.Institution,
		ExternalDescriptor: in.ExternalDescriptor,
	}
	return s.factory.Accounts.Create(ctx, acc, repo.Owner(tid))
}

// ListAccounts returns all of the user's accounts in (created_at, id) order.
func (s *Service) ListAccounts(ctx context.Context) ([]entity.FinancialAccount, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return nil, err
	}
	return s.factory.Accounts.List(ctx, repo.Owner(tid), repo.OrderBy("created_at, id"))
}

// GetAccount returns the user's account with the given ID together with its
// derived balance. ErrNotFound propagates when the account is unknown or
// belongs to another user.
func (s *Service) GetAccount(ctx context.Context, id string) (entity.FinancialAccount, string, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.FinancialAccount{}, "", err
	}
	acc, err := s.factory.Accounts.Get(ctx, id, repo.Owner(tid))
	if err != nil {
		return entity.FinancialAccount{}, "", err
	}
	balance, err := s.balancer.BalanceForAccount(ctx, id, repo.Owner(tid))
	if err != nil {
		return entity.FinancialAccount{}, "", err
	}
	return acc, balance, nil
}

// CreateManualMovement validates the input (amount, currency, description,
// kind/account shape), fetches the referenced accounts (ErrNotFound
// propagates), verifies currency consistency, and persists the movement with
// OriginManual.
func (s *Service) CreateManualMovement(ctx context.Context, in MovementInput) (entity.MoneyMovement, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.MoneyMovement{}, err
	}
	if !commons.IsValidAmount(in.Amount) {
		return entity.MoneyMovement{}, ErrInvalid
	}
	if !commons.IsValidCurrency(in.Currency) {
		return entity.MoneyMovement{}, ErrInvalid
	}
	if strings.TrimSpace(in.Description) == "" {
		return entity.MoneyMovement{}, ErrInvalid
	}
	if err := validateKindShape(in.Kind, in.SourceAccountID, in.DestinationAccountID); err != nil {
		return entity.MoneyMovement{}, err
	}

	accounts := make(map[string]entity.FinancialAccount)
	if in.SourceAccountID != "" {
		acc, err := s.factory.Accounts.Get(ctx, in.SourceAccountID, repo.Owner(tid))
		if err != nil {
			return entity.MoneyMovement{}, err
		}
		accounts[in.SourceAccountID] = acc
	}
	if in.DestinationAccountID != "" {
		acc, err := s.factory.Accounts.Get(ctx, in.DestinationAccountID, repo.Owner(tid))
		if err != nil {
			return entity.MoneyMovement{}, err
		}
		accounts[in.DestinationAccountID] = acc
	}
	if err := currencyMatches(in.Kind, accounts, in.Currency); err != nil {
		return entity.MoneyMovement{}, err
	}

	var source, dest *string
	if in.SourceAccountID != "" {
		sourceID := in.SourceAccountID
		source = &sourceID
	}
	if in.DestinationAccountID != "" {
		destID := in.DestinationAccountID
		dest = &destID
	}
	mv := entity.MoneyMovement{
		Kind:                 in.Kind,
		Amount:               in.Amount,
		Currency:             in.Currency,
		OccurredOn:           in.OccurredOn,
		Description:          in.Description,
		NormDescription:      commons.NormalizeDescription(in.Description),
		Origin:               entity.OriginManual,
		SourceAccountID:      source,
		DestinationAccountID: dest,
	}
	return s.factory.Movements.Create(ctx, mv, repo.Owner(tid))
}

// ListMovements returns the user's movements in (created_at, id) order,
// filtered in memory by account (source OR destination) and inclusive
// occurred-on bounds. The result is never nil.
func (s *Service) ListMovements(ctx context.Context, f MovementListFilter) ([]entity.MoneyMovement, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return nil, err
	}
	all, err := s.factory.Movements.List(ctx, repo.Owner(tid), repo.OrderBy("created_at, id"))
	if err != nil {
		return nil, err
	}
	out := make([]entity.MoneyMovement, 0, len(all))
	for _, m := range all {
		if f.AccountID != "" && !reflect.DeepEqual(m.SourceAccountID, &f.AccountID) && !reflect.DeepEqual(m.DestinationAccountID, &f.AccountID) {
			continue
		}
		if f.OccurredFrom != nil && m.OccurredOn.Before(*f.OccurredFrom) {
			continue
		}
		if f.OccurredTo != nil && m.OccurredOn.After(*f.OccurredTo) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// GetMovement returns the user's movement with the given ID. ErrNotFound
// propagates when the movement is unknown or belongs to another user.
func (s *Service) GetMovement(ctx context.Context, id string) (entity.MoneyMovement, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.MoneyMovement{}, err
	}
	return s.factory.Movements.Get(ctx, id, repo.Owner(tid))
}

// PatchDescription replaces a movement's description (verbatim) and its
// normalized form. Works for any origin. Blank descriptions are rejected
// before any repository call.
func (s *Service) PatchDescription(ctx context.Context, id, desc string) (entity.MoneyMovement, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.MoneyMovement{}, err
	}
	if strings.TrimSpace(desc) == "" {
		return entity.MoneyMovement{}, ErrInvalid
	}
	mv, err := s.factory.Movements.Get(ctx, id, repo.Owner(tid))
	if err != nil {
		return entity.MoneyMovement{}, err
	}
	mv.Description = desc
	mv.NormDescription = commons.NormalizeDescription(desc)
	return s.factory.Movements.Update(ctx, mv, repo.Owner(tid))
}

// DeleteMovement deletes a manual movement. Imported movements are protected:
// deleting one returns ErrConflict and leaves the row untouched. ErrNotFound
// propagates for unknown or foreign-user IDs.
func (s *Service) DeleteMovement(ctx context.Context, id string) error {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return err
	}
	mv, err := s.factory.Movements.Get(ctx, id, repo.Owner(tid))
	if err != nil {
		return err
	}
	if mv.Origin == entity.OriginImport {
		return ErrConflict
	}
	return s.factory.Movements.Delete(ctx, id, repo.Owner(tid))
}

// Link associates a movement with a document, recording who created the link
// and whether the document's extracted price/currency disagrees with the
// movement. Re-linking the same pair is a no-op; linking a movement that is
// already linked to a different document, or a document already linked to
// another movement, returns ErrConflict. Neither side's values are ever
// overwritten.
func (s *Service) Link(ctx context.Context, movementID, documentID, creator string) error {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return err
	}
	mv, err := s.factory.Movements.Get(ctx, movementID, repo.Owner(tid))
	if err != nil {
		return err
	}
	doc, err := s.factory.Documents.Get(ctx, documentID, repo.Owner(tid))
	if err != nil {
		return err
	}
	if mv.LinkedDocumentID != nil {
		if *mv.LinkedDocumentID == documentID {
			return nil // idempotent no-op
		}
		return ErrConflict
	}
	taken, err := s.factory.Movements.List(ctx, repo.Owner(tid), repo.Where("linked_document_id", "=", documentID), repo.Limit(1))
	if err != nil {
		return err
	}
	if len(taken) > 0 {
		return ErrConflict
	}
	docID := documentID
	creatorLocal := creator
	mv.LinkedDocumentID = &docID
	mv.LinkCreator = &creatorLocal
	mv.LinkConflicting = linkConflicts(doc, mv)
	_, err = s.factory.Movements.Update(ctx, mv, repo.Owner(tid))
	return err
}

// Unlink clears a movement's document link. Unlinking a movement that has no
// link is a no-op (no repository update).
func (s *Service) Unlink(ctx context.Context, movementID string) error {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return err
	}
	mv, err := s.factory.Movements.Get(ctx, movementID, repo.Owner(tid))
	if err != nil {
		return err
	}
	if mv.LinkedDocumentID == nil {
		return nil
	}
	mv.LinkedDocumentID = nil
	mv.LinkCreator = nil
	mv.LinkConflicting = false
	_, err = s.factory.Movements.Update(ctx, mv, repo.Owner(tid))
	return err
}

// linkConflicts reports whether the document's extracted price and currency
// both exist and disagree with the movement's amount or currency.
func linkConflicts(doc entity.Document, mv entity.MoneyMovement) bool {
	price, okPrice := doc.ExtractedFields["price"].(string)
	currency, okCurrency := doc.ExtractedFields["currency"].(string)
	if !okPrice || !okCurrency {
		return false
	}
	return price != mv.Amount || currency != mv.Currency
}
