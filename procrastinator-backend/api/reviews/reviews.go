package reviews

import (
	"context"
	"errors"
	"net/http"

	apidto "procrastinator-backend/api/dto"
	"procrastinator-backend/api/gen"
	"procrastinator-backend/api/httpx"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/review"
)

// Service serves the ingest review endpoints (list, get, approve, reject).
type Service struct {
	review  *review.Service
	factory *repo.Factory
}

// New constructs a Service.
func New(review *review.Service, factory *repo.Factory) *Service {
	return &Service{review: review, factory: factory}
}

// ListReviews returns the user's reviews, filtered by state.
func (s *Service) ListReviews(ctx context.Context, request gen.ListReviewsRequestObject) (gen.ListReviewsResponseObject, error) {
	var status entity.IngestReviewState
	if request.Params.Status != nil {
		status = entity.IngestReviewState(*request.Params.Status)
		if status != entity.ReviewStatePending && status != entity.ReviewStateApproved && status != entity.ReviewStateRejected {
			return nil, httpx.NewAPIError(http.StatusBadRequest, "invalid status")
		}
	}
	reviews, err := s.review.List(ctx, status)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	out := make([]gen.IngestReview, 0, len(reviews))
	for _, r := range reviews {
		reviewDTO, err := apidto.ToReview(s.factory, ctx, request.UserId, r)
		if err != nil {
			return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
		}
		out = append(out, reviewDTO)
	}
	return gen.ListReviews200JSONResponse(out), nil
}

// GetReview returns a single review by ID.
func (s *Service) GetReview(ctx context.Context, request gen.GetReviewRequestObject) (gen.GetReviewResponseObject, error) {
	rev, err := s.review.Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, httpx.NewAPIError(http.StatusNotFound, "review not found")
		}
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	reviewDTO, err := apidto.ToReview(s.factory, ctx, request.UserId, rev)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.GetReview200JSONResponse(reviewDTO), nil
}

// ApproveReview approves a pending review: commits the candidate asset + document.
func (s *Service) ApproveReview(ctx context.Context, request gen.ApproveReviewRequestObject) (gen.ApproveReviewResponseObject, error) {
	asset, updated, err := s.review.Approve(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, httpx.NewAPIError(http.StatusNotFound, "review not found")
		}
		if errors.Is(err, review.ErrConflict) {
			return nil, httpx.NewAPIError(http.StatusConflict, "review is not in a pending state")
		}
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	reviewDTO, err := apidto.ToReview(s.factory, ctx, request.UserId, updated)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.ApproveReview200JSONResponse(gen.ApproveReviewResponse{
		Asset:  apidto.ToAsset(asset),
		Review: reviewDTO,
	}), nil
}

// RejectReview rejects a pending review: the upload is discarded.
func (s *Service) RejectReview(ctx context.Context, request gen.RejectReviewRequestObject) (gen.RejectReviewResponseObject, error) {
	updated, err := s.review.Reject(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, httpx.NewAPIError(http.StatusNotFound, "review not found")
		}
		if errors.Is(err, review.ErrConflict) {
			return nil, httpx.NewAPIError(http.StatusConflict, "review is not in a pending state")
		}
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	reviewDTO, err := apidto.ToReview(s.factory, ctx, request.UserId, updated)
	if err != nil {
		return nil, httpx.NewAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.RejectReview200JSONResponse(reviewDTO), nil
}
