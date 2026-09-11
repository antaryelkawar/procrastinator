import { useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { MoreHorizontal } from 'lucide-react';
import { toast } from 'sonner';
import { useDocuments, useDeleteDocument, useReprocessDocument } from './hooks';
import { usePatchAsset } from '@/lib/api/hooks';
import { ApiError } from '@/lib/api/errors';
import type { DocumentRow, DocumentStatus } from '@/lib/api/client';
import type { PatchAssetRequest } from '@/lib/api/schema';
import { formatDate } from '@/lib/format/date';
import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { EmptyState } from '@/components/feedback/empty-state';
import { Loading } from '@/components/feedback/loading';
import { ErrorState } from '@/components/feedback/error-state';
import { DataTable, features } from '@/features/docs/data-table';
import { ConfirmDialog } from '@/features/docs/confirm-dialog';
import { AddButton } from '@/features/docs/add-button';
import { AddComposer } from '@/features/docs/composer/add-composer';

const columnHelper = createColumnHelper<typeof features, DocumentRow>();

/** The four derived document statuses the list can filter by. */
const STATUS_OPTIONS: readonly DocumentStatus[] = [
  'processed',
  'in_review',
  'failed',
  'asset_less',
] as const;

/**
 * Status → Badge presentation. Contrast-checked per variant in both themes:
 * `processed` uses the solid primary fill (default), `failed` a solid
 * destructive fill (bg-destructive text-white — the pale destructive variant
 * measures below the 4.5:1 AA floor at 12px), and the other two the
 * foreground-on-border outline.
 */
function StatusBadge({ status }: { readonly status: DocumentStatus }) {
  if (status === 'failed') {
    return (
      <Badge variant="destructive" className="bg-destructive text-white">
        failed
      </Badge>
    );
  }
  if (status === 'in_review') {
    return <Badge variant="secondary">in review</Badge>;
  }
  if (status === 'asset_less') {
    return <Badge variant="outline">no asset</Badge>;
  }
  return <Badge variant="default">processed</Badge>;
}

/**
 * The editable asset categories. NOTE: the generated `PatchAssetRequest.asset_category`
 * union is stale (it renders `document_only`/`other` as one mangled member,
 * `client.ts:139` pre-existing tsc error — task 10.1 scope), so the cast keeps
 * the known-good wire values (matching `paths.d.ts`) assignable without
 * widening the request type.
 */
const ASSET_CATEGORIES = [
  'appliance',
  'electronics',
  'computing',
  'furniture',
  'vehicle',
  'tool',
  'clothing',
  'document_only',
  'other',
] as const;

interface DocumentActionCellProps {
  readonly document: DocumentRow;
}

/**
 * Per-row "…" action menu (pattern: movement-actions.tsx), status-aware:
 *
 *  - all rows: "Delete" (ConfirmDialog, destructive toast on success);
 *  - processed: "Open asset" (when linked) + "Edit asset" (PATCH dialog via
 *    the shared usePatchAsset) — no "Reprocess" (the pipeline already
 *    succeeded);
 *  - failed / asset_less: "Reprocess" (optional-comment dialog) +
 *    "Open asset" (when linked);
 *  - in_review: "Reprocess" disabled (work already in flight; the backend
 *    would 409) + "Open asset" (when linked).
 *
 * NOTE (tracked blocker): an "asset reprocess" affordance is intentionally
 * ABSENT — no such backend endpoint exists (only `POST /documents/{id}/
 * reprocess`), so no button (enabled or disabled) is offered.
 */
function DocumentActionCell({ document }: DocumentActionCellProps) {
  const navigate = useNavigate();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [reprocessOpen, setReprocessOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [comment, setComment] = useState('');
  const [name, setName] = useState('');
  const [category, setCategory] = useState('');

  const deleteMutation = useDeleteDocument();
  const reprocessMutation = useReprocessDocument();
  const patchAssetMutation = usePatchAsset();

  const assetId = document.asset_id ?? undefined;
  const status = document.status;
  const inReview = status === 'in_review';
  const processed = status === 'processed';

  const handleDeleteConfirm = async () => {
    try {
      await deleteMutation.mutateAsync({ documentId: document.id });
      setDeleteOpen(false);
      toast('Document deleted', {
        description: `${document.source_filename} was removed.`,
      });
    } catch (err) {
      // The dialog stays open (busy is released) so the user can retry.
      toast.error('Delete failed', { description: (err as Error).message });
    }
  };

  const handleReprocessSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = comment.trim();
    try {
      await reprocessMutation.mutateAsync({
        documentId: document.id,
        comment: trimmed === '' ? undefined : trimmed,
      });
      setReprocessOpen(false);
      setComment('');
      toast('Reprocessing document');
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        // Already in flight — surface a friendly message, keep the dialog open.
        toast('Already reprocessing', {
          description: 'This document is already being reprocessed.',
        });
      } else {
        toast.error('Reprocess failed', { description: (err as Error).message });
      }
    }
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!assetId) {
      return;
    }
    const body: PatchAssetRequest = {};
    if (name.trim() !== '') {
      body.name = name.trim();
    }
    if (category !== '') {
      body.asset_category = category as PatchAssetRequest['asset_category'];
    }
    try {
      await patchAssetMutation.mutateAsync({ assetId, body });
      setEditOpen(false);
      toast('Asset updated');
    } catch (err) {
      toast.error('Asset update failed', { description: (err as Error).message });
    }
  };

  return (
    <div className="flex items-center justify-end">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${document.source_filename}`}>
            <MoreHorizontal className="h-4 w-4" />
            <span className="sr-only">Actions</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-44">
          {(status === 'failed' || status === 'asset_less') && (
            <DropdownMenuItem onSelect={() => setReprocessOpen(true)}>
              Reprocess
            </DropdownMenuItem>
          )}
          {inReview && (
            // The backend would 409 — work is already in flight.
            <DropdownMenuItem disabled onSelect={() => undefined}>
              Reprocess (in review…)
            </DropdownMenuItem>
          )}
          {assetId !== undefined && (
            <DropdownMenuItem onSelect={() => navigate(`/assets/${assetId}`)}>
              Open asset
            </DropdownMenuItem>
          )}
          {processed && assetId !== undefined && (
            <DropdownMenuItem onSelect={() => setEditOpen(true)}>
              Edit asset
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            variant="destructive"
            onSelect={() => setDeleteOpen(true)}
          >
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete document"
        description={`“${document.source_filename}” will be removed. The linked asset is kept.`}
        confirmLabel="Delete"
        destructive
        busy={deleteMutation.isPending}
        onConfirm={handleDeleteConfirm}
      />

      <Dialog
        open={reprocessOpen}
        onOpenChange={(open) => {
          setReprocessOpen(open);
          if (open) {
            setComment('');
          }
        }}
      >
        <DialogContent>
          <form onSubmit={handleReprocessSubmit}>
            <DialogHeader>
              <DialogTitle>Reprocess document</DialogTitle>
            </DialogHeader>
            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor={`reprocess-comment-${document.id}`}>Add a note (optional)</Label>
                <Input
                  id={`reprocess-comment-${document.id}`}
                  placeholder="Add a note for the extractor…"
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  autoFocus
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setReprocessOpen(false)}
                disabled={reprocessMutation.isPending}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={reprocessMutation.isPending}>
                {reprocessMutation.isPending ? 'Submitting…' : 'Reprocess'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={editOpen}
        onOpenChange={(open) => {
          setEditOpen(open);
          if (open) {
            setName('');
            setCategory('');
          }
        }}
      >
        <DialogContent>
          <form onSubmit={handleEditSubmit}>
            <DialogHeader>
              <DialogTitle>Edit asset</DialogTitle>
            </DialogHeader>
            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor={`edit-name-${document.id}`}>Asset name</Label>
                <Input
                  id={`edit-name-${document.id}`}
                  placeholder="Asset name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoFocus
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor={`edit-category-${document.id}`}>Asset category</Label>
                <Select value={category} onValueChange={setCategory}>
                  <SelectTrigger id={`edit-category-${document.id}`} className="w-full" aria-label="Asset category">
                    <SelectValue placeholder="No change" />
                  </SelectTrigger>
                  <SelectContent>
                    {ASSET_CATEGORIES.map((value) => (
                      <SelectItem key={value} value={value as string}>
                        {value}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setEditOpen(false)}
                disabled={patchAssetMutation.isPending}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={patchAssetMutation.isPending}>
                {patchAssetMutation.isPending ? 'Saving…' : 'Save'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/**
 * Documents list view (task 8.1). Lists the active user's documents with a
 * status filter + filename search, a status-aware per-row action menu, and a
 * page-level empty state (pattern: asset-list-page.tsx).
 */
export function DocumentsPage() {
  const [status, setStatus] = useState<DocumentStatus | 'all'>('all');
  const [query, setQuery] = useState('');
  const [composerOpen, setComposerOpen] = useState(false);

  const trimmedQuery = query.trim();
  const filters = useMemo(
    () => ({
      status: status === 'all' ? undefined : status,
      q: trimmedQuery === '' ? undefined : trimmedQuery,
    }),
    [status, trimmedQuery],
  );
  const { data: documents, isLoading, error } = useDocuments(filters);

  const columns = useMemo(
    () =>
      [
        columnHelper.accessor('source_filename', {
          header: 'Filename',
          cell: (context) => context.getValue() ?? '-',
        }),
        columnHelper.accessor('source_uploaded_at', {
          header: 'Uploaded',
          cell: (context) => {
            const value = context.getValue();
            return value ? formatDate(value) : '-';
          },
        }),
        columnHelper.accessor('status', {
          header: 'Status',
          cell: (context) => <StatusBadge status={context.getValue()} />,
        }),
        columnHelper.accessor('asset_id', {
          header: 'Asset',
          enableSorting: false,
          cell: (context) => {
            const assetId = context.getValue();
            return assetId ? (
              <Link to={`/assets/${assetId}`} className="text-primary underline-offset-4 hover:underline">
                View asset
              </Link>
            ) : (
              <span className="text-muted-foreground">—</span>
            );
          },
        }),
        {
          id: 'actions',
          header: 'Actions',
          enableSorting: false,
          cell: (context) => <DocumentActionCell document={context.row.original} />,
        },
      ] as ColumnDef<TableFeatures, DocumentRow, unknown>[],
    [],
  );

  return (
    <div className={cn('container mx-auto min-w-0 py-8 space-y-6')}>
      <AddButton label="Add document" onAdd={() => setComposerOpen(true)} />

      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
        <h1 className="text-2xl font-semibold">Documents</h1>
      </div>

      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="w-full sm:w-auto">
          <Label htmlFor="documents-status-select">Status</Label>
          <Select value={status} onValueChange={(value) => setStatus(value as DocumentStatus | 'all')}>
            <SelectTrigger id="documents-status-select" className="w-full sm:w-[180px]" aria-label="Status">
              <SelectValue placeholder="All statuses" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All statuses</SelectItem>
              {STATUS_OPTIONS.map((option) => (
                <SelectItem key={option} value={option}>
                  {option}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="w-full sm:w-auto">
          <Label htmlFor="documents-filename-search">Filename</Label>
          <Input
            id="documents-filename-search"
            type="search"
            placeholder="Search by filename…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full sm:w-[220px]"
          />
        </div>
      </div>

      {isLoading ? (
        <Loading />
      ) : error ? (
        <ErrorState title="Failed to load documents" message={error.message} />
      ) : !documents || documents.length === 0 ? (
        <EmptyState
          title="No documents found"
          description="Upload an invoice or receipt, or clear the filters above."
        >
          <Button asChild>
            <Link to="/">Add something</Link>
          </Button>
        </EmptyState>
      ) : (
        <DataTable
          ariaLabel="Documents"
          // The shared DataTable is typed against `Record<string, unknown>` rows;
          // `DocumentRow` is a plain object type, so the widening is safe.
          data={documents as unknown as Record<string, unknown>[]}
          getRowId={(doc) => String(doc.id)}
          columns={columns as unknown as ColumnDef<TableFeatures, Record<string, unknown>, unknown>[]}
        />
      )}

      <AddComposer open={composerOpen} onOpenChange={setComposerOpen} title="Add a document" />
    </div>
  );
}
