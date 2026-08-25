package entity

import "time"

// Source represents an uploaded document file (one row in sources table).
type Source struct {
	ID          string
	TenantID    string
	Filename    string
	ContentType string
	Size        int64
	Path        string
	SHA256      string
	UploadedAt  time.Time
}
