package api

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/commons-data"
	"procrastinator-backend/commons-server/httpx"
	"procrastinator-backend/procrastinator-core/identity"
	"procrastinator-backend/procrastinator-core/ingest"
	"procrastinator-backend/procrastinator-infra/filestorage"
)

// assetJSON is the JSON representation of a data.Asset returned by the API.
type assetJSON struct {
	ID           string         `json:"id"`
	Brand        *string        `json:"brand,omitempty"`
	Model        *string        `json:"model,omitempty"`
	SerialNumber *string        `json:"serial_number,omitempty"`
	PurchaseDate *time.Time     `json:"purchase_date,omitempty"`
	WarrantyEnd  *time.Time     `json:"warranty_end,omitempty"`
	Price        *string        `json:"price,omitempty"`
	Currency     *string        `json:"currency,omitempty"`
	DocType      string         `json:"doc_type"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// documentJSON is the JSON representation of a data.DocumentWithSource
// returned by the API.
type documentJSON struct {
	ID               string    `json:"id"`
	DocType          string    `json:"doc_type"`
	SourceFilename   string    `json:"source_filename"`
	SourceUploadedAt time.Time `json:"source_uploaded_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// toAssetJSON converts a data.Asset into its JSON DTO. Metadata is
// initialized to an empty map so it marshals as {} rather than null.
func toAssetJSON(a data.Asset) assetJSON {
	metadata := a.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return assetJSON{
		ID:           a.ID,
		Brand:        a.Brand,
		Model:        a.Model,
		SerialNumber: a.SerialNumber,
		PurchaseDate: a.PurchaseDate,
		WarrantyEnd:  a.WarrantyEnd,
		Price:        a.Price,
		Currency:     a.Currency,
		DocType:      a.DocType,
		Metadata:     metadata,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}

// toDocumentJSON converts a data.DocumentWithSource into its JSON DTO.
func toDocumentJSON(d data.DocumentWithSource) documentJSON {
	return documentJSON{
		ID:               d.ID,
		DocType:          d.DocType,
		SourceFilename:   d.SourceFilename,
		SourceUploadedAt: d.SourceUploadedAt,
		CreatedAt:        d.CreatedAt,
	}
}

// handleUpload processes a multipart document upload: it enforces the size
// limit, reads the "file" part, and runs the full ingest pipeline.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBytes)

	if err := r.ParseMultipartForm(0); err != nil {
		if isMaxBytesErr(err) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "malformed multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer func() { _ = file.Close() }()

	payload, err := io.ReadAll(file)
	if err != nil {
		if isMaxBytesErr(err) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to read upload")
		return
	}

	asset, err := s.svc.Process(r.Context(), header.Filename, payload, header.Header.Get("Content-Type"))
	if err != nil {
		s.writeProcessError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAssetJSON(asset))
}

// writeProcessError maps an ingest service error onto the HTTP status
// contract. Checks run in priority order: the domain sentinels first, then
// the transport-level fallback.
func (s *Server) writeProcessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ingest.ErrTooLarge):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
	case errors.Is(err, filestorage.ErrUnsupportedType):
		httpx.WriteError(w, http.StatusUnsupportedMediaType, "unsupported file type")
	case errors.Is(err, ingest.ErrExtraction):
		httpx.WriteError(w, http.StatusBadGateway, "extraction failed")
	case errors.Is(err, identity.ErrNoIdentity):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no usable identity in document")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

// handleListAssets returns all assets visible to the requesting tenant.
func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := s.assets.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset list: internal error")
		return
	}

	out := make([]assetJSON, 0, len(assets))
	for _, a := range assets {
		out = append(out, toAssetJSON(a))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// handleGetAsset returns a single asset by ID.
func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	asset, err := s.assets.GetByID(r.Context(), id)
	if errors.Is(err, data.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "asset not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset: internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAssetJSON(asset))
}

// handleListDocuments returns all documents attached to one asset. The asset
// is looked up first so that unknown or foreign-tenant assets fail with 404
// instead of an empty list.
func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := s.assets.GetByID(r.Context(), id); errors.Is(err, data.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "asset not found")
		return
	} else if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "asset: internal error")
		return
	}

	docs, err := s.docs.ListByAsset(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "document list: internal error")
		return
	}

	out := make([]documentJSON, 0, len(docs))
	for _, d := range docs {
		out = append(out, toDocumentJSON(d))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// isMaxBytesErr reports whether err (or any error in its chain) is an
// *http.MaxBytesError.
func isMaxBytesErr(err error) bool {
	var mbErr *http.MaxBytesError
	return errors.As(err, &mbErr)
}
