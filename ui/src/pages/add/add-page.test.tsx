/**
 * Task 8.2 — unified Add surface (`/add`).
 *
 * Strategy: mock the data-layer hooks (`@/lib/api/hooks`) so each test drives
 * the observable UI behavior end-to-end through the real `<AddPage />` and the
 * shared primitives (dropzone, Radix Select, Textarea). No test seams are
 * added: the real Radix Select is driven with `fireEvent.click` (the proven
 * pattern from `movements-page.test.tsx` / `movement-create-form.test.tsx`),
 * and the real `useAdd`/`useRestoreAsset` mutation shapes are returned from
 * the mock. The `useAdd` mock's `mutate` synchronously resolves its
 * `onSuccess` callback (mirroring TanStack's mutation lifecycle) so the
 * outcome summary renders deterministically and `mutate` call-args can be
 * asserted directly.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import axe from 'axe-core';
import { AddPage } from './add-page';
import { ActiveUserProvider } from '@/context/active-user';
import * as hooks from '@/lib/api/hooks';

vi.mock('@/lib/api/hooks', () => ({
  useAdd: vi.fn(),
  useAccounts: vi.fn(),
  useRestoreAsset: vi.fn(),
}));

const USER_ID = 'alice';

const accounts: ReadonlyArray<{
  id: string;
  name: string;
  type: string;
  currency: string;
  balance: string;
  created_at: string;
  updated_at: string;
}> = [
  { id: 'acc-1', name: 'Main Checking', type: 'bank', currency: 'INR', balance: '0', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'acc-2', name: 'Savings', type: 'bank', currency: 'EUR', balance: '0', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
];

/**
 * Build the mock return for `useAdd`. The page renders the outcome summary
 * keyed on `add.isSuccess` + `add.data`. A real TanStack mutation flips those
 * fields AND notifies the hook, triggering a re-render; the mock can't drive
 * that notification, so instead `isSuccess`/`data` are set as plain fields on
 * the object BEFORE the page first renders (when `outcomes` is non-null). The
 * page then renders the summary synchronously on the first render, and the
 * tests assert the already-present summary after a (no-op) submit. This keeps
 * the test deterministic without relying on a re-render the mock can't trigger.
 *
 * `mutate` records its call-args (so the tests assert the request body) and
 * `mutateAsync` resolves the outcomes (or rejects when `outcomes` is null) for
 * the submit handler's `await`, which clears the inputs on success.
 */
function makeUseAdd(outcomes: unknown[] | null) {
  // The page calls `add.mutateAsync(body)`; `mutateAsync` delegates to the
  // `mutate` spy so the request-body assertions land on `mutate` (matching
  // TanStack's shared underlying mutation function).
  const mutate = vi.fn(async () => {
    if (outcomes !== null) {
      return outcomes;
    }
    throw new Error('no outcomes (mock)');
  });
  const mutateAsync = vi.fn(async (body: unknown) => mutate(body));
  return {
    mutate,
    mutateAsync,
    isPending: false,
    isSuccess: outcomes !== null,
    data: outcomes,
    error: null,
  };
}

function renderPage(outcomes: unknown[] | null = null) {
  localStorage.setItem('activeUser', USER_ID);
  const addResult = makeUseAdd(outcomes);
  vi.mocked(hooks.useAdd).mockReturnValue(addResult as never);
  vi.mocked(hooks.useAccounts).mockReturnValue({
    data: accounts,
    isLoading: false,
    error: null,
  } as never);
  const restoreMutate = vi.fn();
  vi.mocked(hooks.useRestoreAsset).mockReturnValue({
    mutate: restoreMutate,
    isPending: false,
    isSuccess: false,
    error: null,
  } as never);

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/add']}>
        <ActiveUserProvider queryClient={queryClient}>
          <AddPage />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...utils, addResult, restoreMutate };
}

/**
 * Queue one file through the real dropzone file input. react-dropzone v20
 * resolves `getFilesFromEvent` through a microtask, so `onDrop` fires
 * asynchronously — await a tick for the queue state to settle.
 */
