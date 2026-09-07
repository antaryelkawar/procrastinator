/** @vitest-environment jsdom */
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MovementsPage } from './movements-page';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as hooks from '../../lib/api/hooks';
import axe from 'axe-core';

vi.mock('../../lib/api/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/api/hooks')>();
  return {
    ...actual,
    useMovements: vi.fn(),
    useAccounts: vi.fn(),
    usePatchDescription: vi.fn(),
    useDeleteMovement: vi.fn(),
  };
});

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
});

function renderPage(initialPath = '/finance/movements') {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <MovementsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const expenseMovement = {
  id: 'mv1',
  kind: 'expense',
  amount: '50.00',
  currency: 'INR',
  occurred_on: '2026-09-01',
  description: 'Grocery',
  origin: 'manual',
  source_account_id: 'acc1',
  link_conflicting: false,
  recorded_at: '2026-09-01T00:00:00Z',
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
} as const;

const importedMovement = {
  ...expenseMovement,
  id: 'mv2',
  origin: 'import',
  description: 'Imported fee',
} as const;

beforeEach(() => {
  vi.mocked(hooks.useAccounts).mockReturnValue({ data: [], isLoading: false } as any);
  vi.mocked(hooks.usePatchDescription).mockReturnValue({ isPending: false } as any);
  vi.mocked(hooks.useDeleteMovement).mockReturnValue({ isPending: false } as any);
});

describe('MovementsPage', () => {
  it('renders movements with signed amount per kind, occurred date verbatim, description, and origin badge', async () => {
    vi.mocked(hooks.useMovements).mockReturnValue({ data: [expenseMovement], isLoading: false } as any);
    renderPage();

    expect(await screen.findByText('Grocery')).toBeInTheDocument();
    // D2: expense → negative sign (U+2212) + INR symbol, string-exact
    expect(screen.getByText('\u2212\u20B950.00')).toBeInTheDocument();
    // occurred_on rendered verbatim (string surgery, no TZ shift)
    expect(screen.getByText('2026-09-01')).toBeInTheDocument();
    // manual origin badge
    expect(screen.getByText('manual')).toBeInTheDocument();
    // source account shown for an expense
    expect(screen.getByText('acc1')).toBeInTheDocument();
  });

  it('shows no delete affordance for imported movements', async () => {
    vi.mocked(hooks.useMovements).mockReturnValue({ data: [importedMovement], isLoading: false } as any);
    renderPage();

    expect(await screen.findByText('Imported fee')).toBeInTheDocument();
    expect(screen.getByText('import')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /delete/i })).not.toBeInTheDocument();
  });

  it('sends account_id/from/to to the client when filters are applied', async () => {
    vi.mocked(hooks.useAccounts).mockReturnValue({ data: [{ id: 'acc1', name: 'Main', type: 'bank', currency: 'INR', balance: '0' }], isLoading: false } as any);
    vi.mocked(hooks.useMovements).mockReturnValue({ data: [], isLoading: false } as any);
    renderPage();

    // account filter — shared Radix Select: open the trigger (click), then
    // pick the option by its accessible text.
    fireEvent.click(screen.getByLabelText('Account'));
    fireEvent.click(await screen.findByRole('option', { name: 'Main' }));
    await waitFor(() => {
      expect(hooks.useMovements).toHaveBeenCalledWith(
        expect.objectContaining({ accountId: 'acc1' }),
      );
    });

    // date-range filters — type into the date inputs. The create-movement form
    // above also has an "Occurred on" (→ "to") label, so scope by id.
    const fromInput = document.getElementById('movements-date-from') as HTMLInputElement;
    const toInput = document.getElementById('movements-date-to') as HTMLInputElement;
    fireEvent.change(fromInput, { target: { value: '2026-08-01' } });
    fireEvent.change(toInput, { target: { value: '2026-08-31' } });
    await waitFor(() => {
      expect(hooks.useMovements).toHaveBeenCalledWith(
        expect.objectContaining({ from: '2026-08-01', to: '2026-08-31' }),
      );
    });
  });

  it('is axe-clean', async () => {
    vi.mocked(hooks.useMovements).mockReturnValue({ data: [expenseMovement], isLoading: false } as any);
    const { container } = renderPage();
    const results = await axe.run(container);
    expect(results.violations).toEqual([]);
  });
});
