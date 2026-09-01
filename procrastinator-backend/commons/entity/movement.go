package entity

import "time"

// KindExpense represents an expense movement kind.
const KindExpense = "expense"

// KindIncome represents an income movement kind.
const KindIncome = "income"

// KindTransfer represents a transfer movement kind.
const KindTransfer = "transfer"

// ValidKind returns true if s is exactly one of the three movement kind constants.
func ValidKind(s string) bool {
	return s == KindExpense || s == KindIncome || s == KindTransfer
}

// OriginManual represents a manually entered movement origin.
const OriginManual = "manual"

// OriginImport represents a movement imported from a statement batch.
const OriginImport = "import"

// LinkCreatorManual represents a manually created document link.
const LinkCreatorManual = "manual"

// LinkCreatorAuto represents an automatically created document link.
const LinkCreatorAuto = "auto"

// MoneyMovement represents one tenant-scoped ledger movement (one row in money_movements table).
// Money Amount is an exact-decimal string, never a float.
type MoneyMovement struct {
	ID                   string
	TenantID             string
	Kind                 string
	Amount               string
	Currency             string
	OccurredOn           time.Time
	RecordedAt           time.Time
	Description          string
	NormDescription      string
	Origin               string
	SourceAccountID      *string
	DestinationAccountID *string
	ImportBatchID        *string
	ImportLine           *int
	ExternalReference    *string
	LinkedDocumentID     *string
	LinkCreator          *string
	LinkConflicting      bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
