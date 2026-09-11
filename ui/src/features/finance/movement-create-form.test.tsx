/** @vitest-environment jsdom */
/**
 * Movement creation form (task 4.6).
 *
 * Strategy: integration-style tests run the REAL `useAccounts`/
 * `useCreateMovement` hooks against the MSW server, so the "no request before
 * validation" and "post-success invalidation" assertions observe actual
 * network traffic and actual TanStack Query invalidation — not mocks of the
 * very behavior under test. The pure `buildMovementRequest` validator is also
 * unit-tested directly, covering every rule (incl. cross-currency transfer)
 * per design D7.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { MovementCreateForm, buildMovementRequest } from './movement-create-form';
import { ActiveUserProvider } from '@/context/active-user';
import { server } from '@/mocks/server';
import axe from 'axe-core';
import type { Account } from '@/lib/api/schema';

const USER_ID = 'test-user';

const inrAccount: Account = {
  id: 'acc-inr',
  name: 'Savings INR',
  type: 'bank',
  currency: 'INR',
  balance: '1000',
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
};

const eurAccount: Account = {
  id: 'acc-eur',
  name: 'Savings EUR',
  type: 'bank',
  currency: 'EUR',
  balance: '500',
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
};

const accounts = [inrAccount, eurAccount];

// ---------------------------------------------------------------------------
// Render helpers (real hooks + MSW; active user comes from localStorage, as
// the provider reads it on mount)
// ---------------------------------------------------------------------------

let queryClient: QueryClient;

function renderForm() {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ActiveUserProvider queryClient={queryClient}>
        <MovementCreateForm />
      </ActiveUserProvider>
    </QueryClientProvider>,
  );
}

function useAccountsFixture(list: Account[]): void {
  server.use(http.get('/api/users/:userId/finance/accounts', () => HttpResponse.json(list)));
}

/** Wait until the account list has loaded (the form swaps Loading for the fields). */
async function waitForForm(): Promise<void> {
  await screen.findByLabelText('Kind');
}

/** Register a spy on POST /api/users/:userId/finance/movements; returns the captured bodies. */
function trackMovementsPost(): unknown[] {
  const bodies: unknown[] = [];
  server.use(
    http.post('/api/users/:userId/finance/movements', async ({ request }) => {
      bodies.push((await request.json()) as unknown);
      return HttpResponse.json(
        {
          id: 'mv-new',
          kind: 'expense',
          amount: '1250.50',
          currency: 'INR',
          occurred_on: '2026-09-01',
          recorded_at: '2026-09-03T00:00:00Z',
          description: 'created',
          origin: 'manual',
          source_account_id: 'acc-inr',
          link_conflicting: false,
          created_at: '2026-09-03T00:00:00Z',
          updated_at: '2026-09-03T00:00:00Z',
        },
        { status: 201 },
      );
    }),
  );
  return bodies;
}

function fillForm(fields: { amount?: string; description?: string; occurredOn?: string } = {}): void {
  const { amount = '1250.50', description = 'Grocery run', occurredOn = '2026-09-01' } = fields;
  fireEvent.change(screen.getByLabelText('Amount'), { target: { value: amount } });
  fireEvent.change(screen.getByLabelText('Occurred on'), { target: { value: occurredOn } });
  fireEvent.change(screen.getByLabelText('Description'), { target: { value: description } });
}

const KIND_LABELS: Record<string, string> = { expense: 'Expense', income: 'Income', transfer: 'Transfer' };

/** Open a shared Radix Select (by its trigger aria-label) and pick an option by accessible text. */
function pickOption(triggerLabel: string, optionName: string): void {
  fireEvent.click(screen.getByLabelText(triggerLabel));
  fireEvent.click(screen.getByRole('option', { name: optionName }));
}

function selectKind(kind: string): void {
  pickOption('Kind', KIND_LABELS[kind] ?? kind);
}

function selectSource(accountId: string): void {
  const account = accounts.find((a) => a.id === accountId);
  if (!account) throw new Error(`unknown account in selectSource: ${accountId}`);
  pickOption('Source account', `${account.name} (${account.currency})`);
}

