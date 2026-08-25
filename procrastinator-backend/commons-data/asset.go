package data

import "time"

// DocTypeInvoice represents an invoice document type.
const DocTypeInvoice = "invoice"

// DocTypeWarranty represents a warranty document type.
const DocTypeWarranty = "warranty"

// DocTypeAMC represents an AMC (Annual Maintenance Contract) document type.
const DocTypeAMC = "amc"

// DocTypeOther represents other document types.
const DocTypeOther = "other"

// ValidDocType returns true if s is exactly one of the four document type constants.
func ValidDocType(s string) bool {
	return s == DocTypeInvoice || s == DocTypeWarranty || s == DocTypeAMC || s == DocTypeOther
}

// Asset represents a tenant-scoped device record with structured core fields and open metadata.
type Asset struct {
	ID           string
	TenantID     string
	Brand        *string
	Model        *string
	SerialNumber *string
	NormSerial   *string
	NormBrand    *string
	NormModel    *string
	PurchaseDate *time.Time
	WarrantyEnd  *time.Time
	Price        *string
	Currency     *string
	DocType      string
	Metadata     map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UpdateFields specifies which fields to mutate on an existing asset.
// Nil pointer means untouched; non-nil means applied (including deriving norm columns).
// Non-nil Metadata performs a shallow per-key merge overlay.
type UpdateFields struct {
	Brand        *string
	Model        *string
	SerialNumber *string
	PurchaseDate *time.Time
	WarrantyEnd  *time.Time
	Price        *string
	Currency     *string
	DocType      *string
	Metadata     map[string]any
}
