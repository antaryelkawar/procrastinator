package finance

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
)

// ToAccount converts an entity.FinancialAccount into the generated gen.Account DTO
// with the given derived balance. The payload data fields live under the nested
// `data` object; `account_type` mirrors the payload key (renamed from `type`).
func ToAccount(a entity.FinancialAccount, balance string) gen.Account {
	return gen.Account{
		Id:                 a.ID,
		OwnerHouseholdId:   a.OwnerHouseholdID,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
		Data: struct {
			AccountType        string  `json:"account_type"`
			Balance            string  `json:"balance"`
			Currency           string  `json:"currency"`
			ExternalDescriptor *string `json:"external_descriptor,omitempty"`
			Institution        *string `json:"institution,omitempty"`
			Name               string  `json:"name"`
		}{
			Name:               a.Name,
			AccountType:        a.Type,
			Currency:           a.Currency,
			Institution:        a.Institution,
			ExternalDescriptor: a.ExternalDescriptor,
			Balance:            balance,
		},
	}
}

// ToMovement converts an entity.MoneyMovement into the generated gen.Movement DTO.
// OccurredOn is rendered as an openapi_types.Date (marshals as "2006-01-02").
// The payload data fields live under the nested `data` object.
func ToMovement(m entity.MoneyMovement) gen.Movement {
	return gen.Movement{
		Id:                   m.ID,
		SourceAccountId:      m.SourceAccountID,
		DestinationAccountId: m.DestinationAccountID,
		ImportBatchId:        m.ImportBatchID,
		LinkedDocumentId:     m.LinkedDocumentID,
		OwnerHouseholdId:     m.OwnerHouseholdID,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
		Data: struct {
			Amount            string             `json:"amount"`
			Currency          string             `json:"currency"`
			Description       string             `json:"description"`
			ExternalReference *string            `json:"external_reference,omitempty"`
			ImportLine        *int               `json:"import_line,omitempty"`
			Kind              string             `json:"kind"`
			LinkConflicting   bool               `json:"link_conflicting"`
			LinkCreator       *string            `json:"link_creator,omitempty"`
			OccurredOn        openapi_types.Date `json:"occurred_on"`
			Origin            string             `json:"origin"`
			RecordedAt        time.Time          `json:"recorded_at"`
		}{
			Kind:              m.Kind,
			Amount:            m.Amount,
			Currency:          m.Currency,
			OccurredOn:        openapi_types.Date{Time: m.OccurredOn},
			RecordedAt:        m.RecordedAt,
			Description:       m.Description,
			Origin:            m.Origin,
			ImportLine:        m.ImportLine,
			ExternalReference: m.ExternalReference,
			LinkCreator:       m.LinkCreator,
			LinkConflicting:   m.LinkConflicting,
		},
	}
}

// ToImportLine converts an entity.ImportLine into the generated gen.ImportLine DTO.
// OccurredOn is rendered as an openapi_types.Date when set.
func ToImportLine(l entity.ImportLine) gen.ImportLine {
	var occurredOn *openapi_types.Date
	if l.OccurredOn != nil {
		d := openapi_types.Date{Time: *l.OccurredOn}
		occurredOn = &d
	}
	return gen.ImportLine{
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

// ToImportSource converts an entity.Source into the generated gen.ImportSource DTO.
func ToImportSource(src entity.Source) gen.ImportSource {
	return gen.ImportSource{
		Id:          src.ID,
		Filename:    src.Filename,
		ContentType: src.ContentType,
		Size:        src.Size,
		Sha256:      src.SHA256,
		UploadedAt:  src.UploadedAt,
	}
}

// ToImportBatch converts an entity.ImportBatch plus its lines and source into the
// generated gen.ImportBatch DTO. A nil lines slice yields a nil Lines field (list
// responses); a non-nil slice (even empty) yields a non-nil Lines.
func ToImportBatch(batch entity.ImportBatch, lines []entity.ImportLine, src entity.Source) gen.ImportBatch {
	var outLines []gen.ImportLine
	if lines != nil {
		outLines = make([]gen.ImportLine, len(lines))
		for i, l := range lines {
			outLines[i] = ToImportLine(l)
		}
	}
	return gen.ImportBatch{
		Id:        batch.ID,
		AccountId: batch.AccountID,
		Source:    ToImportSource(src),
		CreatedAt: batch.CreatedAt,
		UpdatedAt: batch.UpdatedAt,
		Data: struct {
			Filename             string            `json:"filename"`
			Format               string            `json:"format"`
			LineCountDuplicate   int               `json:"line_count_duplicate"`
			LineCountError       int               `json:"line_count_error"`
			LineCountPossibleDup int               `json:"line_count_possible_dup"`
			LineCountValid       int               `json:"line_count_valid"`
			Lines                *[]gen.ImportLine `json:"lines,omitempty"`
			State                string            `json:"state"`
		}{
			State:                batch.State,
			Filename:             batch.Filename,
			Format:               batch.Format,
			LineCountValid:       batch.LineCountValid,
			LineCountDuplicate:   batch.LineCountDuplicate,
			LineCountPossibleDup: batch.LineCountPossibleDup,
			LineCountError:       batch.LineCountError,
			Lines:                &outLines,
		},
	}
}
