/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider } from '@/context/active-user';
import { QuickSearch } from '@/features/docs/search/quick-search';
import * as hooks from '@/lib/api/hooks';
import type { SearchHit } from '@/lib/api/schema';

vi.mock('@/lib/api/hooks', async () => {
  const actual = await vi.importActual('@/lib/api/hooks');
  return {
    ...actual,
    useQuickSearch: vi.fn(),
  };
});

const mockUseQuickSearch = vi.mocked(hooks.useQuickSearch);

const mockNavigate = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

const mockHits: SearchHit[] = [
  { type: 'asset', id: 'a1', title: 'MacBook Pro', subtitle: 'Laptop', confidence: 0.95 },
  { type: 'account', id: 'acc1', title: 'Checking Account', confidence: null },
  { type: 'movement', id: 'm1', title: 'Salary', subtitle: 'Income', confidence: null },
];

function renderWithProviders() {
  const queryClient = new QueryClient();
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <ActiveUserProvider queryClient={queryClient}>
          <QuickSearch />
        </ActiveUserProvider>
      </QueryClientProvider>
    </MemoryRouter>
  );
}

describe('QuickSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.setItem('activeUser', 'test-user');
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('shows dropdown with hits when results are available', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    await waitFor(() => {
      expect(screen.getByRole('listbox')).toBeInTheDocument();
      expect(screen.getAllByRole('option')).toHaveLength(3);
    });
  });

  it('closes dropdown when query returns zero results (F-03)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: [] },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'nonexistent' } });

    // The component closes the dropdown when hits.length === 0
    await waitFor(() => {
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });
  });

  it('dropdown opens when query matches hits (F-04)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    // Wait for the dropdown to open (debounce settles)
    await waitFor(() => {
      expect(screen.getByRole('listbox')).toBeInTheDocument();
      expect(screen.getAllByRole('option')).toHaveLength(3);
    });
  });

  it('navigates to hit route when option is clicked (F-04)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    await waitFor(() => {
      expect(screen.getAllByRole('option')).toHaveLength(3);
    });

    const firstOption = screen.getAllByRole('option')[0];
    fireEvent.click(firstOption);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/assets/a1');
    });
  });

  it('shows options in dropdown (F-04)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    // Wait for the dropdown to open (debounce settles)
    await waitFor(() => {
      expect(screen.getAllByRole('option')).toHaveLength(3);
      expect(screen.getByText('MacBook Pro')).toBeInTheDocument();
      expect(screen.getByText('Checking Account')).toBeInTheDocument();
      expect(screen.getByText('Salary')).toBeInTheDocument();
    });
  });

  it('shows type badge in dropdown items (N-01)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    await waitFor(() => {
      expect(screen.getByText('asset')).toBeInTheDocument();
      expect(screen.getByText('account')).toBeInTheDocument();
      expect(screen.getByText('movement')).toBeInTheDocument();
    });
  });

  it('shows confidence percentage when present (F-01)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    await waitFor(() => {
      expect(screen.getByText('95%')).toBeInTheDocument();
    });
  });

  it('shows subtitle when present (F-01)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    await waitFor(() => {
      expect(screen.getByText('Laptop')).toBeInTheDocument();
      expect(screen.getByText('Income')).toBeInTheDocument();
    });
  });

  it('clears input after selecting a hit (F-04)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'test' } });

    await waitFor(() => {
      expect(screen.getAllByRole('option')).toHaveLength(3);
    });

    fireEvent.click(screen.getAllByRole('option')[0]);

    await waitFor(() => {
      expect(input.value).toBe('');
    });
  });

  it('has correct ARIA attributes (F-05)', async () => {
    mockUseQuickSearch.mockReturnValue({
      data: { results: mockHits },
    } as any);

    renderWithProviders();
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'test' } });

    await waitFor(() => {
      expect(input).toHaveAttribute('aria-expanded', 'true');
      expect(input).toHaveAttribute('aria-controls', 'quick-search-listbox');
      expect(input).toHaveAttribute('aria-autocomplete', 'list');
      expect(screen.getByRole('listbox')).toHaveAttribute('aria-label', 'Search results');
    });
  });

  it('has a label for accessibility (F-05)', () => {
    mockUseQuickSearch.mockReturnValue({ data: undefined } as any);
    renderWithProviders();
    expect(screen.getByText('Search')).toBeInTheDocument();
  });
});
