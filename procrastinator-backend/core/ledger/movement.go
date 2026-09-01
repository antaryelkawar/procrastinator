package ledger

import (
	"procrastinator-backend/commons/entity"
)

// validateKindShape enforces the movement kind/account-shape rules:
// expense -> source account present, destination absent;
// income  -> destination present, source absent;
// transfer-> both present and distinct. Unknown kind -> error.
func validateKindShape(kind, sourceID, destID string) error {
	switch kind {
	case entity.KindExpense:
		if sourceID == "" || destID != "" {
			return ErrInvalid
		}
	case entity.KindIncome:
		if destID == "" || sourceID != "" {
			return ErrInvalid
		}
	case entity.KindTransfer:
		if sourceID == "" || destID == "" || sourceID == destID {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// currencyMatches verifies that every account in the map has the same currency
// as the movement. Caller contract: the map holds exactly the accounts
// referenced by the validated kind (keyed by account ID), after shape
// validation and fetch. An unknown kind or any currency mismatch returns
// ErrInvalid; an empty map with a valid kind returns nil.
func currencyMatches(kind string, accounts map[string]entity.FinancialAccount, currency string) error {
	if !entity.ValidKind(kind) {
		return ErrInvalid
	}
	for _, acc := range accounts {
		if acc.Currency != currency {
			return ErrInvalid
		}
	}
	return nil
}
