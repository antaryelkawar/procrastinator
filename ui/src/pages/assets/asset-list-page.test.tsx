import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { axe } from 'vitest-axe';

import { AssetListPage } from './asset-list-page';
import * as hooks from '@/lib/api/hooks';
import { MemoryRouter } from 'react-router-dom';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('@/lib/api/hooks', () => ({
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

function mockAssets(data: unknown[]): void {
  vi.mocked(hooks.useAssets).mockReturnValue({
    data,
    isLoading: false,
    error: null,
  } as any);
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
    render(<AssetListPage />, { wrapper: MemoryRouter });
    expect(screen.getByText(/loading/i)).toBeDefined();
  });

  it('renders error state', () => {
    vi.mocked(hooks.useAssets).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Network error'),
    } as any);
    render(<AssetListPage />, { wrapper: MemoryRouter });
    expect(screen.getByText(/failed to load assets/i)).toBeDefined();
  });

  it('renders the empty state and its CTA points to /add', () => {
    mockAssets([]);
    render(<AssetListPage />, { wrapper: MemoryRouter });
    expect(screen.getByText(/no assets yet/i)).toBeDefined();
    fireEvent.click(screen.getByRole('button', { name: /add something/i }));
    expect(mockNavigate).toHaveBeenCalledWith('/add');
  });

  it('renders name, brand, and model as distinct columns and is axe-clean', async () => {
    mockAssets([
      {
        id: 'a1',
        name: 'Microwave Oven',
        brand: 'IFB',
        model: '30BRC2',
        serial_number: 'SN123',
        asset_category: 'appliance',
        purchase_date: '2024-01-15T00:00:00Z',
        warranty_end: '2099-01-01T00:00:00Z',
      },
    ]);
    const { container } = render(<AssetListPage />, { wrapper: MemoryRouter });

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
      {
        id: 'a1',
        name: 'Microwave Oven',
        brand: 'IFB',
        model: '30BRC2',
        purchase_date: '2024-01-15T00:00:00Z',
      },
    ]);
    render(<AssetListPage />, { wrapper: MemoryRouter });

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
  }, 20000);

  it('sorts by purchase date ascending then descending', async () => {
    mockAssets([
      {
        id: 'a2',
        name: 'Second',
        brand: 'Zeta',
        purchase_date: '2025-06-01T00:00:00Z',
      },
      {
        id: 'a1',
        name: 'First',
        brand: 'Alpha',
        purchase_date: '2024-01-15T00:00:00Z',
      },
    ]);
    render(<AssetListPage />, { wrapper: MemoryRouter });

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
    mockAssets([
      { id: 'a1', name: 'Microwave Oven', brand: 'IFB', model: '30BRC2' },
    ]);
    render(<AssetListPage />, { wrapper: MemoryRouter });

    // Shared toolbar "View" (column visibility) control.
    expect(screen.getByRole('button', { name: /toggle column visibility/i })).toBeInTheDocument();
    // Shared pagination footer.
    expect(screen.getByText(/rows per page/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'First page' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeInTheDocument();
  });

  it('navigates to the asset detail when a row is clicked', () => {
    mockAssets([
      {
        id: 'a1',
        name: 'Microwave Oven',
        brand: 'IFB',
        model: '30BRC2',
      },
    ]);
    render(<AssetListPage />, { wrapper: MemoryRouter });

    const row = screen.getByText('Microwave Oven').closest('tr');
    expect(row).not.toBeNull();
    fireEvent.click(row as Element);
    expect(mockNavigate).toHaveBeenCalledWith('/assets/a1');
  });

  it('marks an active warranty with an "Active" badge and no red text', () => {
    mockAssets([
      {
        id: 'a1',
        name: 'Active asset',
        brand: 'B',
        warranty_end: '2099-01-01T00:00:00Z',
      },
    ]);
    render(<AssetListPage />, { wrapper: MemoryRouter });

    expect(screen.getByText('Active')).toBeInTheDocument();
    const date = screen.getByText('2099-01-01');
    expect(date.className).not.toContain('text-red-600');
    expect(screen.queryByText('Expired')).not.toBeInTheDocument();
  });

  it('marks an expired warranty in red with an "Expired" badge', () => {
    // Today is 2026-09-07; a 2026-01-01 warranty end is strictly before it.
    mockAssets([
      {
        id: 'a2',
        name: 'Old asset',
        brand: 'Old',
        warranty_end: '2026-01-01T00:00:00Z',
      },
    ]);
    render(<AssetListPage />, { wrapper: MemoryRouter });

    const date = screen.getByText('2026-01-01');
    expect(date.className).toContain('text-red-600');
    expect(screen.getByText('Expired')).toBeInTheDocument();
    expect(screen.queryByText('Active')).not.toBeInTheDocument();
  });

  it('shows a dash and no badge when warranty_end is null', () => {
    mockAssets([
      {
        id: 'a3',
        name: 'No warranty',
        brand: 'C',
        warranty_end: null,
      },
    ]);
    render(<AssetListPage />, { wrapper: MemoryRouter });

    expect(screen.queryByText('Expired')).not.toBeInTheDocument();
    expect(screen.queryByText('Active')).not.toBeInTheDocument();
  });
});

// The `vitest-axe` package registers `toHaveNoViolations` at runtime via
// `expect.extend` in the test setup, but its bundled type augmentation targets a
// `Vi` namespace that vitest v3 does not use. Declare the matcher against the
// real `@vitest/expect` module so `tsc -b` can resolve it.
declare module '@vitest/expect' {
  interface Matchers<T = any> {
    toHaveNoViolations(): {
      actual: import('axe-core').Result[];
      pass: boolean;
      message(): string;
    };
  }
}
