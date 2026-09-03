/**
 * TanStack Query data layer (design D4/D5) — the ONLY module screens use for
 * server data.
 *
 * One `useQuery` per resource and one `useMutation` per write operation.
 * Every query key carries the active user id (D4); while no user is active the
 * queries are disabled and the mutations reject without sending a request.
 * Every mutation invalidates EXACTLY the key prefixes the D5 map names:
 *
 *   | hook                | invalidated prefixes                                              |
 *   | ------------------- | ----------------------------------------------------------------- |
 *   | useUploadDocument   | ["assets", uid]                                                   |
 *   | useCreateAccount    | ["accounts", uid]                                                 |
 *   | useCreateMovement   | ["movements", uid], ["accounts", uid]                             |
 *   | usePatchDescription | ["movements", uid], ["accounts", uid]                             |
 *   | useDeleteMovement   | ["movements", uid], ["accounts", uid]                             |
 *   | useLinkMovement     | ["movements", uid], ["accounts", uid]                             |
 *   | useUnlinkMovement   | ["movements", uid], ["accounts", uid]                             |
 *   | useUploadStatement  | ["batches", uid], ["batch", uid, id]                              |
 *   | useCommitBatch      | ["batches", uid], ["batch", uid, id], ["movements", uid], ["accounts", uid] |
 *   | useDiscardBatch     | ["batches", uid], ["batch", uid, id]                              |
 *
 * The `["movements", uid]` prefix deliberately carries NO filter element, so
 * it matches every filter variant of the movements key.
 *
 * The backend API is being reworked in parallel: every request goes through
 * the centralized client (`client.ts`, D1) or the XHR upload helper
 * (`upload.ts`, D6), so re-pointing paths is a change to those files only.
 */
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { QueryClient, UseMutationResult, UseQueryResult } from '@tanstack/react-query';
import { apiJson, apiVoid } from './client';
import type { RequestScope } from './client';
import { uploadDocument, uploadStatement } from './upload';
import { ApiError } from './errors';
import { useActiveUser } from '../../context/active-user';
import type {
  Account,
  Asset,
  CommitSummary,
  CreateAccountRequest,
  CreateMovementRequest,
  Document,
  ImportBatch,
  Movement,
} from './types';

// ---------------------------------------------------------------------------
// Query keys (D5) — every key carries the active user id (D4)
// ---------------------------------------------------------------------------

/** `["assets", uid]` — the active user's assets. */
export function assetsKey(userId: string | null): readonly ['assets', string | null] {
  return ['assets', userId];
}

/** `["asset", uid, id]` — one asset. */
export function assetKey(userId: string | null, assetId: string): readonly ['asset', string | null, string] {
  return ['asset', userId, assetId];
}

/** `["asset-docs", uid, id]` — the documents of one asset. */
export function assetDocsKey(
  userId: string | null,
  assetId: string,
): readonly ['asset-docs', string | null, string] {
  return ['asset-docs', userId, assetId];
}

/** `["accounts", uid]` — the active user's finance accounts. */
export function accountsKey(userId: string | null): readonly ['accounts', string | null] {
  return ['accounts', userId];
}

/**
 * Movement list filters in UI shape. The wire names (`account_id`, `from`,
 * `to`) are mapped in {@link movementsResource}; the key keeps the UI shape
 * (D5: `{accountId, from, to}`).
 */
export interface MovementFilterInput {
  readonly accountId?: string;
  readonly from?: string;
  readonly to?: string;
}

/** Always all three keys present (undefined when unset) — stable key shape. */
function normalizeFilters(filters: MovementFilterInput | undefined): MovementFilterInput {
  return { accountId: filters?.accountId, from: filters?.from, to: filters?.to };
}

/** `["movements", uid, {accountId, from, to}]` — movements with the filters. */
export function movementsKey(
  userId: string | null,
  filters?: MovementFilterInput,
): readonly ['movements', string | null, MovementFilterInput] {
  return ['movements', userId, normalizeFilters(filters)];
}

