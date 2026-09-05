package api

import (
	"context"
	"errors"
	"net/http"

	"procrastinator-backend/api/gen"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/core/review"
)

// ListReviews returns the user's reviews, filtered by state.
func (s *Server) ListReviews(ctx context.Context, request gen.ListReviewsRequestObject) (gen.ListReviewsResponseObject, error) {
	var status entity.IngestReviewState
	if request.Params.Status != nil {
		status = entity.IngestReviewState(*request.Params.Status)
		if status != entity.ReviewStatePending && status != entity.ReviewStateApproved && status != entity.ReviewStateRejected {
			return nil, newAPIError(http.StatusBadRequest, "invalid status")
		}
	}
	reviews, err := s.review.List(ctx, status)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "internal error")
	}
	out := make([]gen.IngestReview, 0, len(reviews))
	for _, r := range reviews {
		dto, err := s.toReview(ctx, request.UserId, r)
		if err != nil {
			return nil, newAPIError(http.StatusInternalServerError, "internal error")
		}
		out = append(out, dto)
	}
	return gen.ListReviews200JSONResponse(out), nil
}

// GetReview returns a single review by ID.
func (s *Server) GetReview(ctx context.Context, request gen.GetReviewRequestObject) (gen.GetReviewResponseObject, error) {
	rev, err := s.review.Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, newAPIError(http.StatusNotFound, "review not found")
		}
		return nil, newAPIError(http.StatusInternalServerError, "internal error")
	}
	dto, err := s.toReview(ctx, request.UserId, rev)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.GetReview200JSONResponse(dto), nil
}

// ApproveReview approves a pending review: commits the candidate asset + document.
func (s *Server) ApproveReview(ctx context.Context, request gen.ApproveReviewRequestObject) (gen.ApproveReviewResponseObject, error) {
	asset, updated, err := s.review.Approve(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, newAPIError(http.StatusNotFound, "review not found")
		}
		if errors.Is(err, review.ErrConflict) {
			return nil, newAPIError(http.StatusConflict, "review is not in a pending state")
		}
		return nil, newAPIError(http.StatusInternalServerError, "internal error")
	}
	dto, err := s.toReview(ctx, request.UserId, updated)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.ApproveReview200JSONResponse(gen.ApproveReviewResponse{
		Asset:  toAsset(asset),
		Review: dto,
	}), nil
}

// RejectReview rejects a pending review: the upload is discarded.
func (s *Server) RejectReview(ctx context.Context, request gen.RejectReviewRequestObject) (gen.RejectReviewResponseObject, error) {
	updated, err := s.review.Reject(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, newAPIError(http.StatusNotFound, "review not found")
		}
		if errors.Is(err, review.ErrConflict) {
			return nil, newAPIError(http.StatusConflict, "review is not in a pending state")
		}
		return nil, newAPIError(http.StatusInternalServerError, "internal error")
	}
	dto, err := s.toReview(ctx, request.UserId, updated)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "internal error")
	}
	return gen.RejectReview200JSONResponse(dto), nil
}
