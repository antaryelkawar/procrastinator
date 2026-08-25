package data

import "time"

// Document represents one extracted document (one row in documents table).
type Document struct {
	ID              string
	TenantID        string
	SourceID        string
	AssetID         string
	DocType         string
	ExtractedFields map[string]any
	RawExtraction   string
	CreatedAt       time.Time
}

// DocumentWithSource is a Document with joined source metadata for the documents list endpoint.
type DocumentWithSource struct {
	Document
	SourceFilename   string
	SourceUploadedAt time.Time
}
