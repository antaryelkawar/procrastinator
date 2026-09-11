/**
 * Documents-feature data-layer hooks (task 8.1).
 *
 * Co-located with the documents feature — every hook below is consumed only by
 * files under `features/documents/`. The cross-feature import boundary
 * (design D6) forbids importing the landing cards' `useReprocessDocument`, so
 * this feature owns its own reprocess hook here. Shared query-key helpers
 * (`documentsKey`/`documentsPrefix`), `requireUser`, and the client plumbing
 * stay in `lib/api/` (single source of truth).
 *
 *   | hook                 | invalidated prefixes        |
 *   | -------------------- | --------------------------- |
 *   | useDeleteDocument    | ["documents", uid]          |
 *   | useReprocessDocument | ["documents", uid]          |
 *
 * `usePatchAsset` (editing a processed document's linked asset) lives in the
 * shared `lib/api/hooks` home and is imported by the page directly.
 */
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import {
  documentsKey,
  documentsPrefix,
  invalidatePrefixes,
  requireUser,
  type DocumentFilterInput,
} from '@/lib/api/hooks';
import { ApiError } from '@/lib/api/errors';
import { useActiveUser } from '@/context/active-user';
import type { Document } from '@/lib/api/schema';
import type { DocumentRow } from '@/lib/api/client';

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * `["documents", uid, {status, q}]` — the active user's documents with the
 * supplied filters (status / filename substring).
 */
export function useDocuments(
  filters?: DocumentFilterInput,
): UseQueryResult<DocumentRow[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: documentsKey(activeUser, filters),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => client.listDocuments(user, filters)),
  });
}

// ---------------------------------------------------------------------------
// Mutations (D5 invalidation map)
// ---------------------------------------------------------------------------

/** Variables identifying one document (delete). */
export interface DeleteDocumentVariables {
  readonly documentId: string;
}

/**
 * D5: document soft-delete (+ detach) → `["documents", uid]` — the list
 * refetches and the soft-deleted row disappears; the linked asset is
 * preserved server-side.
 */
export function useDeleteDocument(): UseMutationResult<void, ApiError, DeleteDocumentVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, DeleteDocumentVariables>({
    mutationFn: ({ documentId }: DeleteDocumentVariables) =>
      requireUser(activeUser, (user) => client.deleteDocument(user, documentId)),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [documentsPrefix(activeUser)]);
    },
  });
}

/** Variables identifying the document to reprocess (duplicate choice). */
export interface ReprocessDocumentVariables {
  readonly documentId: string;
  /** Optional user hint passed to the extractor (omitted when blank). */
  readonly comment?: string;
}

/**
 * D5: document reprocess → `["documents", uid]` (refetch surfaces the fresh
 * derived status). A `409` (already in flight) arrives as `ApiError` with
 * `status 409`; the caller decides how to surface it — the mutation itself
 * rejects normally and no invalidation happens on failure.
 */
export function useReprocessDocument(): UseMutationResult<
  Document,
  ApiError,
  ReprocessDocumentVariables
> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Document, ApiError, ReprocessDocumentVariables>({
    mutationFn: ({ documentId, comment }: ReprocessDocumentVariables) =>
      requireUser(activeUser, (user) => client.reprocessDocument(user, documentId, comment)),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [documentsPrefix(activeUser)]);
    },
  });
}
