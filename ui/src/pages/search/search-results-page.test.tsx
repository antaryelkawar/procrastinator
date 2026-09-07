/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider } from '@/context/active-user';
import { SearchResultsPage } from './search-results-page';
import * as hooks from '@/lib/api/hooks';
import type { SearchHit } from '@/lib/api/schema';

vi.mock('@/lib/api/hooks', async () => {
  const actual = await vi.importActual('@/lib/api/hooks');
  return {
    ...actual,
    useSearch: vi.fn(),
  };
});

const mockUseSearch = vi.mocked(hooks.useSearch);

const mockHits: SearchHit[] = [
  { type: 'asset', id: 'a1', title: 'MacBook Pro', subtitle: 'Laptop', confidence: 0.95 },
  { type: 'account', id: 'acc1', title: 'Checking Account', confidence: null },
  { type: 'movement', id: 'm1', title: 'Salary', subtitle: 'Income', confidence: null },
];

function renderWithProviders(initialRoute = '/search?q=test') {
  const queryClient = new QueryClient();
  return render(
    <MemoryRouter initialEntries={[initialRoute]}>
      <QueryClientProvider client={queryClient}>
        <ActiveUserProvider queryClient={queryClient}>
          <SearchResultsPage />
        </ActiveUserProvider>
      </QueryClientProvider>
    </MemoryRouter>
  );
}

describe('SearchResultsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.setItem('activeUser', 'test-user');
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('shows "No search query" when q parameter is empty (F-07)', () => {
    // Mock useSearch even though it won't be called (component returns early)
    mockUseSearch.mockReturnValue({
      data: { results: [], total: 0, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders('/search');
    expect(screen.getByText('No search query')).toBeInTheDocument();
  });

  it('shows loading state initially (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.getByText(/searching/i)).toBeInTheDocument();
  });

  it('shows error state when query fails (F-07)', () => {
    mockUseSearch.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    expect(screen.getByText(/failed to load search results/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument();
  });

  it('calls refetch when retry button is clicked (F-07)', () => {
    const mockRefetch = vi.fn();
    mockUseSearch.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      refetch: mockRefetch,
    } as any);

    renderWithProviders();
    const retryButton = screen.getByRole('button', { name: /retry/i });
    fireEvent.click(retryButton);
    expect(mockRefetch).toHaveBeenCalled();
  });

  it('shows "No results found" when query returns empty results (F-07)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: [], total: 0, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('No results found')).toBeInTheDocument();
    expect(screen.getByText(/no matches for "test"/i)).toBeInTheDocument();
  });

  it('shows results list when data is available (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    
    expect(screen.getByText('MacBook Pro')).toBeInTheDocument();
    expect(screen.getByText('Checking Account')).toBeInTheDocument();
    expect(screen.getByText('Salary')).toBeInTheDocument();
  });

  it('shows query in heading (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders('/search?q=laptop');
    expect(screen.getByText(/search results for "laptop"/i)).toBeInTheDocument();
  });

  it('shows type badges for each hit (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    
    // Type badges are rendered as uppercase text
    const badges = screen.getAllByText(/^(asset|account|movement|document|import batch)$/i);
    expect(badges.length).toBeGreaterThan(0);
  });

  it('shows confidence when present (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('95%')).toBeInTheDocument();
  });

  it('shows subtitle when present (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('Laptop')).toBeInTheDocument();
    expect(screen.getByText('Income')).toBeInTheDocument();
  });

  it('shows total count and page info (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText(/3 results \(page 1 of 1\)/i)).toBeInTheDocument();
  });

  it('shows pagination when total exceeds pageSize (F-07)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 50, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    
    // Check for the pagination info text (rendered as a single span)
    expect(screen.getByText(/Page 1 of 3/)).toBeInTheDocument();
    
    const prevButton = screen.getByRole('button', { name: /previous/i });
    const nextButton = screen.getByRole('button', { name: /next/i });
    
    expect(prevButton).toBeInTheDocument();
    expect(nextButton).toBeInTheDocument();
  });

  it('disables Previous button on first page (F-07)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 50, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /next/i })).toBeEnabled();
  });

  it('disables Next button on last page (F-07)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 50, page: 3, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByRole('button', { name: /previous/i })).toBeEnabled();
    expect(screen.getByRole('button', { name: /next/i })).toBeDisabled();
  });

  it('shows links to resources (F-06)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    
    // Check that links are rendered (they wrap the hit cards)
    const links = screen.getAllByRole('link');
    expect(links.length).toBeGreaterThan(0);
  });

  it('does not show pagination when total <= pageSize (F-07)', () => {
    mockUseSearch.mockReturnValue({
      data: { results: mockHits, total: 3, page: 1, page_size: 20 },
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.queryByRole('button', { name: /previous/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument();
  });
});
