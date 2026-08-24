// Package httpapi provides the HTTP API (chi router + handlers) for the Life Manager backend.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"procrastinator-backend/internal/config"
	"procrastinator-backend/internal/ingest"
	"procrastinator-backend/internal/store/assets"
	"procrastinator-backend/internal/store/documents"
)

// Server holds the ingest service, asset repository, document repository,
// and max upload bytes limit.
type Server struct {
	svc      *ingest.Service
	assets   *assets.Repo
	docs     *documents.Repo
	maxBytes int64
}

// New constructs a Server with the ingest service, assets repo, and documents
// repo wired from the provided pool. If cfg.MaxUploadBytes is non-positive,
// the default 20 MiB limit is used.
func New(cfg config.Config, pool *pgxpool.Pool, ex ingest.Extractor) *Server {
	maxBytes := cfg.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 20971520 // 20 MiB
	}
	return &Server{
		svc:      ingest.NewService(cfg, pool, ex),
		assets:   assets.New(pool),
		docs:     documents.New(pool),
		maxBytes: maxBytes,
	}
}

// Routes registers all API endpoints and returns the chi router.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/api/documents", s.handleUpload)
	r.Get("/api/assets", s.handleListAssets)
	r.Get("/api/assets/{id}", s.handleGetAsset)
	r.Get("/api/assets/{id}/documents", s.handleAssetDocuments)
	return r
}

// writeJSON sets the Content-Type header, writes the HTTP status, and encodes v
// as JSON. Encode errors are ignored — all DTOs are basic types and cannot fail.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload, err := json.Marshal(v)
	if err != nil {
		// Unreachable: every DTO field is a basic type.
		return
	}
	_, _ = w.Write(payload)
}

// writeError writes a JSON error envelope {"error":"<msg>"} with the given HTTP status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: msg})
}
