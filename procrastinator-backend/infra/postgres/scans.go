package postgres

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// rowScanner is the subset of pgx.Row / pgx.Rows used for scanning.
type rowScanner interface {
	Scan(dest ...any) error
}

// decodePayloadData decodes a payload column value ({"data": {...}}) into the
// data object, using json.Decoder.UseNumber() so JSON numbers stay exact
// (money round-trips to the exact decimal string). Missing/empty payload or
// missing "data" yields an empty map, never an error for that case.
func decodePayloadData(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var outer struct {
		Data map[string]any `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	if err := dec.Decode(&outer); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	if outer.Data == nil {
		return map[string]any{}, nil
	}
	return outer.Data, nil
}

// encodePayload encodes a data object as the payload column value.
func encodePayload(data map[string]any) ([]byte, error) {
	return json.Marshal(map[string]any{"data": data})
}

// scanAsset scans a row into an entity.Asset, mapping pgx.ErrNoRows to
// repo.ErrNotFound. Column order is the assets codec selectCols list; the
// data portion comes from the payload column via the codec.
func scanAsset(row rowScanner) (entity.Asset, error) {
	var a entity.Asset
	var id string
	var ownerHouseholdID *string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &a.OwnerID, &ownerHouseholdID, &deletedAt, &a.CreatedAt, &a.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Asset{}, repo.ErrNotFound
		}
		return entity.Asset{}, err
	}
	a.ID = id
	a.OwnerHouseholdID = ownerHouseholdID
	a.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.Asset{}, err
	}
	if err := assetCodec.unmarshal(data, &a); err != nil {
		return entity.Asset{}, err
	}
	return a, nil
}

// scanSource scans a row into an entity.Source, mapping pgx.ErrNoRows to
// repo.ErrNotFound.
func scanSource(row rowScanner) (entity.Source, error) {
	var s entity.Source
	var id string
	var ownerHouseholdID *string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &s.OwnerID, &ownerHouseholdID, &deletedAt, &s.CreatedAt, &s.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Source{}, repo.ErrNotFound
		}
		return entity.Source{}, err
	}
	s.ID = id
	s.OwnerHouseholdID = ownerHouseholdID
	s.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.Source{}, err
	}
	if err := sourceCodec.unmarshal(data, &s); err != nil {
		return entity.Source{}, err
	}
	return s, nil
}

// scanDocument scans a row into an entity.Document, mapping pgx.ErrNoRows to
// repo.ErrNotFound.
func scanDocument(row rowScanner) (entity.Document, error) {
	var d entity.Document
	var id string
	var ownerHouseholdID *string
	var assetID *string
	var deletedAt *time.Time
	var payload []byte
	// asset_id is nullable (ON DELETE SET NULL; NULL when a document is
	// asset-less or a pending duplicate not yet linked), so scan into a
	// pointer and fold NULL into the zero value — matching the DTO, which
	// renders AssetID == "" as asset_id: null.
	err := row.Scan(&id, &d.OwnerID, &ownerHouseholdID, &assetID, &d.SourceID,
		&deletedAt, &d.CreatedAt, &d.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Document{}, repo.ErrNotFound
		}
		return entity.Document{}, err
	}
	d.ID = id
	d.OwnerHouseholdID = ownerHouseholdID
	if assetID != nil {
		d.AssetID = *assetID
	}
	d.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.Document{}, err
	}
	if err := documentCodec.unmarshal(data, &d); err != nil {
		return entity.Document{}, err
	}
	return d, nil
}

// scanFinancialAccount scans a row into an entity.FinancialAccount, mapping
// pgx.ErrNoRows to repo.ErrNotFound.
func scanFinancialAccount(row rowScanner) (entity.FinancialAccount, error) {
	var a entity.FinancialAccount
	var id string
	var ownerHouseholdID *string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &a.OwnerID, &ownerHouseholdID, &deletedAt, &a.CreatedAt, &a.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.FinancialAccount{}, repo.ErrNotFound
		}
		return entity.FinancialAccount{}, err
	}
	a.ID = id
	a.OwnerHouseholdID = ownerHouseholdID
	a.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.FinancialAccount{}, err
	}
	if err := accountCodec.unmarshal(data, &a); err != nil {
		return entity.FinancialAccount{}, err
	}
	return a, nil
}

// scanMoneyMovement scans a row into an entity.MoneyMovement, mapping
// pgx.ErrNoRows to repo.ErrNotFound.
func scanMoneyMovement(row rowScanner) (entity.MoneyMovement, error) {
	var m entity.MoneyMovement
	var id string
	var ownerHouseholdID *string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &m.OwnerID, &ownerHouseholdID, &m.SourceAccountID,
		&m.DestinationAccountID, &m.ImportBatchID, &m.LinkedDocumentID,
		&deletedAt, &m.CreatedAt, &m.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.MoneyMovement{}, repo.ErrNotFound
		}
		return entity.MoneyMovement{}, err
	}
	m.ID = id
	m.OwnerHouseholdID = ownerHouseholdID
	m.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.MoneyMovement{}, err
	}
	if err := movementCodec.unmarshal(data, &m); err != nil {
		return entity.MoneyMovement{}, err
	}
	return m, nil
}

// scanImportBatch scans a row into an entity.ImportBatch, mapping
// pgx.ErrNoRows to repo.ErrNotFound.
func scanImportBatch(row rowScanner) (entity.ImportBatch, error) {
	var b entity.ImportBatch
	var id string
	var ownerHouseholdID *string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &b.OwnerID, &ownerHouseholdID, &b.AccountID, &b.SourceID,
		&deletedAt, &b.CreatedAt, &b.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.ImportBatch{}, repo.ErrNotFound
		}
		return entity.ImportBatch{}, err
	}
	b.ID = id
	b.OwnerHouseholdID = ownerHouseholdID
	b.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.ImportBatch{}, err
	}
	if err := importBatchCodec.unmarshal(data, &b); err != nil {
		return entity.ImportBatch{}, err
	}
	return b, nil
}

// scanImportLine scans a row into an entity.ImportLine, mapping
// pgx.ErrNoRows to repo.ErrNotFound.
func scanImportLine(row rowScanner) (entity.ImportLine, error) {
	var l entity.ImportLine
	var id string
	var ownerHouseholdID *string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &l.OwnerID, &ownerHouseholdID, &l.BatchID, &l.LineRef,
		&deletedAt, &l.CreatedAt, &l.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.ImportLine{}, repo.ErrNotFound
		}
		return entity.ImportLine{}, err
	}
	l.ID = id
	l.OwnerHouseholdID = ownerHouseholdID
	l.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.ImportLine{}, err
	}
	if err := importLineCodec.unmarshal(data, &l); err != nil {
		return entity.ImportLine{}, err
	}
	return l, nil
}

// scanReview scans a row into an entity.IngestReview, mapping pgx.ErrNoRows to
// repo.ErrNotFound.
func scanReview(row rowScanner) (entity.IngestReview, error) {
	var r entity.IngestReview
	var id string
	var ownerHouseholdID *string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &r.OwnerID, &ownerHouseholdID, &r.SourceID,
		&deletedAt, &r.CreatedAt, &r.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.IngestReview{}, repo.ErrNotFound
		}
		return entity.IngestReview{}, err
	}
	r.ID = id
	r.OwnerHouseholdID = ownerHouseholdID
	r.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.IngestReview{}, err
	}
	if err := reviewCodec.unmarshal(data, &r); err != nil {
		return entity.IngestReview{}, err
	}
	return r, nil
}

// scanHousehold scans a row into an entity.Household, mapping pgx.ErrNoRows to
// repo.ErrNotFound.
func scanHousehold(row rowScanner) (entity.Household, error) {
	var h entity.Household
	var id string
	var deletedAt *time.Time
	var payload []byte
	err := row.Scan(&id, &h.OwnerID, &deletedAt, &h.CreatedAt, &h.UpdatedAt, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Household{}, repo.ErrNotFound
		}
		return entity.Household{}, err
	}
	h.ID = id
	h.DeletedAt = deletedAt
	data, err := decodePayloadData(payload)
	if err != nil {
		return entity.Household{}, err
	}
	if err := householdCodec.unmarshal(data, &h); err != nil {
		return entity.Household{}, err
	}
	return h, nil
}
