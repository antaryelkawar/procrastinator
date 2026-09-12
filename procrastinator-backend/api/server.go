package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"procrastinator-backend/api/add"
	"procrastinator-backend/api/assets"
	"procrastinator-backend/api/auth"
	"procrastinator-backend/api/documents"
	"procrastinator-backend/api/finance"
	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/households"
	"procrastinator-backend/api/httpx"
	apisearch "procrastinator-backend/api/search"
	"procrastinator-backend/api/reviews"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/config"
	"procrastinator-backend/core/household"
	"procrastinator-backend/core/ledger"
	"procrastinator-backend/core/lifecycle"
	"procrastinator-backend/core/processing"
	"procrastinator-backend/core/review"
	"procrastinator-backend/core/search"
	"procrastinator-backend/core/statement"
)

// Compile-time assertion that Server implements the generated strict
// ServerInterface.
var _ gen.StrictServerInterface = (*Server)(nil)

// Server serves the HTTP API surface over the processing service, the
// lifecycle service, the ledger service, and the owner-scoped data
// repositories. It is the composition root: it wires the per-domain
// subpackage services and delegates each handler method to the appropriate
// service.
type Server struct {
	svc      *processing.Service
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
	// search serves the search endpoints (quick + paged).
	search *search.Service
	// review serves the ingest review endpoints (list, get, approve, reject).
	review *review.Service
	// lifecycle serves the asset lifecycle endpoints (delete, restore, merge, patch).
	lifecycle *lifecycle.Service

	// Subpackage services (wired in New).
	assetsSvc     *assets.Service
	addSvc        *add.Service
	searchSvc     *apisearch.Service
	financeSvc    *finance.Service
	reviewsSvc    *reviews.Service
	householdsSvc *households.Service
	// documentsSvc serves the documents-section endpoints (list, reprocess, keep, delete).
	documentsSvc *documents.Service
	// basicAuthUsers holds the parsed Basic Auth credential pairs enforced by
	// the auth middleware in Routes(). It is set at construction via New
	// (main passes cfg.BasicAuthUsers); a nil slice rejects every request.
	basicAuthUsers []config.Credential
}

// New constructs a Server. ledgerSvc and balancer serve the finance accounts
// and movements endpoints; maxBytes is the maximum accepted upload size in
// bytes, enforced both at the HTTP layer (MaxBytesReader) and inside the
// processing service. statementSvc and maxStatementBytes serve the statement
// import endpoints (import-batches); the statement size limit is enforced
// both at the HTTP layer (MaxBytesReader) and inside the statement service.
// searchSvc serves the search endpoints; reviewSvc serves the ingest review
// endpoints; lifecycleSvc serves the asset lifecycle endpoints (delete,
// restore, merge, patch). basicAuthUsers are the parsed config credentials
// enforced by the auth middleware in Routes() (a nil slice rejects every
// request; production passes cfg.BasicAuthUsers from main).
func New(svc *processing.Service, factory *repo.Factory, ledgerSvc *ledger.Service, balancer ledger.BalanceQuerier, maxBytes int64, statementSvc *statement.Service, maxStatementBytes int64, householdSvc *household.Service, searchSvc *search.Service, reviewSvc *review.Service, lifecycleSvc *lifecycle.Service, basicAuthUsers []config.Credential) *Server {
	s := &Server{
		svc:               svc,
		factory:           factory,
		ledger:            ledgerSvc,
		balancer:          balancer,
		maxBytes:          maxBytes,
		statement:         statementSvc,
		maxStatementBytes: maxStatementBytes,
		household:         householdSvc,
		search:            searchSvc,
		review:            reviewSvc,
		lifecycle:         lifecycleSvc,
		basicAuthUsers:    basicAuthUsers,
	}
	s.assetsSvc = assets.New(factory, lifecycleSvc)
	s.addSvc = add.New(svc, factory, statementSvc)
	s.searchSvc = apisearch.New(searchSvc)
	s.financeSvc = finance.New(ledgerSvc, balancer, statementSvc, factory)
	s.reviewsSvc = reviews.New(reviewSvc, factory)
	s.householdsSvc = households.New(householdSvc)
	s.documentsSvc = documents.New(factory, svc)
	return s
}