function selectDestination(accountId: string): void {
  const account = accounts.find((a) => a.id === accountId);
  if (!account) throw new Error(`unknown account in selectDestination: ${accountId}`);
  pickOption('Destination account', `${account.name} (${account.currency})`);
}

function submit(): void {
  fireEvent.click(screen.getByRole('button', { name: 'Create movement' }));
}

async function expectFormError(text: string): Promise<void> {
  await waitFor(() => expect(screen.getByTestId('movement-form-error')).toHaveTextContent(text));
}

// ---------------------------------------------------------------------------
// Unit: buildMovementRequest (D7 — every rule directly testable, pure)
// ---------------------------------------------------------------------------

const validExpense = {
  kind: 'expense' as const,
  amount: '1250.50',
  occurredOn: '2026-09-01',
  description: 'Grocery run',
  sourceAccountId: inrAccount.id,
  destinationAccountId: '',
};

describe('buildMovementRequest (pure validation)', () => {
  it('rejects non-positive or malformed amounts, string-only', () => {
    for (const amount of ['', '0', '0.0', '0.000', '-1', '-0.01', '1.2.3', '1,000', '.5', '5.', 'abc', '12 50']) {
      const result = buildMovementRequest({ ...validExpense, amount }, accounts);
      expect(result).toEqual({ ok: false, error: 'Amount must be a positive number.' });
    }
  });

  it('accepts a positive decimal amount', () => {
    const result = buildMovementRequest({ ...validExpense, amount: '0.01' }, accounts);
    expect(result.ok).toBe(true);
  });

  it('rejects a missing occurred date', () => {
    const result = buildMovementRequest({ ...validExpense, occurredOn: '' }, accounts);
    expect(result).toEqual({ ok: false, error: 'Pick the date the movement occurred.' });
  });

  it('rejects a blank or whitespace-only description', () => {
    for (const description of ['', '   ']) {
      const result = buildMovementRequest({ ...validExpense, description }, accounts);
      expect(result).toEqual({ ok: false, error: 'Description cannot be empty.' });
    }
  });

  it('rejects an expense with a missing source account', () => {
    const result = buildMovementRequest({ ...validExpense, sourceAccountId: '' }, accounts);
    expect(result).toEqual({ ok: false, error: 'Pick the account the expense leaves.' });
  });

  it('rejects an income with a missing destination account', () => {
    const result = buildMovementRequest(
      { kind: 'income', amount: '1', occurredOn: '2026-09-01', description: 'x', sourceAccountId: '', destinationAccountId: '' },
      accounts,
    );
    expect(result).toEqual({ ok: false, error: 'Pick the account the income lands in.' });
  });

  it('rejects a transfer with a missing source or destination account', () => {
    const transfer = { kind: 'transfer' as const, amount: '1', occurredOn: '2026-09-01', description: 'x' };
    expect(buildMovementRequest({ ...transfer, sourceAccountId: '', destinationAccountId: inrAccount.id }, accounts)).toEqual({
      ok: false,
      error: 'Pick the account the transfer leaves.',
    });
    expect(buildMovementRequest({ ...transfer, sourceAccountId: inrAccount.id, destinationAccountId: '' }, accounts)).toEqual({
      ok: false,
      error: 'Pick the account the transfer lands in.',
    });
  });

  it('rejects a transfer whose source and destination are the same account', () => {
    const result = buildMovementRequest(
      { kind: 'transfer', amount: '1', occurredOn: '2026-09-01', description: 'x', sourceAccountId: inrAccount.id, destinationAccountId: inrAccount.id },
      accounts,
    );
    expect(result).toEqual({ ok: false, error: 'A transfer needs two different accounts.' });
  });

  it('rejects a cross-currency transfer', () => {
    const result = buildMovementRequest(
      { kind: 'transfer', amount: '1', occurredOn: '2026-09-01', description: 'x', sourceAccountId: inrAccount.id, destinationAccountId: eurAccount.id },
      accounts,
    );
    expect(result).toEqual({ ok: false, error: 'A transfer needs both accounts in the same currency.' });
  });

  it('builds an expense request with the source account currency', () => {
    const result = buildMovementRequest({ ...validExpense, description: '  Grocery run  ' }, accounts);
    expect(result).toEqual({
      ok: true,
      request: {
        kind: 'expense',
        amount: '1250.50',
        currency: 'INR',
        occurred_on: '2026-09-01',
        description: 'Grocery run',
        source_account_id: inrAccount.id,
      },
    });
  });

  it('builds an income request with the destination account currency', () => {
    const result = buildMovementRequest(
      { kind: 'income', amount: '250', occurredOn: '2026-09-01', description: 'Salary', sourceAccountId: '', destinationAccountId: eurAccount.id },
      accounts,
    );
    expect(result).toEqual({
      ok: true,
      request: {
        kind: 'income',
        amount: '250',
        currency: 'EUR',
        occurred_on: '2026-09-01',
        description: 'Salary',
        destination_account_id: eurAccount.id,
      },
    });
  });

  it('builds a transfer request with both accounts of one currency', () => {
    const secondInr: Account = { ...inrAccount, id: 'acc-inr-2', name: 'Current INR' };
    const result = buildMovementRequest(
      { kind: 'transfer', amount: '100', occurredOn: '2026-09-01', description: 'Move cash', sourceAccountId: inrAccount.id, destinationAccountId: 'acc-inr-2' },
      [inrAccount, secondInr],
    );
    expect(result).toEqual({
      ok: true,
      request: {
        kind: 'transfer',
        amount: '100',
        currency: 'INR',
        occurred_on: '2026-09-01',
        description: 'Move cash',
        source_account_id: inrAccount.id,
        destination_account_id: 'acc-inr-2',
      },
    });
  });
});