/**
 * Prefix of EVERY movements key (any filter set) — the D5 invalidation unit.
 * Without the filter element it matches all filter variants.
 */
export function movementsPrefix(userId: string | null): readonly ['movements', string | null] {
  return ['movements', userId];
}

/** `["batches", uid]` — the import batch history. */
export function batchesKey(userId: string | null): readonly ['batches', string | null] {
  return ['batches', userId];
}

/** `["batch", uid, id]` — one import batch. */
export function batchKey(userId: string | null, batchId: string): readonly ['batch', string | null, string] {
  return ['batch', userId, batchId];
}

// ---------------------------------------------------------------------------
// Request plumbing (the wire shape of the request)
// ---------------------------------------------------------------------------

function userScope(userId: string): RequestScope {
  return { kind: 'user', userId };
}

function financeScope(userId: string): RequestScope {
  return { kind: 'finance', userId };
}

/**
 * Resource path for the movement list with ONLY the supplied filters, in wire
 * names (`account_id`, `from`, `to`). The mapping lives here only (D1).
 */
function movementsResource(filters: MovementFilterInput | undefined): string {
  const params = new URLSearchParams();
  const normalized = normalizeFilters(filters);
  if (normalized.accountId !== undefined) {
    params.set('account_id', normalized.accountId);
  }
  if (normalized.from !== undefined) {
    params.set('from', normalized.from);
  }
  if (normalized.to !== undefined) {
    params.set('to', normalized.to);
  }
  const query = params.toString();
  return query === '' ? 'movements' : `movements?${query}`;
}

/**
 * Run `fetchWithUser` for the active user. Queries are only enabled, and the
 * UI only offers mutations, while a user is active, so the rejection is
 * defensive; it keeps the fetchers null-free without non-null assertions.
 */
function requireUser<T>(userId: string | null, fetchWithUser: (user: string) => Promise<T>): Promise<T> {
  if (userId === null) {
    return Promise.reject(new Error('No active user selected'));
  }
  return fetchWithUser(userId);
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/** `["assets", uid]` — the active user's assets. */
export function useAssets(): UseQueryResult<Asset[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: assetsKey(activeUser),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => apiJson<Asset[]>(userScope(user), 'assets')),
  });
}

/** `["asset", uid, id]` — one asset. */
export function useAsset(assetId: string): UseQueryResult<Asset, Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: assetKey(activeUser, assetId),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => apiJson<Asset>(userScope(user), `assets/${assetId}`)),
  });
}

/** `["asset-docs", uid, id]` — the documents of one asset. */
export function useAssetDocuments(assetId: string): UseQueryResult<Document[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: assetDocsKey(activeUser, assetId),
    enabled: activeUser !== null,
    queryFn: () =>
      requireUser(activeUser, (user) => apiJson<Document[]>(userScope(user), `assets/${assetId}/documents`)),
  });
}

/** `["accounts", uid]` — the active user's finance accounts. */
export function useAccounts(): UseQueryResult<Account[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: accountsKey(activeUser),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => apiJson<Account[]>(financeScope(user), 'accounts')),
  });
}

/**
 * `["movements", uid, {accountId, from, to}]` — movements with the supplied
 * filters. `placeholderData: keepPreviousData` keeps the previous list on
 * screen while a filter change re-queries (D5).
 */
export function useMovements(filters?: MovementFilterInput): UseQueryResult<Movement[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: movementsKey(activeUser, filters),
    enabled: activeUser !== null,
    placeholderData: keepPreviousData,
    queryFn: () =>
      requireUser(activeUser, (user) => apiJson<Movement[]>(financeScope(user), movementsResource(filters))),
  });
}

/** `["batches", uid]` — the import batch history. */
export function useBatches(): UseQueryResult<ImportBatch[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: batchesKey(activeUser),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => apiJson<ImportBatch[]>(financeScope(user), 'import-batches')),
  });
}

