package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/internal/identity"
	"procrastinator-backend/internal/ingest"
	"procrastinator-backend/internal/store/assets"
	"procrastinator-backend/internal/store/documents"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// isTooLarge checks if an error or its wrapped cause is an http.MaxBytesError.
func isTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

// uploadStatus maps ingestion errors to HTTP status codes and messages.
func uploadStatus(err error) (int, string) {
	if errors.Is(err, ingest.ErrTooLarge) {
		return http.StatusRequestEntityTooLarge, "upload exceeds size limit"
	}
	if errors.Is(err, ingest.ErrUnsupportedType) {
		return http.StatusUnsupportedMediaType, "unsupported content type"
	}
	if errors.Is(err, ingest.ErrExtraction) {
		return http.StatusBadGateway, "document extraction failed"
	}
	if errors.Is(err, identity.ErrNoIdentity) {
		return http.StatusUnprocessableEntity, "no usable asset identity in document"
	}
	return http.StatusInternalServerError, "internal server error"
}

// datePtr returns a pointer to a date string formatted as "YYYY-MM-DD", or nil if the input time is nil or zero.
func datePtr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// -----------------------------------------------------------------------------
// DTOs for JSON marshaling
// -----------------------------------------------------------------------------

// assetJSON represents an asset in JSON format for API responses.
type assetJSON struct {
	ID           string  `json:"id"`
	Brand        *string `json:"brand,omitempty"`
	Model        *string `json:"model,omitempty"`
	SerialNumber *string `json:"serial_number,omitempty"`

	PurchaseDate  *string   `json:"purchase_date,omitempty"` // "2006-01-02" format
	Price         *string   `json:"price,omitempty"`         // Always a JSON string
	Currency      *string   `json:"currency,omitempty"`
	WarrantyStart *string   `json:"warranty_start,omitempty"` // "2006-01-02" format
	WarrantyEnd   *string   `json:"warranty_end,omitempty"`   // "2006-01-02" format
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// toAssetJSON converts an identity.Asset to assetJSON. Stored display values
// are passed through verbatim (no case normalization): the API mirrors the
// store, and serial identity is carried by the norm_serial column.
func toAssetJSON(a identity.Asset) assetJSON {
	return assetJSON{
		ID:            a.ID,
		Brand:         a.Brand,
		Model:         a.Model,
		SerialNumber:  a.SerialNumber,
		PurchaseDate:  datePtr(a.PurchaseDate),
		Price:         a.Price,
		Currency:      a.Currency,
		WarrantyStart: datePtr(a.WarrantyStart),
		WarrantyEnd:   datePtr(a.WarrantyEnd),
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

// toStoreAssetJSON converts an assets.Asset to assetJSON (same verbatim
// semantics as toAssetJSON).
func toStoreAssetJSON(a assets.Asset) assetJSON {
	return assetJSON{
		ID:            a.ID,
		Brand:         a.Brand,
		Model:         a.Model,
		SerialNumber:  a.SerialNumber,
		PurchaseDate:  datePtr(a.PurchaseDate),
		Price:         a.Price,
		Currency:      a.Currency,
		WarrantyStart: datePtr(a.WarrantyStart),
		WarrantyEnd:   datePtr(a.WarrantyEnd),
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

// toAssetList converts a slice of assets.Asset to a slice of assetJSON,
// ensuring a non-nil slice is always returned.
func toAssetList(as []assets.Asset) []assetJSON {
	if as == nil {
		return []assetJSON{}
	}
	out := make([]assetJSON, len(as))
	for i, a := range as {
		out[i] = toStoreAssetJSON(a)
	}
	return out
}

// documentJSON represents a document in JSON format for API responses.
type documentJSON struct {
	ID               string          `json:"id"`
	DocType          string          `json:"doc_type"`
	ExtractedFields  json.RawMessage `json:"extracted_fields"`
	SourceFilename   string          `json:"source_filename"`
	SourceUploadedAt time.Time       `json:"source_uploaded_at"`
	CreatedAt        time.Time       `json:"created_at"`
}

// toDocumentList converts a slice of documents.DocumentWithSource to a slice of documentJSON,
// ensuring a non-nil slice is always returned.
func toDocumentList(ds []documents.DocumentWithSource) []documentJSON {
	if ds == nil {
		return []documentJSON{}
	}
	out := make([]documentJSON, len(ds))
	for i, d := range ds {
		out[i] = documentJSON{
			ID:               d.ID,
			DocType:          string(d.DocType),
			ExtractedFields:  d.ExtractedFields,
			SourceFilename:   d.SourceFilename,
			SourceUploadedAt: d.SourceUploadedAt,
			CreatedAt:        d.CreatedAt,
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// Handlers
// -----------------------------------------------------------------------------

// handleUpload handles document uploads.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	// Wrap the body to limit upload size.
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBytes)

	// Parse multipart form.
	err := r.ParseMultipartForm(32 << 20) // 32 MiB maximum memory for multipart form
	if err != nil {
		if isTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart request")
		return
	}

	// Get the file from the form.
	file, header, err := r.FormFile("file")
	if err != nil {
		if isTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "missing required file field")
		return
	}
	defer file.Close()

	// Read file data.
	data, err := io.ReadAll(file)
	if err != nil {
		if isTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "failed to read upload")
		return
	}

	// Process the document.
	asset, err := s.svc.Process(r.Context(), header.Filename, data)
	if err != nil {
		status, msg := uploadStatus(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusCreated, toAssetJSON(asset))
}

// handleListAssets lists all assets.
func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	list, err := s.assets.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, toAssetList(list))
}

// handleGetAsset retrieves a single asset by ID.
func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	asset, err := s.assets.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, assets.ErrNotFound) {
			writeError(w, http.StatusNotFound, "asset not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, toStoreAssetJSON(asset))
}

// handleAssetDocuments lists documents associated with a specific asset.
func (s *Server) handleAssetDocuments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// First, verify the asset exists.
	_, err := s.assets.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, assets.ErrNotFound) {
			writeError(w, http.StatusNotFound, "asset not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Then, retrieve documents for the asset.
	docs, err := s.docs.ListByAsset(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, toDocumentList(docs))
}
