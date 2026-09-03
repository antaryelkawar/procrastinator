import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ImportHistoryPage } from './import-history-page';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import * as hooks from '../../lib/api/hooks';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

vi.mock('../../lib/api/hooks', () => ({
  useBatches: vi.fn(),
  useBatch: vi.fn(),
  useCommitBatch: vi.fn(),
  useDiscardBatch: vi.fn(),
  batchKey: 'batch',
}));

describe('ImportHistoryPage', () => {
  const queryClient = new QueryClient();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(hooks.useBatch).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);
    vi.mocked(hooks.useCommitBatch).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    } as any);
    vi.mocked(hooks.useDiscardBatch).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    } as any);
  });

  function renderWithRouter(ui: React.ReactElement) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>{ui}</MemoryRouter>
      </QueryClientProvider>
    );
  }

  test('renders empty state when no batches', () => {
    vi.mocked(hooks.useBatches).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    } as any);

    renderWithRouter(<ImportHistoryPage />);
    expect(screen.getByText('Import History')).toBeDefined();
    expect(screen.getByText(/no import history/i)).toBeDefined();
  });

  test('renders list of batches', () => {
    vi.mocked(hooks.useBatches).mockReturnValue({
      data: [{
        id: 'b1',
        filename: 'test.csv',
        account_id: 'acc1',
        state: 'preview',
        line_count_valid: 1,
        line_count_duplicate: 0,
        line_count_error: 0,
        created_at: '2026-09-03T10:00:00Z',
        lines: null
      }],
      isLoading: false,
      isError: false,
    } as any);

    renderWithRouter(<ImportHistoryPage />);
    expect(screen.getByText('Import History')).toBeDefined();
    expect(screen.getByText('test.csv')).toBeDefined();
    expect(screen.getByText('preview')).toBeDefined();
  });
});
