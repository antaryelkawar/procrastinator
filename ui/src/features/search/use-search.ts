/**
 * Paged-search query hook, co-located with the search feature
 * (asset-management-v2 modular-structure). Consumed only by
 * `features/search/search-results-page.tsx`.
 *
 * Signature: `useSearch(q, filtersOrPage?, pageOrPageSize?, pageSize?)`.
 * The second argument accepts EITHER the structured filters
 * (`SearchFilterInput`) OR a page number (back-compat with the pre-filters
 * `useSearch(q, page, pageSize)` call shape used by the page):
 * `useSearch('q', 2, 20)` ≡ `useSearch('q', undefined, 2, 20)`. Filters are
 * normalized into the query key (D5) so any filter change re-queries; only
 * supplied fields are mapped to wire params (`category`, `purchase_from`, …)
 * in the query function.
 */
import { keepPreviousData, useQuery, type UseQueryResult } from '@tanstack/react-query';
import * as client from '@/lib/api/client';
import { normalizeSearchFilters, requireUser, searchKey, type SearchFilterInput } from '@/lib/api/hooks';
import { useActiveUser } from '@/context/active-user';
import type { SearchResultsPage } from '@/lib/api/schema';

export function useSearch(
  q: string,
  filtersOrPage?: SearchFilterInput | number,
  pageOrPageSize = 1,
  pageSize = 20,
): UseQueryResult<SearchResultsPage, Error> {
  const { activeUser } = useActiveUser();
  const trimmed = q.trim();
  const isLegacy = typeof filtersOrPage === 'number';
  const filters = isLegacy ? undefined : filtersOrPage;
  const page = isLegacy ? filtersOrPage : pageOrPageSize;
  // In legacy mode `pageOrPageSize` is the pageSize (old 3rd arg); in
  // structured mode `pageSize` is the 4th arg.
  const resolvedPageSize = isLegacy ? pageOrPageSize : pageSize;
  const normalized = normalizeSearchFilters(filters);
  const params = {
    q: trimmed,
    page,
    page_size: resolvedPageSize,
    category: normalized.category,
    brand: normalized.brand,
    purchase_from: normalized.purchaseFrom,
    purchase_to: normalized.purchaseTo,
    warranty_status: normalized.warrantyStatus,
    has_documents: normalized.hasDocuments,
    doc_classification: normalized.docClassification,
  };
  return useQuery({
    queryKey: searchKey(activeUser, trimmed, page, resolvedPageSize, filters),
    enabled: activeUser !== null && trimmed.length > 0,
    placeholderData: keepPreviousData,
    queryFn: () => requireUser(activeUser, (user) => client.search(user, params)),
  });
}