// requestErrorFunc is the centralized RequestErrorHandlerFunc for the strict
// handler: it emits the {"error": string} envelope for every binding/parse
// failure (malformed JSON or multipart decode → 400, *http.MaxBytesError →
// 413) instead of the generated plain-text default.
var requestErrorFunc = func(w http.ResponseWriter, _ *http.Request, err error) {
	if httpx.IsMaxBytesErr(err) {
		httpx.WriteErrorEnvelope(w, http.StatusRequestEntityTooLarge, "upload exceeds size limit")
		return
	}
	httpx.WriteErrorEnvelope(w, http.StatusBadRequest, "malformed request")
}

// responseErrorFunc is the centralized ResponseErrorHandlerFunc for the
// strict handler: it unwraps *httpx.APIError and emits the {"error": string}
// envelope with the mapped status, falling back to 500 for any other error.
// When responseErrorDebug is set (flipped per-test by the api test suite),
// the 500 fallback message carries the underlying error for diagnosis.
var responseErrorDebug = false

func responseErrorFunc(w http.ResponseWriter, r *http.Request, err error) {
	var ae *httpx.APIError
	if errors.As(err, &ae) {
		if ae.Status >= 500 {
			log.Printf("api error: %s %s: %v", r.Method, r.URL.Path, err)
		}
		httpx.WriteErrorEnvelope(w, ae.Status, ae.Msg)
		return
	}
	log.Printf("internal error: %s %s: %v", r.Method, r.URL.Path, err)
	if responseErrorDebug {
		httpx.WriteErrorEnvelope(w, http.StatusInternalServerError, "internal error: "+err.Error())
		return
	}
	httpx.WriteErrorEnvelope(w, http.StatusInternalServerError, "internal error")
}

// Routes returns the fully-wired router for the API. All routes are
// user-scoped by path under /api/users/{userId} and resolved by a single
// user middleware. Route registration is derived from the OpenAPI document
// via the generated gen.HandlerWithOptions; handlers are bound and encoded
// by the generated strict server (gen.NewStrictHandlerWithOptions) with the
// centralized request/response error funcs.
//
// Note: the generated strict wrapper binds path params BEFORE the
// Middlewares chain runs, so an empty {userId} surfaces as the generated
// plain-text 400, not a JSON envelope. UserMiddleware handles the
// well-formed cases: malformed {userId} → {"error": string} 400 envelope,
// unknown user → 404 envelope. The {id} segments (asset/account/movement/
// batch/household IDs) are not rejected: an empty {id} routes to the handler,
// which resolves it as not-found (404) — matching the pre-change contract.
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
				// auth.Middleware is appended LAST: the generated chain folds
				// the slice forward (`handler = middleware(handler)`), so the
				// last element wraps outermost and executes FIRST. This makes
				// Basic Auth run before tenancy resolution, so the user
				// registry is never consulted on an auth rejection (design D3).
				auth.Middleware(s.basicAuthUsers),
			},
		},
	)
}

// ---------------------------------------------------------------------------
// Thin delegating methods (one per handler on the generated strict
// ServerInterface). Each forwards to the per-domain subpackage service.
// ---------------------------------------------------------------------------

// UploadDocument delegates to the add service.
func (s *Server) UploadDocument(ctx context.Context, request gen.UploadDocumentRequestObject) (gen.UploadDocumentResponseObject, error) {
	return s.addSvc.UploadDocument(ctx, request)
}

// AddItems delegates to the add service.
func (s *Server) AddItems(ctx context.Context, request gen.AddItemsRequestObject) (gen.AddItemsResponseObject, error) {
	return s.addSvc.AddItems(ctx, request)
}

// ListAssets delegates to the assets service.
func (s *Server) ListAssets(ctx context.Context, request gen.ListAssetsRequestObject) (gen.ListAssetsResponseObject, error) {
	return s.assetsSvc.ListAssets(ctx, request)
}

// GetAsset delegates to the assets service.
func (s *Server) GetAsset(ctx context.Context, request gen.GetAssetRequestObject) (gen.GetAssetResponseObject, error) {
	return s.assetsSvc.GetAsset(ctx, request)
}

// ListAssetDocuments delegates to the assets service.
func (s *Server) ListAssetDocuments(ctx context.Context, request gen.ListAssetDocumentsRequestObject) (gen.ListAssetDocumentsResponseObject, error) {
	return s.assetsSvc.ListAssetDocuments(ctx, request)
}

