import { render, screen, fireEvent } from '@testing-library/react';
import { AccountsPage } from './accounts-page';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { describe, it, expect, vi, beforeAll } from 'vitest';
import { ActiveUserProvider } from '@/context/active-user';

// Mock Radix scrollIntoView issue
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

// Mock the hooks
vi.mock('@/features/finance/hooks', () => ({
  useCreateAccount: vi.fn(),
}));

vi.mock('@/features/docs/hooks', () => ({
  useAccounts: vi.fn(),
}));

// Stub the composer flow for the page test (real flow is covered in its own
// spec). The closed flow renders nothing visible, so the list test is isolated.
vi.mock('./account-composer-flow', () => ({
  AccountComposerFlow: ({ onOpenChange, open }: { onOpenChange: (o: boolean) => void; open: boolean }) => (
    <button
      type="button"
      aria-label="composer-open-indicator"
      data-open={open}
      onClick={() => onOpenChange(true)}
    />
  ),
}));

import { useAccounts } from '@/features/docs/hooks';
import { useCreateAccount } from '@/features/finance/hooks';

const Wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={queryClient}>
    <ActiveUserProvider queryClient={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
    </ActiveUserProvider>
  </QueryClientProvider>
);

describe('AccountsPage', () => {
  it('renders account list with balance', async () => {
    (useAccounts as any).mockReturnValue({
      data: [{ id: 'acc1', data: { name: 'Main Bank', account_type: 'bank', currency: 'USD', balance: '150' }, created_at: '', updated_at: '' }],
      isLoading: false,
    });
    (useCreateAccount as any).mockReturnValue({ mutate: vi.fn() });

    render(<AccountsPage />, { wrapper: Wrapper });
    expect(await screen.findByText('Main Bank (bank)')).toBeDefined();
    expect(await screen.findByText('$150')).toBeDefined();
  });

  it('wires the Add account button to the composer', async () => {
    (useAccounts as any).mockReturnValue({ data: [], isLoading: false });
    (useCreateAccount as any).mockReturnValue({ mutate: vi.fn() });

    render(<AccountsPage />, { wrapper: Wrapper });

    // The AddButton renders with accessible name "Add account".
    expect(await screen.findByLabelText('Add account')).toBeDefined();
    expect(screen.getByLabelText('Add account')).toHaveAttribute('aria-label', 'Add account');

    // Clicking it opens the (stubbed) composer.
    fireEvent.click(screen.getByLabelText('Add account'));
    expect(screen.getByLabelText('composer-open-indicator')).toHaveAttribute('data-open', 'true');
  });
});
