package api

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
)

// toAccount converts an entity.FinancialAccount into the generated gen.Account DTO
// with the given derived balance.
func toAccount(a entity.FinancialAccount, balance string) gen.Account {
	return gen.Account{
		Id:                 a.ID,
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

// toMovement converts an entity.MoneyMovement into the generated gen.Movement DTO.
// OccurredOn is rendered as an openapi_types.Date (marshals as "2006-01-02").
func toMovement(m entity.MoneyMovement) gen.Movement {
	return gen.Movement{
		Id:                   m.ID,
		Kind:                 m.Kind,
		Amount:               m.Amount,
		Currency:             m.Currency,
		OccurredOn:           openapi_types.Date{Time: m.OccurredOn},
		RecordedAt:           m.RecordedAt,
		Description:          m.Description,
		Origin:               m.Origin,
		SourceAccountId:      m.SourceAccountID,
		DestinationAccountId: m.DestinationAccountID,
		ImportBatchId:        m.ImportBatchID,
		ImportLine:           m.ImportLine,
		ExternalReference:    m.ExternalReference,
		LinkedDocumentId:     m.LinkedDocumentID,
		LinkCreator:          m.LinkCreator,
		LinkConflicting:      m.LinkConflicting,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// toImportLine converts an entity.ImportLine into the generated gen.ImportLine DTO.
// OccurredOn is rendered as an openapi_types.Date when set.
func toImportLine(l entity.ImportLine) gen.ImportLine {
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

// toImportSource converts an entity.Source into the generated gen.ImportSource DTO.
func toImportSource(src entity.Source) gen.ImportSource {
	return gen.ImportSource{
		Id:          src.ID,
		Filename:    src.Filename,
		ContentType: src.ContentType,
		Size:        src.Size,
		Sha256:      src.SHA256,
		UploadedAt:  src.UploadedAt,
	}
}

// toImportBatch converts an entity.ImportBatch plus its lines and source into the
// generated gen.ImportBatch DTO. A nil lines slice yields a nil Lines field (list
// responses); a non-nil slice (even empty) yields a non-nil Lines.
func toImportBatch(batch entity.ImportBatch, lines []entity.ImportLine, src entity.Source) gen.ImportBatch {
	var outLines []gen.ImportLine
	if lines != nil {
		outLines = make([]gen.ImportLine, len(lines))
		for i, l := range lines {
			outLines[i] = toImportLine(l)
		}
	}
	return gen.ImportBatch{
		Id:                   batch.ID,
		State:                batch.State,
		AccountId:            batch.AccountID,
		Source:               toImportSource(src),
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