/** `["batch", uid, id]` — one import batch with its parsed lines. */
export function useBatch(batchId: string): UseQueryResult<ImportBatch, Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: batchKey(activeUser, batchId),
    enabled: activeUser !== null,
    queryFn: () =>
      requireUser(activeUser, (user) => apiJson<ImportBatch>(financeScope(user), `import-batches/${batchId}`)),
  });
}

// ---------------------------------------------------------------------------
// Mutations (D5 invalidation map)
// ---------------------------------------------------------------------------


/**
 * Invalidate EXACTLY the given key prefixes (D5). One `invalidateQueries`
 * call per prefix keeps the "exactly these prefixes" contract observable.
 */
function invalidatePrefixes(queryClient: QueryClient, prefixes: ReadonlyArray<readonly unknown[]>): void {
  for (const prefix of prefixes) {
    // Fire-and-forget: invalidation only schedules refetches.
    void queryClient.invalidateQueries({ queryKey: prefix });
  }
}

/** The D5 prefixes shared by every movement mutation. */
function movementInvalidations(userId: string): ReadonlyArray<readonly unknown[]> {
  return [movementsPrefix(userId), accountsKey(userId)];
}

/**
 * D5: document upload → `["assets", uid]`. The upload goes through the XHR
 * helper (D6) so progress can be reported.
 */
export interface UploadDocumentVariables {
  readonly file: File;
  readonly onProgress?: (event: { loaded: number; total: number }) => void;
}

export function useUploadDocument(): UseMutationResult<Asset, ApiError, UploadDocumentVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Asset, ApiError, UploadDocumentVariables>({
    mutationFn: ({ file, onProgress }: UploadDocumentVariables) =>
      requireUser(activeUser, (user) => uploadDocument(user, file, onProgress ?? (() => undefined))),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [assetsKey(activeUser)]);
    },
  });
}

/**
 * D5: statement upload → `["batches", uid]`, `["batch", uid, id]`. The upload
 * goes through the XHR helper (D6) so progress can be reported.
 */
export interface UploadStatementVariables {
  readonly accountId: string;
  readonly file: File;
  readonly onProgress?: (event: { loaded: number; total: number }) => void;
}

export function useUploadStatement(): UseMutationResult<ImportBatch, ApiError, UploadStatementVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<ImportBatch, ApiError, UploadStatementVariables>({
    mutationFn: ({ accountId, file, onProgress }: UploadStatementVariables) =>
      requireUser(activeUser, (user) => uploadStatement(user, accountId, file, onProgress ?? (() => undefined))),
    onSuccess: (batch) => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [batchesKey(activeUser), batchKey(activeUser, batch.id)]);
    },
  });
}

/**
 * D5: account create → `["accounts", uid]`.
 */
export function useCreateAccount(): UseMutationResult<Account, ApiError, CreateAccountRequest> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Account, ApiError, CreateAccountRequest>({
    mutationFn: (body: CreateAccountRequest) =>
      requireUser(activeUser, (user) => apiJson<Account>(financeScope(user), 'accounts', { method: 'POST', body })),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [accountsKey(activeUser)]);
    },
  });
}

/**
 * D5: movement create → `["movements", uid]` + `["accounts", uid]`
 * (a new movement changes account balances).
 */
export function useCreateMovement(): UseMutationResult<Movement, ApiError, CreateMovementRequest> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Movement, ApiError, CreateMovementRequest>({
    mutationFn: (body: CreateMovementRequest) =>
      requireUser(activeUser, (user) => apiJson<Movement>(financeScope(user), 'movements', { method: 'POST', body })),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, movementInvalidations(activeUser));
    },
  });
}

/** Variables for the description PATCH. */
export interface PatchDescriptionVariables {
  readonly movementId: string;
  readonly description: string;
}

/**
 * D5: description patch → `["movements", uid]` + `["accounts", uid]`.
 */
