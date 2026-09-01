package api

import (
	"time"

	"procrastinator-backend/commons/entity"
)

// accountJSON is the JSON representation of a financial account returned by
// the API. Balance is the derived balance (exact-decimal string, "0" when the
// account has no movements); it is never client-settable.
type accountJSON struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Type               string    `json:"type"`
	Currency           string    `json:"currency"`
	Institution        *string   `json:"institution,omitempty"`
	ExternalDescriptor *string   `json:"external_descriptor,omitempty"`
	Balance            string    `json:"balance"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// movementJSON is the JSON representation of a money movement returned by the
// API. Amount and the balance-related values are exact-decimal strings; import
// provenance fields appear only for movements with origin "import"; the link
// fields appear only when a document link exists.
type movementJSON struct {
	ID                   string    `json:"id"`
	Kind                 string    `json:"kind"`
	Amount               string    `json:"amount"`
	Currency             string    `json:"currency"`
	OccurredOn           string    `json:"occurred_on"`
	RecordedAt           time.Time `json:"recorded_at"`
	Description          string    `json:"description"`
	Origin               string    `json:"origin"`
	SourceAccountID      *string   `json:"source_account_id,omitempty"`
	DestinationAccountID *string   `json:"destination_account_id,omitempty"`
	ImportBatchID        *string   `json:"import_batch_id,omitempty"`
	ImportLine           *int      `json:"import_line,omitempty"`
	ExternalReference    *string   `json:"external_reference,omitempty"`
	LinkedDocumentID     *string   `json:"linked_document_id,omitempty"`
	LinkCreator          *string   `json:"link_creator,omitempty"`
	LinkConflicting      bool      `json:"link_conflicting"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// toAccountJSON converts an entity.FinancialAccount into its JSON DTO with the
// given derived balance.
func toAccountJSON(a entity.FinancialAccount, balance string) accountJSON {
	return accountJSON{
		ID:                 a.ID,
		Name:               a.Name,
		Type:               a.Type,
		Currency:           a.Currency,
		Institution:        a.Institution,
		ExternalDescriptor: a.ExternalDescriptor,
		Balance:            balance,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

// toMovementJSON converts an entity.MoneyMovement into its JSON DTO. OccurredOn
// is rendered as a "2006-01-02" date string.
func toMovementJSON(m entity.MoneyMovement) movementJSON {
	return movementJSON{
		ID:                   m.ID,
		Kind:                 m.Kind,
		Amount:               m.Amount,
		Currency:             m.Currency,
		OccurredOn:           m.OccurredOn.Format("2006-01-02"),
		RecordedAt:           m.RecordedAt,
		Description:          m.Description,
		Origin:               m.Origin,
		SourceAccountID:      m.SourceAccountID,
		DestinationAccountID: m.DestinationAccountID,
		ImportBatchID:        m.ImportBatchID,
		ImportLine:           m.ImportLine,
		ExternalReference:    m.ExternalReference,
		LinkedDocumentID:     m.LinkedDocumentID,
		LinkCreator:          m.LinkCreator,
		LinkConflicting:      m.LinkConflicting,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// importLineJSON is the JSON representation of one parsed statement import
// line. OccurredOn is a "2006-01-02" date string; Amount is an exact-decimal
// string; the nullable line fields are omitted when null/empty.
type importLineJSON struct {
	LineRef           int     `json:"line_ref"`
	RawLine           string  `json:"raw_line"`
	OccurredOn        *string `json:"occurred_on,omitempty"`
	Amount            *string `json:"amount,omitempty"`
	Direction         *string `json:"direction,omitempty"`
	Description       *string `json:"description,omitempty"`
	ExternalReference *string `json:"external_reference,omitempty"`
	Status            string  `json:"status"`
	ErrorReason       *string `json:"error_reason,omitempty"`
}

// importSourceJSON is the JSON representation of the uploaded statement's
// source metadata.
type importSourceJSON struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// importBatchJSON is the JSON representation of a statement import batch with
// its parsed lines (upload, get, and discard responses carry the lines; list
// responses may omit them).
type importBatchJSON struct {
	ID                   string           `json:"id"`
	State                string           `json:"state"`
	AccountID            string           `json:"account_id"`
	Source               importSourceJSON `json:"source"`
	Filename             string           `json:"filename"`
	Format               string           `json:"format"`
	LineCountValid       int              `json:"line_count_valid"`
	LineCountDuplicate   int              `json:"line_count_duplicate"`
	LineCountPossibleDup int              `json:"line_count_possible_dup"`
	LineCountError       int              `json:"line_count_error"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
	Lines                []importLineJSON `json:"lines"`
}

// commitSummaryJSON is the JSON response of a batch commit.
type commitSummaryJSON struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

// toImportLineJSON converts an entity.ImportLine into its JSON DTO. OccurredOn
// is rendered as a "2006-01-02" date string when set.
func toImportLineJSON(l entity.ImportLine) importLineJSON {
	var occurredOn *string
	if l.OccurredOn != nil {
		s := l.OccurredOn.Format("2006-01-02")
		occurredOn = &s
	}
	return importLineJSON{
		LineRef:           l.LineRef,
		RawLine:           l.RawLine,
		OccurredOn:        occurredOn,
		Amount:            l.Amount,
		Direction:         l.Direction,
		Description:       l.Description,
		ExternalReference: l.ExternalReference,
		Status:            l.Status,
		ErrorReason:       l.ErrorReason,
	}
}

// toImportSourceJSON converts an entity.Source into its JSON DTO.
func toImportSourceJSON(src entity.Source) importSourceJSON {
	return importSourceJSON{
		ID:          src.ID,
		Filename:    src.Filename,
		ContentType: src.ContentType,
		Size:        src.Size,
		SHA256:      src.SHA256,
		UploadedAt:  src.UploadedAt,
	}
}

// toImportBatchJSON converts an entity.ImportBatch plus its lines and source
// into the batch JSON DTO. A nil lines slice yields a nil Lines field (list
// responses); a non-nil slice (even empty) yields a non-nil Lines.
func toImportBatchJSON(batch entity.ImportBatch, lines []entity.ImportLine, src entity.Source) importBatchJSON {
	var outLines []importLineJSON
	if lines != nil {
		outLines = make([]importLineJSON, len(lines))
		for i, l := range lines {
			outLines[i] = toImportLineJSON(l)
		}
	}
	return importBatchJSON{
		ID:                   batch.ID,
		State:                batch.State,
		AccountID:            batch.AccountID,
		Source:               toImportSourceJSON(src),
		Filename:             batch.Filename,
		Format:               batch.Format,
		LineCountValid:       batch.LineCountValid,
		LineCountDuplicate:   batch.LineCountDuplicate,
		LineCountPossibleDup: batch.LineCountPossibleDup,
		LineCountError:       batch.LineCountError,
		CreatedAt:            batch.CreatedAt,
		UpdatedAt:            batch.UpdatedAt,
		Lines:                outLines,
	}
}
