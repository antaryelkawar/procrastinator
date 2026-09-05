package entity

import "time"

// Document represents one extracted document (one row in documents table).
type Document struct {
	ID               string
	OwnerID          string
	SourceID         string
	AssetID          string
	DocType          string
	ExtractedFields  map[string]any
	RawExtraction    string
	CreatedAt        time.Time
	// Confidence is the LLM extraction confidence (0.0–1.0) carried by the
	// document; nil means absent.
	Confidence *float64
	// OwnerHouseholdID is the owner_household_id column; nil means NULL
	// (a personal row has no household owner).
	OwnerHouseholdID *string
}

// DocumentWithSource is a Document with joined source metadata for the documents list endpoint.
type DocumentWithSource struct {
	Document
	SourceFilename   string
	SourceUploadedAt time.Time
}
