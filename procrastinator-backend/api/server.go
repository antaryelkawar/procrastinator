package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/tenant"
	"procrastinator-backend/core/ingest"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/statement"
)

// Server serves the HTTP API surface over the ingest service, the ledger
// service, and the tenant-scoped data repositories.
type Server struct {
	svc      *ingest.Service
	factory  *repo.Factory
	ledger   *ledger.Service
	balancer ledger.BalanceQuerier
	maxBytes int64
	// statement serves the statement import endpoints (import-batches).
	statement *statement.Service
	// maxStatementBytes is the maximum accepted statement upload size in
	// bytes for the statement import endpoints.
	maxStatementBytes int64
}

// New constructs a Server. ledgerSvc and balancer serve the finance accounts
// and movements endpoints; maxBytes is the maximum accepted upload size in
// bytes, enforced both at the HTTP layer (MaxBytesReader) and inside the
// ingest service. statementSvc and maxStatementBytes serve the statement
// import endpoints (import-batches); the statement size limit is enforced
// both at the HTTP layer (MaxBytesReader) and inside the statement service.
func New(svc *ingest.Service, factory *repo.Factory, ledgerSvc *ledger.Service, balancer ledger.BalanceQuerier, maxBytes int64, statementSvc *statement.Service, maxStatementBytes int64) *Server {
	return &Server{
		svc:               svc,
		factory:           factory,
		ledger:            ledgerSvc,
		balancer:          balancer,
		maxBytes:          maxBytes,
		statement:         statementSvc,
		maxStatementBytes: maxStatementBytes,
	}
}

// tenantFromCtx extracts the tenant ID from ctx, failing the request with a
// 500 when the tenant is missing or invalid. The middleware guarantees a
// valid tenant, so a failure here is defensive.
func tenantFromCtx(w http.ResponseWriter, ctx context.Context) (string, bool) {
	tid, err := tenant.TenantFrom(ctx)
	if err != nil {
		// Unreachable invariant: the middleware guarantees a valid tenant.
		httpx.WriteError(w, http.StatusInternalServerError, "internal: no tenant in context")
		return "", false
	}
	return tid, true
}

// listDocumentsWithSource returns the documents attached to assetID visible to
// the user under the scope access rule, joined with their source metadata,
// ordered by created_at then id.
func (s *Server) listDocumentsWithSource(ctx context.Context, tid, assetID string) ([]entity.DocumentWithSource, error) {
	scoped, ok := s.factory.Documents.(scopedDocumentLister)
	if !ok {
		return nil, errDocumentsNotScopeAware
	}
	list, err := scoped.ListScoped(ctx, repo.Tenant(tid), repo.Where("asset_id", "=", assetID), repo.OrderBy("created_at, id"))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return []entity.DocumentWithSource{}, nil
	}
	out := make([]entity.DocumentWithSource, 0, len(list))
	for _, d := range list {
		src, err := s.factory.Sources.Get(ctx, d.SourceID, repo.Tenant(tid))
		if err != nil {
			return nil, err
		}
		dw := entity.DocumentWithSource{Document: d}
		dw.SourceFilename = src.Filename
		dw.SourceUploadedAt = src.UploadedAt
		out = append(out, dw)
	}
	return out, nil
}

// Routes returns the fully-wired router for the API.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// User-scoped routes (URL-path tenancy). The tenant middleware is applied
	// INSIDE the {userId} route group: chi does not expose route params to
	// top-level Use middleware, so the middleware must sit below the group
	// prefix for chi.URLParam(r, "userId") to resolve.
	r.Route("/api/users/{userId}", func(ur chi.Router) {
		ur.Use(httpx.TenantMiddleware(s.factory.Tenants))
		ur.Post("/documents", s.handleUpload)
		ur.Get("/assets", s.handleListAssets)
		ur.Get("/assets/{assetId}", s.handleGetAsset)
		ur.Get("/assets/{assetId}/documents", s.handleListDocuments)
	})

	// Finance routes (header tenancy, transitional): this feature is not yet on
	// URL-path tenancy. The same middleware resolves the user from the
	// X-Tenant-ID header when no {userId} path param is present.
	r.Route("/api/finance", func(fr chi.Router) {
		fr.Use(httpx.TenantMiddleware(s.factory.Tenants))
		fr.Post("/accounts", s.createAccount)
		fr.Get("/accounts", s.listAccounts)
		fr.Get("/accounts/{id}", s.getAccount)
		fr.Post("/movements", s.createMovement)
		fr.Get("/movements", s.listMovements)
		fr.Get("/movements/{id}", s.getMovement)
		fr.Patch("/movements/{id}", s.patchMovement)
		fr.Delete("/movements/{id}", s.deleteMovement)
		fr.Post("/movements/{id}/link", s.linkMovement)
		fr.Delete("/movements/{id}/link", s.unlinkMovement)
		fr.Post("/import-batches", s.createImportBatch)
		fr.Get("/import-batches", s.listImportBatches)
		fr.Get("/import-batches/{id}", s.getImportBatch)
		fr.Post("/import-batches/{id}/commit", s.commitImportBatch)
		fr.Post("/import-batches/{id}/discard", s.discardImportBatch)
	})

	return r
}
