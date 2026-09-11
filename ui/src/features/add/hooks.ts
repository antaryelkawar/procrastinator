/**
 * Add-feature data-layer hooks (asset-management-v2 modular-structure).
 *
 * Co-located with the add feature — both hooks are consumed only by
 * `features/add/add-page.tsx`. Shared query-key helpers, `requireUser`, and the
 * client/errors plumbing stay in `lib/api/` (single source of truth). The
 * shared `useAccounts` hook (add + finance) lives in `@/features/docs/hooks`.
 */
import { useMutation, useQueryClient, type UseMutationResult } from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import {
  assetKey,
  assetsKey,
  batchesKey,
  invalidatePrefixes,
  requireUser,
} from '@/lib/api/hooks';
import { ApiError } from '@/lib/api/errors';
import { useActiveUser } from '@/context/active-user';
import type { AddItemOutcome, Asset } from '@/lib/api/schema';

/** Variables identifying one asset (restore). */
export interface AssetIdVariables {
  readonly assetId: string;
}

/**
 * D5: asset restore → `["assets", uid]` + `["asset", uid, id]` (the restored
 * asset reappears in the list and its detail changes).
 */
export function useRestoreAsset(): UseMutationResult<Asset, ApiError, AssetIdVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<Asset, ApiError, AssetIdVariables>({
    mutationFn: ({ assetId }: AssetIdVariables) =>
      requireUser(activeUser, (user) => client.restoreAsset(user, assetId)),
    onSuccess: (_asset, variables) => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [assetsKey(activeUser), assetKey(activeUser, variables.assetId)]);
    },
  });
}

/** Variables for the unified add (files and/or text, optional account scope). */
export interface AddVariables {
  readonly files?: File[];
  readonly text?: string;
  readonly account_id?: string;
}

/**
 * D5: unified add → `["assets", uid]` (committed assets appear) AND
 * `["batches", uid]` (a `statement_preview` outcome creates an import batch).
 * Exactly these two prefixes — nothing else changes.
 */
export function useAdd(): UseMutationResult<AddItemOutcome[], ApiError, AddVariables> {
  const { activeUser } = useActiveUser();
  const queryClient = useQueryClient();
  return useMutation<AddItemOutcome[], ApiError, AddVariables>({
    mutationFn: (body: AddVariables) =>
      requireUser(activeUser, (user) => client.addItems(user, body)),
    onSuccess: () => {
      if (activeUser === null) {
        return;
      }
      invalidatePrefixes(queryClient, [assetsKey(activeUser), batchesKey(activeUser)]);
    },
  });
}
