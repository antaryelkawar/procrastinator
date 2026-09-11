/**
 * Shared, multi-feature data-layer hooks (asset-management-v2 modular-structure).
 *
 * These hooks are each consumed by 2+ features (so they cannot live inside a
 * single feature folder) but are not part of the shared `lib/api` plumbing —
 * they are feature-facing query hooks. Per the modular-structure delta,
 * `features/docs/` is the ONLY cross-feature import target, so these live here.
 *
 *   | hook               | consumers                       |
 *   | ------------------ | ------------------------------- |
 *   | useAssets          | assets, finance                 |
 *   | useAssetDocuments  | assets, finance                 |
 *   | useAccounts        | add, finance                    |
 *
 * The query-key helpers, `requireUser`, and the shared client/upload/errors
 * plumbing stay in `lib/api/` (the single source of truth for keys + plumbing).
 */
import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import { requireUser, accountsKey, assetsKey, assetDocsKey } from '@/lib/api/hooks';
import { useActiveUser } from '@/context/active-user';
import type { Account, Asset, Document } from '@/lib/api/schema';

/** `["assets", uid]` — the active user's assets. */
export function useAssets(): UseQueryResult<Asset[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: assetsKey(activeUser),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => client.listAssets(user)),
  });
}

/** `["asset-docs", uid, id]` — the documents of one asset. */
export function useAssetDocuments(assetId: string): UseQueryResult<Document[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: assetDocsKey(activeUser, assetId),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => client.listAssetDocuments(user, assetId)),
  });
}

/** `["accounts", uid]` — the active user's finance accounts. */
export function useAccounts(): UseQueryResult<Account[], Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: accountsKey(activeUser),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => client.listAccounts(user)),
  });
}