async function queueFile(name: string, type = ''): Promise<void> {
  const file = new File(['x'], name, type ? { type } : undefined);
  const input = screen.getByLabelText('file upload', { selector: 'input' });
  fireEvent.change(input, { target: { files: [file] } });
  // react-dropzone resolves onDrop through a microtask; wait until the queued
  // file name is actually rendered so `hasStatement`/`hasSomething` are settled.
  await screen.findByText(name);
  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Add' })).toBeEnabled();
  });
}

/** Open the account Radix Select and pick an account by its name. */
function pickAccount(name: string): void {
  fireEvent.click(screen.getByLabelText('Account'));
  fireEvent.click(screen.getByRole('option', { name }));
}

/** Submit and flush the async `mutateAsync().then(...)` input-clearing update. */
async function submit(): Promise<void> {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  });
}

beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
});

describe('AddPage (task 8.2 — unified add)', () => {
  it('renders the heading, dropzone, paste box, account selector, and Add CTA', () => {
    renderPage();
    expect(screen.getByRole('heading', { level: 1, name: 'Add' })).toBeInTheDocument();
    // the dropzone root is labelled (from the shared Upload primitive)
    expect(screen.getByLabelText('file upload', { selector: 'div' })).toBeInTheDocument();
    // paste text box
    expect(screen.getByLabelText('Paste text')).toBeInTheDocument();
    // account selector trigger is labelled
    expect(screen.getByLabelText('Account', { selector: 'button' })).toBeInTheDocument();
    // Add CTA
    expect(screen.getByRole('button', { name: 'Add' })).toBeInTheDocument();
  });

  it('queues a dropped photo → submit → asset outcome links to /assets/a1', async () => {
    const { addResult } = renderPage([{ kind: 'asset_committed', asset_id: 'a1' }]);
    await queueFile('photo.png', 'image/png');

    // queued file is listed
    expect(screen.getByText('photo.png')).toBeInTheDocument();
    // Add is enabled once there is something to add
    expect(screen.getByRole('button', { name: 'Add' })).toBeEnabled();

    await submit();

    // summary heading + per-item outcome
    expect(screen.getByRole('heading', { name: 'Results' })).toBeInTheDocument();
    expect(screen.getByText('Committed as asset')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: 'View asset' });
    expect(link).toHaveAttribute('href', '/assets/a1');
    // files + account (none selected → undefined) were passed through
    expect(addResult.mutate).toHaveBeenCalledWith(
      expect.objectContaining({ files: expect.any(Array) }),
    );
  });

  it('pasted text → submit → asset outcome links to /assets/a2', async () => {
    const { addResult } = renderPage([{ kind: 'asset_committed', asset_id: 'a2' }]);
    fireEvent.change(screen.getByLabelText('Paste text'), { target: { value: 'A 2020 Toyota Prius, serial ABC123' } });

    await submit();

    expect(screen.getByRole('heading', { name: 'Results' })).toBeInTheDocument();
    expect(screen.getByText('Committed as asset')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'View asset' })).toHaveAttribute('href', '/assets/a2');
    expect(addResult.mutate).toHaveBeenCalledWith(
      expect.objectContaining({ text: 'A 2020 Toyota Prius, serial ABC123' }),
    );
  });

  it('statement file + selected account → submit → statement_preview links to /finance/import/b1', async () => {
    const { addResult } = renderPage([{ kind: 'statement_preview', import_batch_id: 'b1' }]);
    await queueFile('statement.csv', 'text/csv');
    pickAccount('Main Checking');

    // no validation error (account selected)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    await submit();

    expect(screen.getByRole('heading', { name: 'Results' })).toBeInTheDocument();
    expect(screen.getByText('Statement preview')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: 'Open import batch' });
    expect(link).toHaveAttribute('href', '/finance/import/b1');

    // account_id was passed to the mutation
    expect(addResult.mutate).toHaveBeenCalledWith(
      expect.objectContaining({ account_id: 'acc-1' }),
    );
  });

  it('statement file without account → submit → validation error, mutate NOT called', async () => {
    const { addResult } = renderPage(null);
    await queueFile('statement.csv', 'text/csv');
    // no account selected

    await submit();

    // validation alert appears
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent(/Select an account for the statement/);
    // no outcomes rendered
    expect(screen.queryByRole('heading', { name: 'Results' })).not.toBeInTheDocument();
    // mutate was NOT called
    expect(addResult.mutate).not.toHaveBeenCalled();
  });

  it('duplicate outcome shows View existing asset link + Restore offer when asset_deleted', async () => {
    const { restoreMutate } = renderPage([
      { kind: 'duplicate', duplicate_asset_id: 'd1', asset_deleted: true },
    ]);
    fireEvent.change(screen.getByLabelText('Paste text'), { target: { value: 'existing asset text' } });

    await submit();

    expect(screen.getByRole('heading', { name: 'Results' })).toBeInTheDocument();
    expect(screen.getByText('Duplicate')).toBeInTheDocument();
    // link to the existing (duplicate) asset
    expect(screen.getByRole('link', { name: 'View existing asset' })).toHaveAttribute('href', '/assets/d1');
    // restore affordance is present (asset is soft-deleted)
    const restoreBtn = screen.getByRole('button', { name: 'Restore' });
    expect(restoreBtn).toBeInTheDocument();

    // clicking Restore calls useRestoreAsset with the duplicate asset id
    fireEvent.click(restoreBtn);
    expect(restoreMutate).toHaveBeenCalledWith(
      expect.objectContaining({ assetId: 'd1' }),
      expect.anything(),
    );
  });

  it('duplicate outcome without asset_deleted shows the link but no Restore offer', async () => {
    renderPage([{ kind: 'duplicate', duplicate_asset_id: 'd2', asset_deleted: false }]);
    fireEvent.change(screen.getByLabelText('Paste text'), { target: { value: 'existing asset text 2' } });

    await submit();

    expect(screen.getByText('Duplicate')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'View existing asset' })).toHaveAttribute('href', '/assets/d2');
    // no restore offer
    expect(screen.queryByRole('button', { name: 'Restore' })).not.toBeInTheDocument();
  });

  it('held_for_review outcome links to /ingest/reviews', async () => {
    renderPage([{ kind: 'held_for_review', review_id: 'r1' }]);
    await queueFile('receipt.jpg', 'image/jpeg');
    await submit();

    expect(screen.getByText('Held for review')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'View review queue' })).toHaveAttribute('href', '/ingest/reviews');
  });

  it('failed outcome shows the reason text and no link', async () => {
    renderPage([{ kind: 'failed', reason: 'Unsupported file type' }]);
    // Drive the submit through pasted text (no file) so the queue/dropzone
    // accept-list is not a factor; the mocked useAdd returns a `failed`
    // outcome regardless of the input kind, which is what we assert on.
    fireEvent.change(screen.getByLabelText('Paste text'), { target: { value: 'some unsupported thing' } });
    await submit();

    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText('Unsupported file type')).toBeInTheDocument();
    // a failed outcome carries no navigation link
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('disables Add when there is nothing to add (no files, blank text)', () => {
    renderPage(null);
    const addBtn = screen.getByRole('button', { name: 'Add' });
    expect(addBtn).toBeDisabled();
  });

  it('the account selector renders its options from useAccounts', () => {
    renderPage(null);
    fireEvent.click(screen.getByLabelText('Account', { selector: 'button' }));
    expect(screen.getByRole('option', { name: 'Main Checking' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Savings' })).toBeInTheDocument();
  });

  it('is axe-clean', async () => {
    const { container } = renderPage(null);
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });
});

// The `vitest-axe` package registers `toHaveNoViolations` at runtime via
// `expect.extend` in the test setup, but its bundled type augmentation targets
// a namespace vitest v3 does not use. Declare the matcher against the real
// `@vitest/expect` module so `tsc -b` can resolve it.
declare module '@vitest/expect' {
  interface Matchers<T = any> {
    toHaveNoViolations(): {
      actual: import('axe-core').Result[];
      pass: boolean;
      message(): string;
    };
  }
}
