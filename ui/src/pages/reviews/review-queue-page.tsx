import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { Check, X } from 'lucide-react';
import { toast } from 'sonner';
import { useReviews, useApproveReview, useRejectReview } from '@/lib/api/hooks';
import { formatConfidence } from '@/lib/format/confidence';
import { formatTimestamp } from '@/lib/format/date';
import { Loading } from '@/components/feedback/loading';
import { ErrorState } from '@/components/feedback/error-state';
import { EmptyState } from '@/components/feedback/empty-state';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { DataTable, features } from '@/components/data-table';
import type { IngestReview } from '@/lib/api/schema';

type ReviewStatus = 'pending' | 'approved' | 'rejected';

const columnHelper = createColumnHelper<typeof features, IngestReview>();

/**
 * Columns for the review queue, rendered through the shared DataTable
 * (consolidated-primitives decision: no per-page hand-rolled tables).
 */
function buildColumns(
  onApprove: (review: IngestReview) => void,
  onReject: (review: IngestReview) => void,
): ColumnDef<TableFeatures, IngestReview, unknown>[] {
  return [
    columnHelper.accessor('doc_type', {
      header: 'Doc type',
      cell: (context) => <span className="font-medium capitalize">{context.getValue()}</span>,
    }),
    columnHelper.accessor('confidence', {
      header: 'Confidence',
      cell: (context) => formatConfidence(context.getValue()) ?? '—',
    }),
    columnHelper.accessor('source_filename', {
      header: 'Source',
      cell: (context) => (
        <span className="max-w-[200px] truncate" title={context.getValue()}>
          {context.getValue()}
        </span>
      ),
    }),
    columnHelper.accessor('best_matched_asset_title', {
      header: 'Best match',
      cell: (context) =>
        context.getValue() ? (
          <span title={context.getValue() as string}>{context.getValue()}</span>
        ) : (
          <span className="text-muted-foreground italic">no match</span>
        ),
    }),
    columnHelper.accessor('created_at', {
      header: 'Created',
      cell: (context) => formatTimestamp(context.getValue()),
    }),
    columnHelper.accessor('state', {
      header: 'Status',
      cell: (context) => (
        <Badge
          variant={
            context.getValue() === 'approved'
              ? 'default'
              : context.getValue() === 'rejected'
                ? 'destructive'
                : 'secondary'
          }
        >
          {context.getValue()}
        </Badge>
      ),
    }),
    {
      id: 'actions',
      header: <span className="float-right">Actions</span>,
      cell: (context) => {
        const review = context.row.original;
        if (review.state !== 'pending') return null;
        return (
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => onApprove(review)}
              aria-label={`Approve ${review.source_filename}`}
            >
              <Check className="mr-1 size-4" />
              Approve
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onReject(review)}
              aria-label={`Reject ${review.source_filename}`}
            >
              <X className="mr-1 size-4" />
              Reject
            </Button>
          </div>
        );
      },
    },
  ] as ColumnDef<TableFeatures, IngestReview, unknown>[];
}

/**
 * P12.7: Ingest review queue at `/ingest/reviews` (design D8).
 * Lists reviews filtered by status (default pending), with approve/reject
 * actions behind confirmation dialogs. Surfaces 409/404 as errors.
 */
export function ReviewQueuePage() {
  const [status, setStatus] = useState<ReviewStatus>('pending');
  const { data: reviews, isLoading, isError, refetch } = useReviews(status);
  const navigate = useNavigate();

  const [confirmingReview, setConfirmingReview] = useState<IngestReview | null>(null);
  const [action, setAction] = useState<'approve' | 'reject'>('approve');

  const approveMutation = useApproveReview();
  const rejectMutation = useRejectReview();

  const openConfirm = (review: IngestReview, a: 'approve' | 'reject') => {
    setConfirmingReview(review);
    setAction(a);
  };

  const closeConfirm = () => {
    setConfirmingReview(null);
  };

  const handleConfirm = async () => {
    if (!confirmingReview) return;
    const reviewId = confirmingReview.id;
    try {
      if (action === 'approve') {
        const result = await approveMutation.mutateAsync({ reviewId });
        toast.success('Review approved', {
          description: 'Asset committed successfully',
          action: {
            label: 'View asset',
            onClick: () => navigate(`/assets/${result.asset.id}`),
          },
        });
      } else {
        await rejectMutation.mutateAsync({ reviewId });
        toast.success('Review rejected');
      }
      closeConfirm();
      refetch();
    } catch (error) {
      const errorStatus = (error as { status?: number })?.status;
      if (errorStatus === 409) {
        toast.error('Review already processed', {
          description: 'This review is no longer pending',
        });
      } else if (errorStatus === 404) {
        toast.error('Review not found', {
          description: 'This review may have been removed',
        });
      } else {
        toast.error('Action failed', {
          description: 'An unexpected error occurred',
        });
      }
      closeConfirm();
      refetch();
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Ingest review queue</h1>
        <Select value={status} onValueChange={(v) => setStatus(v as ReviewStatus)}>
          <SelectTrigger className="w-[180px]" aria-label="Filter by status">
            <SelectValue placeholder="Filter by status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="pending">Pending</SelectItem>
            <SelectItem value="approved">Approved</SelectItem>
            <SelectItem value="rejected">Rejected</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <Loading label="Loading reviews" rows={6} />
      ) : isError ? (
        <ErrorState message="Failed to load reviews" onRetry={() => refetch()} />
      ) : !reviews || reviews.length === 0 ? (
        <EmptyState
          title={`No ${status} reviews`}
          description={
            status === 'pending'
              ? 'All uploads have been processed. New low-confidence uploads will appear here for review.'
              : `No reviews with status "${status}"`
          }
        />
      ) : (
        <div className="rounded-md border">
          <DataTable
            ariaLabel="Ingest reviews"
            data={reviews as IngestReview[]}
            getRowId={(review) => review.id}
            columns={buildColumns((review) => openConfirm(review, 'approve'), (review) => openConfirm(review, 'reject'))}
          />
        </div>
      )}

      {confirmingReview ? (
        <ConfirmDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) closeConfirm();
          }}
          title={action === 'approve' ? 'Approve review?' : 'Reject review?'}
          description={
            action === 'approve'
              ? 'This will commit the candidate as an asset. The review will be marked as approved.'
              : 'This will discard the candidate. The review will be marked as rejected and cannot be undone.'
          }
          confirmLabel={action === 'approve' ? 'Approve' : 'Reject'}
          destructive={action === 'reject'}
          busy={approveMutation.isPending || rejectMutation.isPending}
          onConfirm={handleConfirm}
        />
      ) : null}
    </div>
  );
}