export function usePatchDescription(): UseMutationResult<Movement, ApiError, PatchDescriptionVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Movement, ApiError, PatchDescriptionVariables>({
    mutationFn: ({ movementId, description }: PatchDescriptionVariables) =>
      requireUser(activeUser, (user) =>
        apiJson<Movement>(financeScope(user), `movements/${movementId}`, {
          method: 'PATCH',
          body: { description },
        }),
      ),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, movementInvalidations(activeUser));
    },
  });
}

/** Variables identifying one movement (delete/unlink). */
export interface MovementIdVariables {
  readonly movementId: string;
}

/**
 * D5: movement delete → `["movements", uid]` + `["accounts", uid]`.
 */
export function useDeleteMovement(): UseMutationResult<void, ApiError, MovementIdVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, MovementIdVariables>({
    mutationFn: ({ movementId }: MovementIdVariables) =>
      requireUser(activeUser, (user) => apiVoid(financeScope(user), `movements/${movementId}`, { method: 'DELETE' })),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, movementInvalidations(activeUser));
    },
  });
}

/** Variables for the movement–document link. */
export interface LinkMovementVariables {
  readonly movementId: string;
  readonly documentId: string;
}

/**
 * D5: movement link → `["movements", uid]` + `["accounts", uid]`.
 */
export function useLinkMovement(): UseMutationResult<Movement, ApiError, LinkMovementVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Movement, ApiError, LinkMovementVariables>({
    mutationFn: ({ movementId, documentId }: LinkMovementVariables) =>
      requireUser(activeUser, (user) =>
        apiJson<Movement>(financeScope(user), `movements/${movementId}/link`, {
          method: 'POST',
          body: { document_id: documentId },
        }),
      ),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, movementInvalidations(activeUser));
    },
  });
}

/**
 * D5: movement unlink → `["movements", uid]` + `["accounts", uid]`.
 */
export function useUnlinkMovement(): UseMutationResult<void, ApiError, MovementIdVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, MovementIdVariables>({
    mutationFn: ({ movementId }: MovementIdVariables) =>
      requireUser(activeUser, (user) => apiVoid(financeScope(user), `movements/${movementId}/link`, { method: 'DELETE' })),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, movementInvalidations(activeUser));
    },
  });
}

/** Variables identifying one import batch (commit/discard). */
export interface BatchIdVariables {
  readonly batchId: string;
}

/**
 * D5: batch commit → `["batches", uid]` + `["batch", uid, id]` AND — commit
 * writes movements — `["movements", uid]` + `["accounts", uid]`.
 */
export function useCommitBatch(): UseMutationResult<CommitSummary, ApiError, BatchIdVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<CommitSummary, ApiError, BatchIdVariables>({
    mutationFn: ({ batchId }: BatchIdVariables) =>
      requireUser(activeUser, (user) =>
        apiJson<CommitSummary>(financeScope(user), `import-batches/${batchId}/commit`, { method: 'POST' }),
      ),
    onSuccess: (_summary, variables) => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [
        batchesKey(activeUser),
        batchKey(activeUser, variables.batchId),
        movementsPrefix(activeUser),
        accountsKey(activeUser),
      ]);
    },
  });
}

/**
 * D5: batch discard → `["batches", uid]` + `["batch", uid, id]` (no movements
 * are written, so movements/accounts stay untouched).
 */
export function useDiscardBatch(): UseMutationResult<ImportBatch, ApiError, BatchIdVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<ImportBatch, ApiError, BatchIdVariables>({
    mutationFn: ({ batchId }: BatchIdVariables) =>
      requireUser(activeUser, (user) =>
        apiJson<ImportBatch>(financeScope(user), `import-batches/${batchId}/discard`, { method: 'POST' }),
      ),
    onSuccess: (_batch, variables) => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [batchesKey(activeUser), batchKey(activeUser, variables.batchId)]);
    },
  });
}
