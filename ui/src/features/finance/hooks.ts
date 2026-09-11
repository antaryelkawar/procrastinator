/**
 * Finance-feature data-layer hooks (asset-management-v2 modular-structure).
 *
 * Co-located with the finance feature — every hook below is consumed only by
 * files under `features/finance/`. Shared query-key helpers, `requireUser`, and
 * the client/upload/errors plumbing stay in `lib/api/` (single source of truth).
 *
 * The shared `useAccounts` hook (add + finance) lives in `@/features/docs/hooks`.
 */
import { keepPreviousData, useMutation, useQuery, useQueryClient, type UseMutationResult, type UseQueryResult } from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import { uploadStatement } from '@/lib/api/upload';
import {
  accountsKey,
  batchKey,
  batchesKey,
  invalidatePrefixes,
  movementInvalidations,
  movementsKey,
  movementsPrefix,
  requireUser,
  type MovementFilterInput,
} from '@/lib/api/hooks';
import { ApiError } from '@/lib/api/errors';
import { useActiveUser } from '@/context/active-user';
import type {
  Account,
  CommitSummary,
  CreateAccountRequest,
  CreateMovementInput,
  ImportBatch,
  Movement,
} from '@/lib/api/schema';

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * `["movements", uid, {accountId, from, to}]` — movements with the supplied
 * filters. `placeholderData: keepPreviousData` keeps the previous list on
 * screen while a filter change re-queries (D5).
 */
export function useMovements(filters?: MovementFilterInput): UseQueryResult<Movement[], Error> {
  const { activeUser } = useActiveUser();
  const normalized: MovementFilterInput = {
    accountId: filters?.accountId,
    from: filters?.from,
    to: filters?.to,
  };
  const params = {
    account_id: normalized.accountId,
    from: normalized.from,
    to: normalized.to,
  };
  return useQuery({
    queryKey: movementsKey(activeUser, filters),
    enabled: activeUser !== null,
    placeholderData: keepPreviousData,
    queryFn: () => requireUser(activeUser, (user) => client.listMovements(user, params)),
  });
}

/** `["batches", uid]` — the import batch history. */
export function useBatches(): UseQueryResult<ImportBatch[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: batchesKey(activeUser),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => client.listImportBatches(user)),
  });
}

/** `["batch", uid, id]` — one import batch with its parsed lines. */
export function useBatch(batchId: string): UseQueryResult<ImportBatch, Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: batchKey(activeUser, batchId),
    enabled: activeUser !== null && batchId !== '',
    queryFn: () => requireUser(activeUser, (user) => client.getImportBatch(user, batchId)),
  });
}

// ---------------------------------------------------------------------------
// Mutations (D5 invalidation map)
// ---------------------------------------------------------------------------

/** Variables for the statement upload. */
export interface UploadStatementVariables {
  readonly accountId: string;
  readonly file: File;
  readonly onProgress?: (event: { loaded: number; total: number }) => void;
}

/**
 * D5: statement upload → `["batches", uid]`, `["batch", uid, id]`. The upload
 * goes through the XHR helper (D6) so progress can be reported.
 */
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
      requireUser(activeUser, (user) => client.createAccount(user, body)),
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
export function useCreateMovement(): UseMutationResult<Movement, ApiError, CreateMovementInput> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Movement, ApiError, CreateMovementInput>({
    mutationFn: (body: CreateMovementInput) =>
      requireUser(activeUser, (user) => client.createMovement(user, body)),
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
        client.patchMovement(user, movementId, {
          description,
          amount: '',
          currency: '',
          occurred_on: '',
          kind: '',
          source_account_id: '',
          destination_account_id: '',
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
      requireUser(activeUser, (user) => client.deleteMovement(user, movementId)),
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
        client.linkMovement(user, movementId, { document_id: documentId }),
      ),
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
      requireUser(activeUser, (user) => client.commitImportBatch(user, batchId)),
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
      requireUser(activeUser, (user) => client.discardImportBatch(user, batchId)),
    onSuccess: (_batch, variables) => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [batchesKey(activeUser), batchKey(activeUser, variables.batchId)]);
    },
  });
}