// DeleteAsset delegates to the assets service.
func (s *Server) DeleteAsset(ctx context.Context, request gen.DeleteAssetRequestObject) (gen.DeleteAssetResponseObject, error) {
	return s.assetsSvc.DeleteAsset(ctx, request)
}

// RestoreAsset delegates to the assets service.
func (s *Server) RestoreAsset(ctx context.Context, request gen.RestoreAssetRequestObject) (gen.RestoreAssetResponseObject, error) {
	return s.assetsSvc.RestoreAsset(ctx, request)
}

// MergeAsset delegates to the assets service.
func (s *Server) MergeAsset(ctx context.Context, request gen.MergeAssetRequestObject) (gen.MergeAssetResponseObject, error) {
	return s.assetsSvc.MergeAsset(ctx, request)
}

// PatchAsset delegates to the assets service.
func (s *Server) PatchAsset(ctx context.Context, request gen.PatchAssetRequestObject) (gen.PatchAssetResponseObject, error) {
	return s.assetsSvc.PatchAsset(ctx, request)
}

// QuickSearch delegates to the search service.
func (s *Server) QuickSearch(ctx context.Context, request gen.QuickSearchRequestObject) (gen.QuickSearchResponseObject, error) {
	return s.searchSvc.QuickSearch(ctx, request)
}

// Search delegates to the search service.
func (s *Server) Search(ctx context.Context, request gen.SearchRequestObject) (gen.SearchResponseObject, error) {
	return s.searchSvc.Search(ctx, request)
}

// CreateAccount delegates to the finance service.
func (s *Server) CreateAccount(ctx context.Context, request gen.CreateAccountRequestObject) (gen.CreateAccountResponseObject, error) {
	return s.financeSvc.CreateAccount(ctx, request)
}

// ListAccounts delegates to the finance service.
func (s *Server) ListAccounts(ctx context.Context, request gen.ListAccountsRequestObject) (gen.ListAccountsResponseObject, error) {
	return s.financeSvc.ListAccounts(ctx, request)
}

// GetAccount delegates to the finance service.
func (s *Server) GetAccount(ctx context.Context, request gen.GetAccountRequestObject) (gen.GetAccountResponseObject, error) {
	return s.financeSvc.GetAccount(ctx, request)
}

// CreateMovement delegates to the finance service.
func (s *Server) CreateMovement(ctx context.Context, request gen.CreateMovementRequestObject) (gen.CreateMovementResponseObject, error) {
	return s.financeSvc.CreateMovement(ctx, request)
}

// ListMovements delegates to the finance service.
func (s *Server) ListMovements(ctx context.Context, request gen.ListMovementsRequestObject) (gen.ListMovementsResponseObject, error) {
	return s.financeSvc.ListMovements(ctx, request)
}

// GetMovement delegates to the finance service.
func (s *Server) GetMovement(ctx context.Context, request gen.GetMovementRequestObject) (gen.GetMovementResponseObject, error) {
	return s.financeSvc.GetMovement(ctx, request)
}

// PatchMovement delegates to the finance service.
func (s *Server) PatchMovement(ctx context.Context, request gen.PatchMovementRequestObject) (gen.PatchMovementResponseObject, error) {
	return s.financeSvc.PatchMovement(ctx, request)
}

// DeleteMovement delegates to the finance service.
func (s *Server) DeleteMovement(ctx context.Context, request gen.DeleteMovementRequestObject) (gen.DeleteMovementResponseObject, error) {
	return s.financeSvc.DeleteMovement(ctx, request)
}

// LinkMovement delegates to the finance service.
func (s *Server) LinkMovement(ctx context.Context, request gen.LinkMovementRequestObject) (gen.LinkMovementResponseObject, error) {
	return s.financeSvc.LinkMovement(ctx, request)
}

// UnlinkMovement delegates to the finance service.
func (s *Server) UnlinkMovement(ctx context.Context, request gen.UnlinkMovementRequestObject) (gen.UnlinkMovementResponseObject, error) {
	return s.financeSvc.UnlinkMovement(ctx, request)
}

// CreateImportBatch delegates to the finance service.
func (s *Server) CreateImportBatch(ctx context.Context, request gen.CreateImportBatchRequestObject) (gen.CreateImportBatchResponseObject, error) {
	return s.financeSvc.CreateImportBatch(ctx, request)
}

