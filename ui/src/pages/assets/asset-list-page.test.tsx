import { render, screen, fireEvent } from '@testing-library/react';
import { vi, describe, it, expect } from 'vitest';
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

describe('AssetListPage', () => {
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

  it('renders empty state', () => {
    vi.mocked(hooks.useAssets).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as any);
    render(<AssetListPage />, { wrapper: MemoryRouter });
    expect(screen.getByText(/no assets yet/i)).toBeDefined();
    fireEvent.click(screen.getByText(/upload document/i));
    expect(mockNavigate).toHaveBeenCalledWith('/upload');
  });

  it('renders asset list', async () => {
    const assets = [{
      id: 'a1',
      brand: 'BrandX',
      model: 'ModelY',
      serial_number: 'SN123',
      warranty_end: '2025-01-01T00:00:00Z',
      doc_type: 'invoice',
    }];
    vi.mocked(hooks.useAssets).mockReturnValue({
      data: assets,
      isLoading: false,
      error: null,
    } as any);
    render(<AssetListPage />, { wrapper: MemoryRouter });
    
    expect(screen.getByText('BrandX')).toBeDefined();
    // We expect 2025-01-01 twice (one for table, one for card)
    expect(screen.getAllByText('2025-01-01').length).toBe(2);
    
    const row = screen.getAllByText('BrandX')[0].closest('tr') || screen.getAllByText('BrandX')[0].closest('.p-4');
    if (row) {
      fireEvent.click(row);
      expect(mockNavigate).toHaveBeenCalledWith('/assets/a1');
    }
  });

  it('marks expired warranty in red', async () => {
    // Current date is 2026-09-03
    const expiredAsset = {
      id: 'a2',
      brand: 'Old',
      model: 'Model',
      serial_number: 'SN',
      warranty_end: '2026-01-01T00:00:00Z',
      doc_type: 'warranty',
    };
    vi.mocked(hooks.useAssets).mockReturnValue({
      data: [expiredAsset],
      isLoading: false,
      error: null,
    } as any);
    render(<AssetListPage />, { wrapper: MemoryRouter });
    
    const dates = screen.getAllByText('2026-01-01');
    expect(dates.length).toBe(2);
    dates.forEach(dateEl => {
      expect(dateEl.className).toContain('text-red-600');
    });
  });
});
