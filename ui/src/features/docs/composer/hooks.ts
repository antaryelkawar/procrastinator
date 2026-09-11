/**
 * Shared ingest mutation hooks (task 14.1 / design D6/D11).
 *
 * These hooks are the single, shared ingest plumbing consumed by the
 * `features/docs/composer` AddComposer AND by cross-feature callers such as
 * `features/landing` (the landing [+] button) — the shared feature
 * `features/docs` is the unrestricted module, so landing/documents/etc. may
 * import it. The four hook implementations live here (moved from the deleted
 * landing ingest cards) and import only absolute `@/lib/api/*` +
 * `@/context/active-user`. Shared query-key helpers (`assetsKey`, `batchesKey`,
 * `reviewsPrefix`), `requireUser`, and the client/upload plumbing stay in
 * `lib/api/` (the single source of truth).
 *
 *   | hook                 | invalidated prefixes                          |
 *   | -------------------- | --------------------------------------------- |
 *   | useIngestFile        | ["assets", uid] (committed) / ["reviews", uid] (held) |
 *   | useIngestText        | ["assets", uid], ["batches", uid]             |
 *   | useReprocessDocument | ["assets", uid]                               |
 *   | useKeepDocument      | ["assets", uid]                               |
 */
import { useMutation, useQueryClient, type UseMutationResult } from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import { uploadDocument } from '@/lib/api/upload';
import type { UploadDocumentResult } from '@/lib/api/upload';
import {
  assetsKey,
  batchesKey,
  invalidatePrefixes,
  requireUser,
  reviewsPrefix,
} from '@/lib/api/hooks';
import { ApiError } from '@/lib/api/errors';
import { useActiveUser } from '@/context/active-user';
import type { AddItemOutcome, Document } from '@/lib/api/schema';

/** Variables for a file ingest (camera/image or document card). */
export interface IngestFileVariables {
  readonly file: File;
  /** Optional free-text user directive sent as the multipart `note` field. */
  readonly note?: string;
}

/**
 * File ingest (camera/image or document card). Wraps the XHR upload helper,
 * which resolves the discriminated union: 201 → committed asset, 202 → held
 * for review, 409 → structured `DuplicateReport` (the caller renders the
 * reprocess/keep modal from `report.prompt`).
 *
 * D5 invalidation: committed assets invalidate `["assets", uid]`; held-for-
 * review invalidates the reviews prefix (the review queue gains a row).
 */
export function useIngestFile(): UseMutationResult<
  UploadDocumentResult,
  ApiError,
  IngestFileVariables
> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<UploadDocumentResult, ApiError, IngestFileVariables>({
    mutationFn: ({ file, note }: IngestFileVariables) =>
      requireUser(activeUser, (user) => uploadDocument(user, file, () => undefined, note)),
    onSuccess: (result) => {
      if (activeUser === null) {
        return;
      }
      if (result.kind === 'committed') {
        invalidatePrefixes(queryClient, [assetsKey(activeUser)]);
      } else if (result.kind === 'held') {
        invalidatePrefixes(queryClient, [reviewsPrefix(activeUser)]);
      }
      // 'duplicate' → no invalidation (nothing was committed yet).
    },
  });
}

/** Variables for a text-only ingest (text card). */
export interface IngestTextVariables {
  readonly text: string;
}

/**
 * Text-only ingest (text card). Wraps `client.addItems({ text })` — the backend
 * wraps the pasted text as a `pasted.txt` document. No account or doc-type
 * pre-selection.
 *
 * D5 invalidation mirrors `useAdd`: `["assets", uid]` (committed assets appear)
 * AND `["batches", uid]` (a `statement_preview` outcome creates a batch).
 */
export function useIngestText(): UseMutationResult<
  AddItemOutcome[],
  ApiError,
  IngestTextVariables
> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<AddItemOutcome[], ApiError, IngestTextVariables>({
    mutationFn: ({ text }: IngestTextVariables) =>
      requireUser(activeUser, (user) => client.addItems(user, { text })),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [assetsKey(activeUser), batchesKey(activeUser)]);
    },
  });
}

/** Variables identifying the document to reprocess (duplicate choice). */
export interface ReprocessVariables {
  readonly userId: string;
  readonly documentId: string;
  /** Optional user hint passed to the extractor. */
  readonly comment?: string;
}

/**
 * Reprocess a duplicate document (POST `{reprocess_uri}`). D5: invalidate
 * `["assets", uid]` (a reprocessed asset may be committed or change shape).
 */
export function useReprocessDocument(): UseMutationResult<
  Document,
  ApiError,
  ReprocessVariables
> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Document, ApiError, ReprocessVariables>({
    mutationFn: ({ userId, documentId, comment }: ReprocessVariables) =>
      client.reprocessDocument(userId, documentId, comment),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [assetsKey(activeUser)]);
    },
  });
}

/** Variables identifying the document to keep (duplicate choice). */
export interface KeepVariables {
  readonly userId: string;
  readonly documentId: string;
}

/**
 * Keep the existing document on a duplicate (POST `{keep_uri}`). D5:
 * invalidate `["assets", uid]` (the kept document may surface a new asset).
 */
export function useKeepDocument(): UseMutationResult<Document, ApiError, KeepVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Document, ApiError, KeepVariables>({
    mutationFn: ({ userId, documentId }: KeepVariables) =>
      client.keepDocument(userId, documentId),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [assetsKey(activeUser)]);
    },
  });
}
