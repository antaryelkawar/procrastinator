import { describe, expect, beforeEach, vi, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AssetDetailPage } from './asset-detail-page';
import * as useAssetModule from '@/features/assets/use-asset';
import * as docsModule from '@/features/docs/hooks';
import { Asset, Document } from '@/lib/api/schema';

vi.mock('@/features/assets/use-asset', async () => {
  const actual = await vi.importActual('@/features/assets/use-asset');
  return {
    ...actual,
    useAsset: vi.fn(),
  };
});

vi.mock('@/features/docs/hooks', () => ({
  useAssetDocuments: vi.fn(),
}));

const mockAsset: Asset = {
  id: 'asset-1',
  owner_household_id: null,
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
  data: {
    name: null,
    brand: 'Apple',
    model: 'iPhone 15',
    serial_number: 'SN12345',
    asset_category: null,
    category_confidence: null,
    category_user_set: null,
    purchase_date: '2023-01-01T00:00:00Z',
    warranty_end: '2024-01-01T00:00:00Z',
    price: '39999.99',
    currency: 'INR',
    metadata: {},
    confidence: null,
    deleted_at: null,
    merged_into: null,
    merged_at: null,
    merged_assets: null,
  },
};

const mockDocs: Document[] = [
  {
    id: 'doc-1',
    asset_id: 'asset-1',
    source_id: 'src-1',
    source_filename: 'receipt.pdf',
    source_uploaded_at: '2023-01-02T10:00:00Z',
    owner_household_id: null,
    status: 'processed',
    created_at: '2023-01-02T10:00:00Z',
    updated_at: '2023-01-02T10:00:00Z',
    data: {
      doc_type: 'other',
      confidence: null,
      extracted_fields: null,
      raw_extraction: null,
      user_directive: null,
    },
  },
];

describe('AssetDetailPage', () => {
  const queryClient = new QueryClient();

  beforeEach(() => {
    (useAssetModule.useAsset as any).mockReturnValue({ data: mockAsset, isLoading: false, error: null });
    (docsModule.useAssetDocuments as any).mockReturnValue({ data: mockDocs, isLoading: false, error: null });
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
    (useAssetModule.useAsset as any).mockReturnValue({ data: undefined, isLoading: false, error: { status: 404 } });
    (docsModule.useAssetDocuments as any).mockReturnValue({ data: undefined, isLoading: false, error: null });
    
    renderPage('unknown-id');
    expect(screen.getByText('Asset not found')).toBeInTheDocument();
  });
});
