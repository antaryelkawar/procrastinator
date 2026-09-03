import { render, screen, fireEvent, within } from '@testing-library/react';
import { AccountsPage } from './accounts-page';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeAll } from 'vitest';
import { ActiveUserProvider } from '../../context/active-user';

// Mock Radix scrollIntoView issue
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

// Mock the hooks
vi.mock('../../lib/api/hooks', async () => {
  const actual = await vi.importActual<any>('../../lib/api/hooks');
  return {
    ...actual,
    useAccounts: vi.fn(),
    useCreateAccount: vi.fn(),
  };
});

import { useAccounts, useCreateAccount } from '../../lib/api/hooks';

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
      data: [{ id: 'acc1', name: 'Main Bank', type: 'bank', currency: 'USD', balance: '150', created_at: '', updated_at: '' }],
      isLoading: false,
    });
    (useCreateAccount as any).mockReturnValue({ mutate: vi.fn() });

    render(<AccountsPage />, { wrapper: Wrapper });
    expect(await screen.findByText('Main Bank (bank)')).toBeDefined();
    expect(await screen.findByText('$150')).toBeDefined();
  });

  it('rejects empty form submission', async () => {
    (useAccounts as any).mockReturnValue({ data: [], isLoading: false });
    (useCreateAccount as any).mockReturnValue({ mutate: vi.fn() });
    
    // Change to getByRole("button", { name: "Create Account" })
    render(<AccountsPage />, { wrapper: Wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Create Account' }));
    expect(await screen.findByText('Name, Type, and Currency are required')).toBeDefined();
  });

  it('shows error on server 400', async () => {
    (useAccounts as any).mockReturnValue({ data: [], isLoading: false });
    const mutate = vi.fn((_vars, { onError }) => onError({ message: 'Validation failed' }));
    (useCreateAccount as any).mockReturnValue({ mutate, isPending: false });
    
    render(<AccountsPage />, { wrapper: Wrapper });
    // Fill all fields to pass client-side validation
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Bad Account' } });
    fireEvent.change(screen.getByLabelText('Currency'), { target: { value: 'USD' } });
    // Type is a select, fireEvent.click on the trigger and selecting might be complex
    // Let's just bypass by testing the error callback directly in a new test or using a simplified approach
    // Actually, I can just fire the mutation manually if I had access, but I don't.
    // Type is a select, use within to avoid conflict
    fireEvent.click(screen.getByLabelText('Type'));
    const listbox = screen.getByRole('listbox');
    fireEvent.click(within(listbox).getByText('Bank'));
    
    fireEvent.click(screen.getByRole('button', { name: 'Create Account' }));
    
    expect(await screen.findByTestId('error-message')).toHaveTextContent('Validation failed');
  });
});
