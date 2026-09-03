package entity

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

// Asset represents an owner-scoped device record with structured core fields and open metadata.
type Asset struct {
	ID               string
	OwnerID          string
	Brand            *string
	Model            *string
	SerialNumber     *string
	NormSerial       *string
	NormBrand        *string
	NormModel        *string
	PurchaseDate     *time.Time
	WarrantyEnd      *time.Time
	Price            *string
	Currency         *string
	DocType          string
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
}