// ---------------------------------------------------------------------------
// Integration: rendered form against MSW (real hooks, real network)
// ---------------------------------------------------------------------------

describe('MovementCreateForm (rendered)', () => {
  beforeEach(() => {
    localStorage.setItem('activeUser', USER_ID);
  });

  it('renders the loading state while accounts are in flight, then the form', async () => {
    server.use(
      http.get('/api/finance/accounts', () =>
        new Promise((resolve) => setTimeout(() => resolve(HttpResponse.json(accounts)), 50)),
      ),
    );
    renderForm();

    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.queryByLabelText('Kind')).not.toBeInTheDocument();

    await waitForForm();
    expect(screen.getByLabelText('Amount')).toBeInTheDocument();
    expect(screen.getByLabelText('Occurred on')).toBeInTheDocument();
    expect(screen.getByLabelText('Description')).toBeInTheDocument();
  });

  it('shows a source account for an expense, a destination for income, both for transfer', async () => {
    useAccountsFixture(accounts);
    renderForm();
    await waitForForm();

    // default kind: expense
    expect(screen.getByLabelText('Source account')).toBeInTheDocument();
    expect(screen.queryByLabelText('Destination account')).not.toBeInTheDocument();

    selectKind('income');
    expect(screen.queryByLabelText('Source account')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Destination account')).toBeInTheDocument();

    selectKind('transfer');
    expect(screen.getByLabelText('Source account')).toBeInTheDocument();
    expect(screen.getByLabelText('Destination account')).toBeInTheDocument();
  });

  it('rejects a non-positive amount before any request', async () => {
    useAccountsFixture(accounts);
    const bodies = trackMovementsPost();
    renderForm();
    await waitForForm();

    fillForm({ amount: '0' });
    selectSource(inrAccount.id);
    submit();

    await expectFormError('Amount must be a positive number.');
    expect(bodies).toHaveLength(0);
  });

  it('rejects a blank description before any request', async () => {
    useAccountsFixture(accounts);
    const bodies = trackMovementsPost();
    renderForm();
    await waitForForm();

    fillForm({ description: '   ' });
    selectSource(inrAccount.id);
    submit();

    await expectFormError('Description cannot be empty.');
    expect(bodies).toHaveLength(0);
  });

  it('rejects a missing account before any request', async () => {
    useAccountsFixture(accounts);
    const bodies = trackMovementsPost();
    renderForm();
    await waitForForm();

    fillForm();
    // no account selected
    submit();

    await expectFormError('Pick the account the expense leaves.');
    expect(bodies).toHaveLength(0);
  });

  it('rejects identical transfer accounts before any request', async () => {
    useAccountsFixture(accounts);
    const bodies = trackMovementsPost();
    renderForm();
    await waitForForm();

    selectKind('transfer');
    fillForm();
    selectSource(inrAccount.id);
    selectDestination(inrAccount.id);
    submit();

    await expectFormError('A transfer needs two different accounts.');
    expect(bodies).toHaveLength(0);
  });

  it('rejects a cross-currency transfer before any request', async () => {
    useAccountsFixture(accounts);
    const bodies = trackMovementsPost();
    renderForm();
    await waitForForm();

    selectKind('transfer');
    fillForm();
    selectSource(inrAccount.id);
    selectDestination(eurAccount.id);
    submit();

    await expectFormError('A transfer needs both accounts in the same currency.');
    expect(bodies).toHaveLength(0);
  });

  it('creates a valid expense, resets the form, and invalidates movements + accounts', async () => {
    useAccountsFixture(accounts);
    const bodies = trackMovementsPost();
    renderForm();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    await waitForForm();

    fillForm({ amount: '1250.50', description: 'Grocery run', occurredOn: '2026-09-01' });
    selectSource(inrAccount.id);
    submit();

    // exactly one request, with the exact wire body
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toEqual({
      kind: 'expense',
      amount: '1250.50',
      currency: 'INR',
      occurred_on: '2026-09-01',
      description: 'Grocery run',
      source_account_id: inrAccount.id,
    });

    // form reset to its initial values (amount/description/accounts cleared)
    await waitFor(() => expect(screen.getByLabelText('Amount')).toHaveValue(''));
    expect(screen.getByLabelText('Description')).toHaveValue('');
    // the source account Select is unselected → its placeholder text is shown
    expect(screen.getByText('Select an account')).toBeInTheDocument();
    expect(screen.queryByTestId('movement-form-error')).not.toBeInTheDocument();

    // D5 invalidation: movements + accounts for the active user
    await waitFor(() => {
      expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ['movements', USER_ID] }));
      expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ['accounts', USER_ID] }));
    });
  });

  it('keeps entered values when the server rejects the request', async () => {
    useAccountsFixture(accounts);
    server.use(
      http.post('/api/users/:userId/finance/movements', () =>
        HttpResponse.json({ error: 'Balance would go negative.' }, { status: 400 }),
      ),
    );
    renderForm();
    await waitForForm();

    fillForm({ amount: '999999', description: 'Too big' });
    selectSource(inrAccount.id);
    submit();

    // mapped 400 copy surfaces as the form error…
    await expectFormError('That request was invalid. Check the details and try again.');
    // …and the user's values are retained (D7)
    expect(screen.getByLabelText('Amount')).toHaveValue('999999');
    expect(screen.getByLabelText('Description')).toHaveValue('Too big');
    // the selected source account is shown in the Select trigger (the combobox
    // itself, not the open option list)
    const trigger = screen.getByLabelText('Source account');
    expect(trigger).toHaveTextContent(`${inrAccount.name} (${inrAccount.currency})`);
  });

  it('is axe-clean (full transfer form incl. the error region)', async () => {
    useAccountsFixture(accounts);
    const { container } = renderForm();
    await waitForForm();

    // exercise the error region + both transfer selects so axe sees the full form
    selectKind('transfer');
    fillForm({ amount: '0' });
    selectSource(inrAccount.id);
    selectDestination(inrAccount.id);
    submit();
    await expectFormError('Amount must be a positive number.');

    const results = await axe.run(container);
    expect(results.violations).toEqual([]);
  });
});
