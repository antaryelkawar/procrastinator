package entity

// Document classification vocabulary (6 values, migration 00006).
// These classify the *document*, not the asset: the asset carries its own
// intrinsic AssetCategory.

// DocTypeInvoice represents an invoice document type.
const DocTypeInvoice = "invoice"

// DocTypeReceipt represents a receipt document type.
const DocTypeReceipt = "receipt"

// DocTypeWarranty represents a warranty document type.
const DocTypeWarranty = "warranty"

// DocTypeAMC represents an AMC (Annual Maintenance Contract) document type.
const DocTypeAMC = "amc"

// DocTypeStatement represents a statement (e.g. bank/credit) document type.
const DocTypeStatement = "statement"

// DocTypeOther represents other document types.
const DocTypeOther = "other"

// ValidDocType returns true if s is exactly one of the six document type
// constants (case-sensitive).
func ValidDocType(s string) bool {
	switch s {
	case DocTypeInvoice, DocTypeReceipt, DocTypeWarranty, DocTypeAMC,
		DocTypeStatement, DocTypeOther:
		return true
	}
	return false
}
