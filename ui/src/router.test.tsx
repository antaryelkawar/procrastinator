/** @vitest-environment jsdom */
/**
 * Design D9 (route-surface cleanup) — route-table test.
 *
 * Asserts the EXACT route surface of `AppRoutes`:
 *  - each reachable route renders its view (7 routes),
 *  - each legacy path redirects to its closest surviving view (5 paths),
 *  - an unknown path renders the not-found state with a way back,
 *  - `/assets/:assetId` deep-links to the asset detail view.
 *
 * Mocking strategy (Pattern A, same as `app-shell.test.tsx`): every data-hook
 * module is `vi.mock`ed so no real network/MSW is hit; each mock returns a
 * non-loading state so headings render synchronously.
 */
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppRoutes } from '@/router';
import { ActiveUserProvider } from '@/context/active-user';
import * as hooks from '@/lib/api/hooks';
import * as docsHooks from '@/features/docs/hooks';
import * as assetsHooks from '@/features/assets/use-asset';
import * as financeHooks from '@/features/finance/hooks';
import * as searchHooks from '@/features/search/use-search';
import * as reviewsHooks from '@/features/reviews/hooks';
import * as docFeatureHooks from '@/features/documents/hooks';

// Pattern A: mock every data-hook module the 7 reachable routes touch so no
// real network/MSW is hit. `@/lib/api/hooks` is spread from the real module
// (key builders like `documentsKey`/`requireUser` are plain functions the
// un-mocked `@/features/documents/hooks` still imports) with `useQuickSearch`
// swapped out.
vi.mock('@/lib/api/hooks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/hooks')>()),
  useQuickSearch: vi.fn(),
}));

vi.mock('@/features/docs/hooks', () => ({
  useAssets: vi.fn(),
  useAssetDocuments: vi.fn(),
  useAccounts: vi.fn(),
}));

vi.mock('@/features/assets/use-asset', () => ({
  useAsset: vi.fn(),
}));

vi.mock('@/features/finance/hooks', () => ({
  useCreateAccount: vi.fn(),
}));

vi.mock('@/features/search/use-search', () => ({
  useSearch: vi.fn(),
}));

vi.mock('@/features/reviews/hooks', () => ({
  useReviews: vi.fn(),
  useApproveReview: vi.fn(),
  useRejectReview: vi.fn(),
}));

vi.mock('@/features/documents/hooks', () => ({
  useDocuments: vi.fn(),
  useDeleteDocument: vi.fn(),
  useReprocessDocument: vi.fn(),
}));

vi.mock('@/features/docs/composer/hooks', () => ({
  useIngestFile: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
  useIngestText: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
  useReprocessDocument: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
  useKeepDocument: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
}));

// The app shell mounts the sonner Toaster; mock it so no real
// portal/timers run in jsdom.
vi.mock('sonner', () => ({ Toaster: () => null, toast: vi.fn() }));

function renderApp(initialPath = '/'): ReturnType<typeof render> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <ActiveUserProvider queryClient={queryClient}>
          <AppRoutes />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('route table (design D9)', () => {
  beforeEach(() => {
    vi.mocked(docsHooks.useAssets).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(assetsHooks.useAsset).mockReturnValue({ data: { id: 'inv-42', data: { doc_type: 'invoice', metadata: {} }, created_at: '', updated_at: '' }, isLoading: false, error: null } as any);
    vi.mocked(docsHooks.useAssetDocuments).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(docsHooks.useAccounts).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(financeHooks.useCreateAccount).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(searchHooks.useSearch).mockReturnValue({ data: undefined, isLoading: false, error: null } as any);
    vi.mocked(reviewsHooks.useReviews).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(reviewsHooks.useApproveReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(reviewsHooks.useRejectReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(hooks.useQuickSearch).mockReturnValue({ data: { results: [] }, isLoading: false } as any);
    vi.mocked(docFeatureHooks.useDocuments).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(docFeatureHooks.useDeleteDocument).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(docFeatureHooks.useReprocessDocument).mockReturnValue({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null } as any);
  });

  it('renders the landing page at /', async () => {
    renderApp('/');
    await screen.findByRole('heading', { name: 'Insights' });
  });

  it('renders the search view at /search', async () => {
    renderApp('/search');
    // No `?q=` param → the page renders its empty prompt state with the
    // stable "Search" h1.
    await screen.findByRole('heading', { level: 1, name: 'Search' });
  });

  it('renders the review queue at /ingest/reviews', async () => {
    renderApp('/ingest/reviews');
    await screen.findByRole('heading', { level: 1, name: 'Ingest review queue' });
  });

  it('renders the asset list at /assets', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
  });

  it('renders the documents list at /documents', async () => {
    renderApp('/documents');
    await screen.findByRole('heading', { level: 1, name: 'Documents' });
  });

  it('renders the accounts view at /finance/accounts', async () => {
    renderApp('/finance/accounts');
    await screen.findByRole('heading', { level: 1, name: 'Accounts' });
  });

  it('deep-links /assets/:assetId to the asset detail view', async () => {
    renderApp('/assets/inv-42');
    expect(await screen.findByRole('heading', { level: 1, name: /Asset/i })).toBeInTheDocument();
  });

  it('redirects the legacy /add to the landing page', async () => {
    renderApp('/add');
    await screen.findByRole('heading', { name: 'Insights' });
  });

  it('redirects legacy finance sub-routes to /finance/accounts', async () => {
    const legacyPaths = [
      '/finance/movements',
      '/finance/import',
      '/finance/import/history',
      '/finance/import/batch-7',
    ];
    for (const path of legacyPaths) {
      // Render into a fresh container so `screen` queries stay scoped to the
      // current render (each legacy path must reach Accounts on its own).
      const { unmount } = renderApp(path);
      await waitFor(() =>
        expect(screen.getByRole('heading', { level: 1, name: 'Accounts' })).toBeInTheDocument(),
      );
      unmount();
    }
  });

  it('renders a not-found state for unknown routes with a way back', () => {
    renderApp('/no/such/page');
    expect(screen.getByRole('heading', { level: 1, name: 'Page not found' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Go to assets' })).toBeInTheDocument();
  });
});
