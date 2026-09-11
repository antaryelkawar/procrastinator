/**
 * Single-asset query hook, co-located with the assets feature
 * (asset-management-v2 modular-structure). Consumed only by
 * `features/assets/asset-detail-page.tsx`.
 */
import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import { requireUser, assetKey } from '@/lib/api/hooks';
import { useActiveUser } from '@/context/active-user';
import type { Asset } from '@/lib/api/schema';

/** `["asset", uid, id]` — one asset. */
export function useAsset(assetId: string): UseQueryResult<Asset, Error> {
  const { activeUser } = useActiveUser();
  return useQuery({
    queryKey: assetKey(activeUser, assetId),
    enabled: activeUser !== null,
    queryFn: () => requireUser(activeUser, (user) => client.getAsset(user, assetId)),
  });
}
