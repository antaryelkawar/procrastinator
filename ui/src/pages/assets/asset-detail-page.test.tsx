import { describe, expect, beforeEach, vi, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AssetDetailPage } from './asset-detail-page';
import * as hooks from '@/lib/api/hooks';
import { Asset, Document } from '@/lib/api/schema';

vi.mock('@/lib/api/hooks', async () => {
  const actual = await vi.importActual('@/lib/api/hooks');
  return {
    ...actual,
    useAsset: vi.fn(),
    useAssetDocuments: vi.fn(),
  };
});

const mockAsset: Asset = {
  id: 'asset-1',
  brand: 'Apple',
  model: 'iPhone 15',
  serial_number: 'SN12345',
  purchase_date: '2023-01-01T00:00:00Z',
  warranty_end: '2024-01-01T00:00:00Z',
  price: '39999.99',
  currency: 'INR',
  doc_type: 'other',
  metadata: {},
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
};

const mockDocs: Document[] = [
  {
    id: 'doc-1',
    doc_type: 'other',
    source_filename: 'receipt.pdf',
    source_uploaded_at: '2023-01-02T10:00:00Z',
    created_at: '2023-01-02T10:00:00Z',
  },
];

describe('AssetDetailPage', () => {
  const queryClient = new QueryClient();

  beforeEach(() => {
    (hooks.useAsset as any).mockReturnValue({ data: mockAsset, isLoading: false, error: null });
    (hooks.useAssetDocuments as any).mockReturnValue({ data: mockDocs, isLoading: false, error: null });
  });

  function renderPage(assetId: string) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/assets/${assetId}`]}>
          <Routes>
            <Route path="/assets/:assetId" element={<AssetDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );
  }

  it('renders asset fields correctly', async () => {
    renderPage('asset-1');
    expect(screen.getByText('Apple iPhone 15')).toBeInTheDocument();
    expect(screen.getByText('SN12345')).toBeInTheDocument();
  });

  it('renders document list', async () => {
    renderPage('asset-1');
    expect(screen.getByText('receipt.pdf')).toBeInTheDocument();
  });

  it('renders not found state for unknown asset', async () => {
    (hooks.useAsset as any).mockReturnValue({ data: undefined, isLoading: false, error: { status: 404 } });
    (hooks.useAssetDocuments as any).mockReturnValue({ data: undefined, isLoading: false, error: null });
    
    renderPage('unknown-id');
    expect(screen.getByText('Asset not found')).toBeInTheDocument();
  });
});
