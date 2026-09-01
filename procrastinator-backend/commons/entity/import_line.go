package entity

import "time"

// LineStatusValid marks a line that parsed and validated cleanly.
const LineStatusValid = "valid"

// LineStatusDuplicate marks a line that is an exact duplicate of an existing line.
const LineStatusDuplicate = "duplicate"

// LineStatusPossibleDuplicate marks a line that may duplicate an existing line but is not confirmed.
const LineStatusPossibleDuplicate = "possible-duplicate"

// LineStatusError marks a line that failed parsing or validation.
const LineStatusError = "error"

// ValidStatus returns true if s is exactly one of the four line status constants.
func ValidStatus(s string) bool {
	return s == LineStatusValid || s == LineStatusDuplicate || s == LineStatusPossibleDuplicate || s == LineStatusError
}

// ImportLine represents one parsed line of an import batch (one row in import_lines table).
type ImportLine struct {
	ID                string
	TenantID          string
	BatchID           string
	LineRef           int
	RawLine           string
	OccurredOn        *time.Time
	Amount            *string
	Direction         *string
	Description       *string
	NormDescription   *string
	ExternalReference *string
	Status            string
	ErrorReason       *string
	CreatedAt         time.Time
}
