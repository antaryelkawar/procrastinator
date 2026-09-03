package entity

import "time"

// AccountTypeBank represents a bank account type.
const AccountTypeBank = "bank"

// AccountTypeWallet represents a wallet account type.
const AccountTypeWallet = "wallet"

// AccountTypeCash represents a cash account type.
const AccountTypeCash = "cash"

// AccountTypeCreditCard represents a credit card account type.
const AccountTypeCreditCard = "credit_card"

// ValidAccountType returns true if s is exactly one of the four account type constants.
func ValidAccountType(s string) bool {
	return s == AccountTypeBank || s == AccountTypeWallet || s == AccountTypeCash || s == AccountTypeCreditCard
}

// FinancialAccount represents one owner-scoped account (one row in financial_accounts table).
type FinancialAccount struct {
	ID        string
	OwnerID   string
	// Name is required and must be non-empty. Non-emptiness is enforced by the
	// service layer (core/ledger), not by this entity. The entity imposes NO
	// name uniqueness: two accounts may share the same name — uniqueness is a
	// DB/service concern.
	Name               string
	Type               string
	Currency           string
	Institution        *string
	ExternalDescriptor *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
}
