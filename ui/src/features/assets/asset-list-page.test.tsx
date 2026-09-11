import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { axe } from 'vitest-axe';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AssetListPage } from './asset-list-page';
import * as hooks from '@/features/docs/hooks';
import { MemoryRouter } from 'react-router';

const mockNavigate = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('@/features/docs/hooks', () => ({
  useAssets: vi.fn(),
}));

/**
 * Flush pending macrotasks so Radix's open-menu `setTimeout` callbacks (focus
 * management, pointer-grace timers) fire and the event loop drains. Without
 * this, a subsequent `await` while the menu's timers are still queued makes
 * vitest wait for a macrotask chain that never settles → test timeout.
 */
function flushMacrotasks(ms = 500): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function asset(id: string, data: Record<string, unknown> = {}): unknown {
  return {
    id,
    data: { metadata: {}, ...data },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  };
}

function mockAssets(data: unknown[]): void {
  vi.mocked(hooks.useAssets).mockReturnValue({
    data,
    isLoading: false,
    error: null,
  } as any);
}

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
});

function renderPage() {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <AssetListPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AssetListPage', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
  });

  it('renders loading state', () => {
    vi.mocked(hooks.useAssets).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as any);
    renderPage();
    expect(screen.getByText(/loading/i)).toBeDefined();
  });

  it('renders error state', () => {
    vi.mocked(hooks.useAssets).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Network error'),
    } as any);
    renderPage();
    expect(screen.getByText(/failed to load assets/i)).toBeDefined();
  });

  it('renders the empty state and its CTA points to the landing page', () => {
    mockAssets([]);
    renderPage();
    expect(screen.getByText(/no assets yet/i)).toBeDefined();
    fireEvent.click(screen.getByRole('button', { name: /add something/i }));
    expect(mockNavigate).toHaveBeenCalledWith('/');
  });

  it('renders name, brand, and model as distinct columns and is axe-clean', async () => {
    mockAssets([
      asset('a1', {
        name: 'Microwave Oven',
        brand: 'IFB',
        model: '30BRC2',
        serial_number: 'SN123',
        asset_category: 'appliance',
        purchase_date: '2024-01-15T00:00:00Z',
        warranty_end: '2099-01-01T00:00:00Z',
      }),
    ]);
    const { container } = renderPage();

    expect(screen.getByText('Microwave Oven')).toBeInTheDocument();
    expect(screen.getByText('IFB')).toBeInTheDocument();
    expect(screen.getByText('30BRC2')).toBeInTheDocument();

    // Distinct column headers, each present exactly once.
    expect(screen.getByRole('columnheader', { name: 'Name' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Brand' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Model' })).toBeInTheDocument();

    // A nameless asset still shows its brand/model (name cell falls back to '-').
    expect(await axe(container)).toHaveNoViolations();
  });

  it('hides a column via the shared View dropdown', async () => {
    mockAssets([
      asset('a1', {
        name: 'Microwave Oven',
        brand: 'IFB',
        model: '30BRC2',
        purchase_date: '2024-01-15T00:00:00Z',
      }),
    ]);
    renderPage();

    const viewButton = screen.getByRole('button', { name: /toggle column visibility/i });
    fireEvent.pointerDown(viewButton);
    await waitFor(() => {
      expect(screen.getByRole('menuitemcheckbox', { name: 'Brand' })).toBeInTheDocument();
    });

    // Uncheck Brand → its header disappears while Name stays.
    fireEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Brand' }));
    await flushMacrotasks();
    await waitFor(() => {
      expect(screen.queryByRole('columnheader', { name: 'Brand' })).not.toBeInTheDocument();
    });
    expect(screen.getByRole('columnheader', { name: 'Name' })).toBeInTheDocument();
    // Radix DropdownMenu open/close is slow in jsdom (~17-20s/interaction); the
    // asset composer flow now also mounts on this page, adding render overhead.
    // 60s gives headroom under parallel load (matches the other Radix-menu tests).
  }, 60000);

  it('sorts by purchase date ascending then descending', async () => {
    mockAssets([
      asset('a2', {
        name: 'Second',
        brand: 'Zeta',
        purchase_date: '2025-06-01T00:00:00Z',
      }),
      asset('a1', {
        name: 'First',
        brand: 'Alpha',
        purchase_date: '2024-01-15T00:00:00Z',
      }),
    ]);
    renderPage();

    // First click → ascending: earlier (2024) row before later (2025) row.
    fireEvent.click(screen.getByRole('button', { name: /^sort by purchase date/i }));
    await waitFor(() => {
      const rows = screen.getAllByRole('row').slice(1); // drop header row
      expect(rows[0].textContent).toContain('First');
      expect(rows[1].textContent).toContain('Second');
    });

    // Second click → descending: 2025 row first.
    fireEvent.click(screen.getByRole('button', { name: /^sort by purchase date/i }));
    await waitFor(() => {
      const rows = screen.getAllByRole('row').slice(1);
      expect(rows[0].textContent).toContain('Second');
      expect(rows[1].textContent).toContain('First');
    });
  });

  it('exposes the shared DataTable controls (View dropdown + pagination footer)', () => {
    mockAssets([asset('a1', { name: 'Microwave Oven', brand: 'IFB', model: '30BRC2' })]);
    renderPage();

    // Shared toolbar "View" (column visibility) control.
    expect(screen.getByRole('button', { name: /toggle column visibility/i })).toBeInTheDocument();
    // Shared pagination footer.
    expect(screen.getByText(/rows per page/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'First page' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeInTheDocument();
  });

  it('navigates to the asset detail when a row is clicked', () => {
    mockAssets([
      asset('a1', {
        name: 'Microwave Oven',
        brand: 'IFB',
        model: '30BRC2',
      }),
    ]);
    renderPage();

    const row = screen.getByText('Microwave Oven').closest('tr');
    expect(row).not.toBeNull();
    fireEvent.click(row as Element);
    expect(mockNavigate).toHaveBeenCalledWith('/assets/a1');
  });

  it('marks an active warranty with an "Active" badge and no red text', () => {
    mockAssets([
      asset('a1', {
        name: 'Active asset',
        brand: 'B',
        warranty_end: '2099-01-01T00:00:00Z',
      }),
    ]);
    renderPage();

    expect(screen.getByText('Active')).toBeInTheDocument();
    const date = screen.getByText('2099-01-01');
    expect(date.className).not.toContain('text-red-600');
    expect(screen.queryByText('Expired')).not.toBeInTheDocument();
  });

  it('marks an expired warranty in red with an "Expired" badge', () => {
    // Today is 2026-09-07; a 2026-01-01 warranty end is strictly before it.
    mockAssets([
      asset('a2', {
        name: 'Old asset',
        brand: 'Old',
        warranty_end: '2026-01-01T00:00:00Z',
      }),
    ]);
    renderPage();

    const date = screen.getByText('2026-01-01');
    expect(date.className).toContain('text-red-600');
    expect(screen.getByText('Expired')).toBeInTheDocument();
    expect(screen.queryByText('Active')).not.toBeInTheDocument();
  });

  it('shows a dash and no badge when warranty_end is null', () => {
    mockAssets([
      asset('a3', {
        name: 'No warranty',
        brand: 'C',
        warranty_end: null,
      }),
    ]);
    renderPage();

    expect(screen.queryByText('Expired')).not.toBeInTheDocument();
    expect(screen.queryByText('Active')).not.toBeInTheDocument();
  });
});

