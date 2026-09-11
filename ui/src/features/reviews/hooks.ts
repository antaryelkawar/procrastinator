/**
 * Reviews-feature data-layer hooks (asset-management-v2 modular-structure).
 *
 * Co-located with the reviews feature — all three hooks are consumed only by
 * `features/reviews/review-queue-page.tsx`. Shared query-key helpers,
 * `requireUser`, and the client/errors plumbing stay in `lib/api/`.
 */
import { useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import {
  assetsKey,
  invalidatePrefixes,
  requireUser,
  reviewKey,
  reviewsKey,
  reviewsPrefix,
} from '@/lib/api/hooks';
import { ApiError } from '@/lib/api/errors';
import { useActiveUser } from '@/context/active-user';
import type {
  ApproveReviewResponse,
  IngestReview,
} from '@/lib/api/schema';

/** `["reviews", uid, status]` — the active user's reviews by status. */
export function useReviews(status: 'pending' | 'approved' | 'rejected' = 'pending'): UseQueryResult<ReadonlyArray<IngestReview>, Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: reviewsKey(activeUser, status),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => client.listReviews(user, { status })),
  });
}

/** Variables for approve/reject. */
export interface ReviewIdVariables {
  readonly reviewId: string;
}

/**
 * D5: approve invalidates `["reviews", uid]`, `["review", uid, id]`, AND
 * `["assets", uid]` (approve creates/merges an Asset + Document, mirroring
 * `useCommitBatch`'s multi-prefix invalidation).
 */
export function useApproveReview(): UseMutationResult<ApproveReviewResponse, ApiError, ReviewIdVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<ApproveReviewResponse, ApiError, ReviewIdVariables>({
    mutationFn: ({ reviewId }: ReviewIdVariables) =>
      requireUser(activeUser, (user) => client.approveReview(user, reviewId)),
    onSuccess: (_data, variables) => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [
        reviewsPrefix(activeUser),
        reviewKey(activeUser, variables.reviewId),
        assetsKey(activeUser),
      ]);
    },
  });
}

/** D5: reject invalidates `["reviews", uid]` and `["review", uid, id]` only. */
export function useRejectReview(): UseMutationResult<IngestReview, ApiError, ReviewIdVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<IngestReview, ApiError, ReviewIdVariables>({
    mutationFn: ({ reviewId }: ReviewIdVariables) =>
      requireUser(activeUser, (user) => client.rejectReview(user, reviewId)),
    onSuccess: (_data, variables) => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [
        reviewsPrefix(activeUser),
        reviewKey(activeUser, variables.reviewId),
      ]);
    },
  });
}
