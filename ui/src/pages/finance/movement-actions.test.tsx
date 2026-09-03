/** @vitest-environment jsdom */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MovementActions } from './movement-actions';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as hooks from '../../lib/api/hooks';
import axe from 'axe-core';

vi.mock('../../lib/api/hooks', () => ({
  usePatchDescription: vi.fn(),
  useDeleteMovement: vi.fn(),
}));

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
});

const manualMovement = {
  id: 'mv1',
  kind: 'expense',
  amount: '50.00',
  currency: 'INR',
  occurred_on: '2026-09-01',
  description: 'Grocery',
  origin: 'manual',
  source_account_id: 'acc1',
} as any;

const importedMovement = {
  ...manualMovement,
  id: 'mv2',
  origin: 'import',
  description: 'Imported fee',
} as any;

function renderActions(movement = manualMovement) {
  return render(
    <QueryClientProvider client={queryClient}>
      <MovementActions movement={movement} />
    </QueryClientProvider>,
  );
}

describe('MovementActions', () => {
  const mockPatch = vi.fn();
  const mockDelete = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(hooks.usePatchDescription).mockReturnValue({ mutateAsync: mockPatch, isPending: false } as any);
    vi.mocked(hooks.useDeleteMovement).mockReturnValue({ mutateAsync: mockDelete, isPending: false } as any);
  });

  it('renders edit button for all movements', () => {
    const { rerender } = renderActions(manualMovement);
    expect(screen.getByRole('button', { name: /edit/i })).toBeInTheDocument();

    rerender(
      <QueryClientProvider client={queryClient}>
        <MovementActions movement={importedMovement} />
      </QueryClientProvider>
    );
    expect(screen.getByRole('button', { name: /edit/i })).toBeInTheDocument();
  });

  it('renders delete button only for manual movements', () => {
    const { rerender } = renderActions(manualMovement);
    expect(screen.getByRole('button', { name: /delete/i })).toBeInTheDocument();

    rerender(
      <QueryClientProvider client={queryClient}>
        <MovementActions movement={importedMovement} />
      </QueryClientProvider>
    );
    expect(screen.queryByRole('button', { name: /delete/i })).not.toBeInTheDocument();
  });

  it('patches description when edit dialog is submitted', async () => {
    renderActions();
    
    fireEvent.click(screen.getByRole('button', { name: /edit/i }));
    
    const input = screen.getByLabelText(/description field/i);
    fireEvent.change(input, { target: { value: 'New Description' } });
    
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      expect(mockPatch).toHaveBeenCalledWith({
        movementId: manualMovement.id,
        description: 'New Description',
      });
    });
  });

  it('rejects blank description client-side', async () => {
    renderActions();
    
    fireEvent.click(screen.getByRole('button', { name: /edit/i }));
    
    const input = screen.getByLabelText(/description field/i);
    fireEvent.change(input, { target: { value: '   ' } });
    
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    expect(mockPatch).not.toHaveBeenCalled();
    expect(screen.getByText(/description cannot be blank/i)).toBeInTheDocument();
  });

  it('deletes movement when delete dialog is confirmed', async () => {
    renderActions();
    
    fireEvent.click(screen.getByRole('button', { name: /delete/i }));
    
    expect(screen.getByText(/are you sure/i)).toBeInTheDocument();
    
    // ConfirmDialog confirm button has name "Delete" from props
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(mockDelete).toHaveBeenCalledWith({
        movementId: manualMovement.id,
      });
    });
  });

  it('is axe-clean', async () => {
    const { container } = renderActions();
    const results = await axe.run(container);
    expect(results.violations).toEqual([]);
  });
});
