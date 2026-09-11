import { useEffect } from 'react';
import { useSearchParams } from 'react-router';
import { useSearch } from '@/features/search/use-search';
import { formatConfidence } from '@/lib/format/confidence';
import { Loading } from '@/components/feedback/loading';
import { ErrorState } from '@/components/feedback/error-state';
import { EmptyState } from '@/components/feedback/empty-state';
import { Button } from '@/components/ui/button';
import type { SearchHit } from '@/lib/api/schema';
import { hitRoute } from '@/features/docs/search/quick-search';
import { Link } from 'react-router';

/**
 * P12.6: Paged search results page at `/search` (design D8).
 * Reads `?q=` from URL, renders hits in endpoint order with type indicator,
 * title, optional subtitle, optional confidence, and pagination controls.
 */
export function SearchResultsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const q = searchParams.get('q') ?? '';
  const page = Math.max(1, Number(searchParams.get('page')) || 1);
  const pageSize = 20;

  const { data, isLoading, isError, refetch } = useSearch(q, page, pageSize);

  // Reset to page 1 when query changes
  useEffect(() => {
    if (page !== 1 && q.trim() !== '') {
      setSearchParams({ q, page: '1' }, { replace: true });
    }
  }, [q, page, setSearchParams]);

  const updatePage = (newPage: number) => {
    setSearchParams({ q, page: String(newPage) });
  };

  if (!q.trim()) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">Search</h1>
        <EmptyState
          title="No search query"
          description="Enter a search term in the search box above to find assets, accounts, movements, documents, and import batches."
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Search results for "{q}"</h1>

      {isLoading ? (
        <Loading label="Searching" rows={6} />
      ) : isError ? (
        <ErrorState message="Failed to load search results" onRetry={() => refetch()} />
      ) : !data || data.results.length === 0 ? (
        <EmptyState
          title="No results found"
          description={`No matches for "${q}". Try a different search term.`}
        />
      ) : (
        <>
          <div className="text-sm text-muted-foreground">
            {data.total} result{data.total !== 1 ? 's' : ''} (page {data.page} of {Math.ceil(data.total / data.page_size)})
          </div>
          <ul className="space-y-2">
            {data.results.map((hit) => (
              <SearchHitRow key={`${hit.type}-${hit.id}`} hit={hit} />
            ))}
          </ul>
          <PaginationControls
            page={data.page}
            pageSize={data.page_size}
            total={data.total}
            onPageChange={updatePage}
          />
        </>
      )}
    </div>
  );
}

function SearchHitRow({ hit }: { hit: SearchHit }) {
  const confidence = formatConfidence(hit.confidence);
  const route = hitRoute(hit);
  const typeLabel = hit.type.replace('_', ' ');

  return (
    <li>
      <Link
        to={route}
        className="block rounded-lg border p-4 transition-colors hover:bg-accent/50"
      >
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            <div className="mb-1 flex items-center gap-2">
              <span className="rounded bg-muted px-2 py-0.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                {typeLabel}
              </span>
            </div>
            <div className="font-medium">{hit.title}</div>
            {hit.subtitle ? (
              <div className="mt-1 text-sm text-muted-foreground">{hit.subtitle}</div>
            ) : null}
          </div>
          {confidence ? (
            <div className="shrink-0 text-sm text-muted-foreground">{confidence}</div>
          ) : null}
        </div>
      </Link>
    </li>
  );
}

interface PaginationControlsProps {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
}

function PaginationControls({ page, pageSize, total, onPageChange }: PaginationControlsProps) {
  const totalPages = Math.ceil(total / pageSize);
  if (totalPages <= 1) return null;

  return (
    <div className="flex items-center justify-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={page <= 1}
        onClick={() => onPageChange(page - 1)}
      >
        Previous
      </Button>
      <span className="text-sm text-muted-foreground">
        Page {page} of {totalPages}
      </span>
      <Button
        variant="outline"
        size="sm"
        disabled={page >= totalPages}
        onClick={() => onPageChange(page + 1)}
      >
        Next
      </Button>
    </div>
  );
}