// ListImportBatches delegates to the finance service.
func (s *Server) ListImportBatches(ctx context.Context, request gen.ListImportBatchesRequestObject) (gen.ListImportBatchesResponseObject, error) {
	return s.financeSvc.ListImportBatches(ctx, request)
}

// GetImportBatch delegates to the finance service.
func (s *Server) GetImportBatch(ctx context.Context, request gen.GetImportBatchRequestObject) (gen.GetImportBatchResponseObject, error) {
	return s.financeSvc.GetImportBatch(ctx, request)
}

// CommitImportBatch delegates to the finance service.
func (s *Server) CommitImportBatch(ctx context.Context, request gen.CommitImportBatchRequestObject) (gen.CommitImportBatchResponseObject, error) {
	return s.financeSvc.CommitImportBatch(ctx, request)
}

// DiscardImportBatch delegates to the finance service.
func (s *Server) DiscardImportBatch(ctx context.Context, request gen.DiscardImportBatchRequestObject) (gen.DiscardImportBatchResponseObject, error) {
	return s.financeSvc.DiscardImportBatch(ctx, request)
}

// ListReviews delegates to the reviews service.
func (s *Server) ListReviews(ctx context.Context, request gen.ListReviewsRequestObject) (gen.ListReviewsResponseObject, error) {
	return s.reviewsSvc.ListReviews(ctx, request)
}

// GetReview delegates to the reviews service.
func (s *Server) GetReview(ctx context.Context, request gen.GetReviewRequestObject) (gen.GetReviewResponseObject, error) {
	return s.reviewsSvc.GetReview(ctx, request)
}

// ApproveReview delegates to the reviews service.
func (s *Server) ApproveReview(ctx context.Context, request gen.ApproveReviewRequestObject) (gen.ApproveReviewResponseObject, error) {
	return s.reviewsSvc.ApproveReview(ctx, request)
}

// RejectReview delegates to the reviews service.
func (s *Server) RejectReview(ctx context.Context, request gen.RejectReviewRequestObject) (gen.RejectReviewResponseObject, error) {
	return s.reviewsSvc.RejectReview(ctx, request)
}

// CreateHousehold delegates to the households service.
func (s *Server) CreateHousehold(ctx context.Context, request gen.CreateHouseholdRequestObject) (gen.CreateHouseholdResponseObject, error) {
	return s.householdsSvc.CreateHousehold(ctx, request)
}

// AddHouseholdMember delegates to the households service.
func (s *Server) AddHouseholdMember(ctx context.Context, request gen.AddHouseholdMemberRequestObject) (gen.AddHouseholdMemberResponseObject, error) {
	return s.householdsSvc.AddHouseholdMember(ctx, request)
}

// ListHouseholds delegates to the households service.
func (s *Server) ListHouseholds(ctx context.Context, request gen.ListHouseholdsRequestObject) (gen.ListHouseholdsResponseObject, error) {
	return s.householdsSvc.ListHouseholds(ctx, request)
}

// GetHousehold delegates to the households service.
func (s *Server) GetHousehold(ctx context.Context, request gen.GetHouseholdRequestObject) (gen.GetHouseholdResponseObject, error) {
	return s.householdsSvc.GetHousehold(ctx, request)
}

// ListDocuments delegates to the documents service.
func (s *Server) ListDocuments(ctx context.Context, request gen.ListDocumentsRequestObject) (gen.ListDocumentsResponseObject, error) {
	return s.documentsSvc.ListDocuments(ctx, request)
}

// DeleteDocument delegates to the documents service.
func (s *Server) DeleteDocument(ctx context.Context, request gen.DeleteDocumentRequestObject) (gen.DeleteDocumentResponseObject, error) {
	return s.documentsSvc.DeleteDocument(ctx, request)
}

// KeepDocument delegates to the documents service.
func (s *Server) KeepDocument(ctx context.Context, request gen.KeepDocumentRequestObject) (gen.KeepDocumentResponseObject, error) {
	return s.documentsSvc.KeepDocument(ctx, request)
}

// ReprocessDocument delegates to the documents service.
func (s *Server) ReprocessDocument(ctx context.Context, request gen.ReprocessDocumentRequestObject) (gen.ReprocessDocumentResponseObject, error) {
	return s.documentsSvc.ReprocessDocument(ctx, request)
}
