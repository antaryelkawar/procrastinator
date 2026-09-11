import { describe, test, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ImportPage } from './import-page';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider } from '@/context/active-user';

// Mocking hooks or providing a real QueryClient would be better,
// but for this task I will mock the hooks.
vi.mock('@/features/finance/hooks', () => ({
  useUploadStatement: () => ({
    mutateAsync: vi.fn().mockResolvedValue({
      id: 'batch-1',
      filename: 'test.csv',
      line_count_valid: 8,
      line_count_duplicate: 1,
      line_count_error: 1,
      lines: [
        { line_ref: 1, status: 'valid', occurred_on: '2026-09-01', amount: '100', direction: 'in', description: 'test' },
        // ... (truncated for brevity)
      ]
    }),
    isPending: false,
    data: null,
  }),
  useCommitBatch: () => ({ mutate: vi.fn(), isSuccess: false }),
  useDiscardBatch: () => ({ mutate: vi.fn(), isSuccess: false }),
}));

vi.mock('@/features/docs/hooks', () => ({
  useAccounts: () => ({ data: [{ id: '1', name: 'Bank' }], isLoading: false }),
}));

describe('ImportPage', () => {
  test('renders upload form', () => {
    const queryClient = new QueryClient();
    render(
      <QueryClientProvider client={queryClient}>
        <ActiveUserProvider queryClient={queryClient}>
          <ImportPage />
        </ActiveUserProvider>
      </QueryClientProvider>
    );
    expect(screen.getByLabelText(/Account/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/CSV\/PDF File/i)).toBeInTheDocument();
  });
});
