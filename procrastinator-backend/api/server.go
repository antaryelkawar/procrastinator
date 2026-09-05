package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/household"
	"procrastinator-backend/core/ingest"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/statement"
)

// Compile-time assertion that Server implements the generated strict
// ServerInterface.
var _ gen.StrictServerInterface = (*Server)(nil)

// Server serves the HTTP API surface over the ingest service, the ledger
// service, and the owner-scoped data repositories.
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
	// household serves the household endpoints (create, add-member, list, get).
	household *household.Service
}

// New constructs a Server. ledgerSvc and balancer serve the finance accounts
// and movements endpoints; maxBytes is the maximum accepted upload size in
// bytes, enforced both at the HTTP layer (MaxBytesReader) and inside the
// ingest service. statementSvc and maxStatementBytes serve the statement
// import endpoints (import-batches); the statement size limit is enforced
// both at the HTTP layer (MaxBytesReader) and inside the statement service.
func New(svc *ingest.Service, factory *repo.Factory, ledgerSvc *ledger.Service, balancer ledger.BalanceQuerier, maxBytes int64, statementSvc *statement.Service, maxStatementBytes int64, householdSvc *household.Service) *Server {
	return &Server{
		svc:               svc,
		factory:           factory,
		ledger:            ledgerSvc,
		balancer:          balancer,
		maxBytes:          maxBytes,
		statement:         statementSvc,
		maxStatementBytes: maxStatementBytes,
		household:         householdSvc,
	}
}

// listDocumentsWithSource returns the documents attached to assetID visible to
// the user under the scope access rule, joined with their source metadata,
// ordered by created_at then id.
func (s *Server) listDocumentsWithSource(ctx context.Context, tid, assetID string) ([]entity.DocumentWithSource, error) {
	list, err := s.factory.Documents.List(ctx, repo.Owner(tid), repo.Where("asset_id", "=", assetID), repo.OrderBy("created_at, id"))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return []entity.DocumentWithSource{}, nil
	}
	out := make([]entity.DocumentWithSource, 0, len(list))
	for _, d := range list {
		// Fetch the source through the same scope access rule as the document.
		// A household document's source is owned by the same household, so a
		// member must be able to read it even though its owner_id is the
		// owning user rather than the requesting member (the generic
		// owner-scoped Get would hide it from members).
		srcs, err := s.factory.Sources.List(ctx, repo.Owner(tid), repo.Where("id", "=", d.SourceID))
		if err != nil {
			return nil, err
		}
		if len(srcs) == 0 {
			// Unreachable when the document is visible (the source shares its
			// scope); fail closed rather than emit a partial row.
			return nil, repo.ErrNotFound
		}
		dw := entity.DocumentWithSource{Document: d}
		dw.SourceFilename = srcs[0].Filename
		dw.SourceUploadedAt = srcs[0].UploadedAt
		out = append(out, dw)
	}
	return out, nil
}

// requestErrorFunc is the centralized RequestErrorHandlerFunc for the strict
// handler: it emits the {"error": string} envelope for every binding/parse
// failure (malformed JSON or multipart decode → 400, *http.MaxBytesError →
// 413) instead of the generated plain-text default.
var requestErrorFunc = func(w http.ResponseWriter, _ *http.Request, err error) {
	if isMaxBytesErr(err) {
		writeErrorEnvelope(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		return
	}
	writeErrorEnvelope(w, http.StatusBadRequest, "malformed request")
}

// responseErrorFunc is the centralized ResponseErrorHandlerFunc for the
// strict handler: it unwraps *apiError and emits the {"error": string}
// envelope with the mapped status, falling back to 500 for any other error.
// When responseErrorDebug is set (flipped per-test by the api test suite),
// the 500 fallback message carries the underlying error for diagnosis.
var responseErrorDebug = false

func responseErrorFunc(w http.ResponseWriter, _ *http.Request, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeErrorEnvelope(w, ae.status, ae.msg)
		return
	}
	if responseErrorDebug {
		writeErrorEnvelope(w, http.StatusInternalServerError, "internal error: "+err.Error())
		return
	}
	writeErrorEnvelope(w, http.StatusInternalServerError, "internal error")
}

// Routes returns the fully-wired router for the API. All routes are
// user-scoped by path under /api/users/{userId} and resolved by a single
// user middleware. Route registration is derived from the OpenAPI document
// via the generated gen.HandlerWithOptions; handlers are bound and encoded
// by the generated strict server (gen.NewStrictHandlerWithOptions) with the
// centralized request/response error funcs.
//
// The UserMiddleware rejects an empty or malformed {userId} with a
// {"error": string} 400 envelope before the strict handler runs. The {id}
// segments (asset/account/movement/batch/household IDs) are not rejected: an
// empty {id} routes to the handler, which resolves it as not-found (404) —
// matching the pre-change contract.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	return gen.HandlerWithOptions(
		gen.NewStrictHandlerWithOptions(s, nil, gen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  requestErrorFunc,
			ResponseErrorHandlerFunc: responseErrorFunc,
		}),
		gen.ChiServerOptions{
			BaseRouter: r,
			Middlewares: []gen.MiddlewareFunc{
				httpx.UserMiddleware(s.factory.Users),
				httpx.MaxBodyMiddleware(s.maxBytes, s.maxStatementBytes),
			},
		},
	)
}
