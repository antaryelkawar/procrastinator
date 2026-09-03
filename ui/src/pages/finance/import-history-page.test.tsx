import { render, screen } from '@testing-library/react';
import { ImportHistoryPage } from './import-history-page';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import * as hooks from '../../lib/api/hooks';

vi.mock('../../lib/api/hooks', () => ({
  useBatches: vi.fn(),
  useBatch: vi.fn(),
  useCommitBatch: vi.fn(),
  useDiscardBatch: vi.fn(),
}));

describe('ImportHistoryPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(hooks.useBatch).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
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

  test('renders empty state when no batches', () => {
    vi.mocked(hooks.useBatches).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    } as any);

    render(<ImportHistoryPage />);
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

    render(<ImportHistoryPage />);
    expect(screen.getByText('test.csv')).toBeDefined();
    expect(screen.getByText('preview')).toBeDefined();
  });
});
