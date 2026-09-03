/**
 * Tests for the TanStack Query data layer (design D4/D5).
 *
 * The client module and the XHR upload module are mocked at the module
 * boundary (the app is built against mocked API responses — the backend is
 * being reworked in parallel). These tests assert:
 *   1. every query KEY carries the active user id (D4) and maps to the
 *      expected resource request;
 *   2. every mutation invalidates EXACTLY the D5 prefixes — no more, no less —
 *      and nothing is invalidated when a mutation fails;
 *   3. no request goes out while no user is active.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider } from '../../context/active-user';
import * as client from './client';
import * as upload from './upload';
import {
  useAccounts,
  useAsset,
  useAssetDocuments,
  useAssets,
  useBatch,
  useBatches,
  useCommitBatch,
  useCreateAccount,
  useCreateMovement,
  useDeleteMovement,
  useDiscardBatch,
  useLinkMovement,
  useMovements,
  usePatchDescription,
  useUnlinkMovement,
  useUploadDocument,
  useUploadStatement,
} from './hooks';
import type { MovementFilterInput } from './hooks';
import { ApiError } from './errors';
import type {
  Account,
  Asset,
  CommitSummary,
  CreateAccountRequest,
  CreateMovementRequest,
  Document,
  ImportBatch,
  Movement,
} from './types';

vi.mock('./client', () => ({
  apiJson: vi.fn(),
  apiVoid: vi.fn(),
}));

vi.mock('./upload', () => ({
  uploadDocument: vi.fn(),
  uploadStatement: vi.fn(),
}));

const ALICE = 'alice';

// ---------------------------------------------------------------------------
// Fixtures (shapes mirror the wire contract in types.ts)
// ---------------------------------------------------------------------------

const assetFixture: Asset = {
  id: 'a1',
  brand: 'Dell',
  model: 'XPS 13',
  serial_number: 'SN-1',
  purchase_date: '2026-01-15T00:00:00Z',
  warranty_end: '2026-09-01T00:00:00Z',
  price: '39999.99',
  currency: 'INR',
  doc_type: 'invoice',
  metadata: {},
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z',
  scope_type: 'personal',
};

const documentFixture: Document = {
  id: 'd1',
  doc_type: 'invoice',
  source_filename: 'invoice.pdf',
  source_uploaded_at: '2026-08-01T10:00:00Z',
  created_at: '2026-08-01T10:00:00Z',
  scope_type: 'personal',
};

const accountFixture: Account = {
  id: 'acc1',
  name: 'Main',
  type: 'bank',
  currency: 'INR',
  balance: '150',
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z',
};

const movementFixture: Movement = {
  id: 'mv1',
  kind: 'expense',
  amount: '100',
  currency: 'INR',
  occurred_on: '2026-08-20',
  recorded_at: '2026-08-20T09:00:00Z',
  description: 'groceries',
  origin: 'manual',
  source_account_id: 'acc1',
  link_conflicting: false,
  created_at: '2026-08-20T09:00:00Z',
  updated_at: '2026-08-20T09:00:00Z',
};

const batchFixture: ImportBatch = {
  id: 'b1',
  state: 'preview',
  account_id: 'acc1',
  source: {
    id: 's1',
    filename: 'statement.csv',
    content_type: 'text/csv',
    size: 1024,
    sha256: 'abc123',
    uploaded_at: '2026-08-20T09:00:00Z',
  },
  filename: 'statement.csv',
  format: 'csv',
  line_count_valid: 8,
  line_count_duplicate: 1,
  line_count_possible_dup: 0,
  line_count_error: 1,
  created_at: '2026-08-20T09:00:00Z',
  updated_at: '2026-08-20T09:00:00Z',
  lines: null,
};

const discardedBatchFixture: ImportBatch = { ...batchFixture, state: 'discarded' };

const commitSummaryFixture: CommitSummary = { created: 8, skipped: 1 };

const pdfFile = new File(['invoice'], 'invoice.pdf', { type: 'application/pdf' });
const csvFile = new File(['date,amount'], 'statement.csv', { type: 'text/csv' });

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

function makeWrapper(queryClient: QueryClient): (props: { children?: ReactNode }) => ReactElement {
  return function Wrapper({ children }: { children?: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <ActiveUserProvider queryClient={queryClient}>{children}</ActiveUserProvider>
      </QueryClientProvider>
    );
  };
}

/**
 * Render a hook inside QueryClientProvider + ActiveUserProvider. The active
 * user is pre-seeded in localStorage (the provider reads it on mount), or
 * cleared when `user` is `null`.
 */
function renderWithUser<T, P = void>(
  hook: (props: P) => T,
  user: string | null = ALICE,
  initialProps?: P,
): ReturnType<typeof renderHook<T, P>> & { queryClient: QueryClient } {
  const queryClient = makeQueryClient();
  if (user === null) {
    localStorage.removeItem('activeUser');
  } else {
    localStorage.setItem('activeUser', user);
  }
  const rendered = renderHook(hook, {
    initialProps,
    wrapper: makeWrapper(queryClient),
  });
  return { queryClient, ...rendered };
}

/** Every live query key in the client, for exact key-shape assertions. */
function queryKeys(queryClient: QueryClient): ReadonlyArray<ReadonlyArray<unknown>> {
  return queryClient.getQueryCache().getAll().map((query) => query.queryKey);
}

/** The key prefixes passed to every `invalidateQueries` call so far. */
function invalidatedKeys(queryClient: QueryClient): ReadonlyArray<ReadonlyArray<unknown>> {
  const calls = vi.mocked(queryClient.invalidateQueries).mock.calls;
  return calls.map((call) => (call[0] as { queryKey: ReadonlyArray<unknown> }).queryKey);
}

/** Assert the mutation invalidated EXACTLY the given prefixes — no more, no less. */
function expectInvalidatedExactly(
  queryClient: QueryClient,
  expected: ReadonlyArray<ReadonlyArray<unknown>>,
): void {
  const actual = invalidatedKeys(queryClient);
  expect(actual).toHaveLength(expected.length);
  for (const key of expected) {
    expect(actual).toContainEqual(key);
  }
}

beforeEach(() => {
  localStorage.clear();
  vi.mocked(client.apiJson).mockReset();
  vi.mocked(client.apiVoid).mockReset();
  vi.mocked(upload.uploadDocument).mockReset();
  vi.mocked(upload.uploadStatement).mockReset();
});

// ---------------------------------------------------------------------------
// Queries — keys (D5) + user scoping (D4)
// ---------------------------------------------------------------------------

describe('query hooks — every key carries the active user id (D4/D5)', () => {
  it('useAssets: key ["assets", uid] → GET the user-tenanted assets', async () => {
    vi.mocked(client.apiJson).mockResolvedValue([assetFixture]);
    const { queryClient } = renderWithUser(() => useAssets());
    await waitFor(() => {
      expect(queryClient.getQueryData(['assets', ALICE])).toEqual([assetFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['assets', ALICE]);
    expect(client.apiJson).toHaveBeenCalledTimes(1);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'user', userId: ALICE }, 'assets');
  });

  it('useAsset: key ["asset", uid, id] → GET one asset', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(assetFixture);
    const { queryClient } = renderWithUser(() => useAsset('a1'));
    await waitFor(() => {
      expect(queryClient.getQueryData(['asset', ALICE, 'a1'])).toEqual(assetFixture);
    });
    expect(queryKeys(queryClient)).toContainEqual(['asset', ALICE, 'a1']);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'user', userId: ALICE }, 'assets/a1');
  });

  it('useAssetDocuments: key ["asset-docs", uid, id] → GET the asset documents', async () => {
    vi.mocked(client.apiJson).mockResolvedValue([documentFixture]);
    const { queryClient } = renderWithUser(() => useAssetDocuments('a1'));
    await waitFor(() => {
      expect(queryClient.getQueryData(['asset-docs', ALICE, 'a1'])).toEqual([documentFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['asset-docs', ALICE, 'a1']);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'user', userId: ALICE }, 'assets/a1/documents');
  });

  it('useAccounts: key ["accounts", uid] → GET the finance accounts (header tenancy)', async () => {
    vi.mocked(client.apiJson).mockResolvedValue([accountFixture]);
    const { queryClient } = renderWithUser(() => useAccounts());
    await waitFor(() => {
      expect(queryClient.getQueryData(['accounts', ALICE])).toEqual([accountFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['accounts', ALICE]);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'finance', userId: ALICE }, 'accounts');
  });

  it('useMovements (no filters): key ["movements", uid, {}] → GET movements', async () => {
    vi.mocked(client.apiJson).mockResolvedValue([movementFixture]);
    const { queryClient } = renderWithUser(() => useMovements());
    await waitFor(() => {
      expect(queryClient.getQueryData(['movements', ALICE, {}])).toEqual([movementFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['movements', ALICE, {}]);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'finance', userId: ALICE }, 'movements');
  });

  it('useMovements (all filters): key carries {accountId,from,to}; wire params are account_id/from/to', async () => {
    vi.mocked(client.apiJson).mockResolvedValue([movementFixture]);
    const filters: MovementFilterInput = { accountId: 'acc1', from: '2026-01-01', to: '2026-01-31' };
    const { queryClient } = renderWithUser(() => useMovements(filters));
    await waitFor(() => {
      expect(queryClient.getQueryData(['movements', ALICE, filters])).toEqual([movementFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['movements', ALICE, filters]);
    expect(client.apiJson).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'movements?account_id=acc1&from=2026-01-01&to=2026-01-31',
    );
  });

  it('useMovements (partial filters): only supplied params reach the wire', async () => {
    vi.mocked(client.apiJson).mockResolvedValue([]);
    const { queryClient } = renderWithUser(() => useMovements({ from: '2026-01-01' }));
    await waitFor(() => {
      expect(queryClient.getQueryData(['movements', ALICE, { from: '2026-01-01' }])).toEqual([]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['movements', ALICE, { from: '2026-01-01' }]);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'finance', userId: ALICE }, 'movements?from=2026-01-01');
  });

  it('useBatches: key ["batches", uid] → GET the import batches', async () => {
    vi.mocked(client.apiJson).mockResolvedValue([batchFixture]);
    const { queryClient } = renderWithUser(() => useBatches());
    await waitFor(() => {
      expect(queryClient.getQueryData(['batches', ALICE])).toEqual([batchFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['batches', ALICE]);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'finance', userId: ALICE }, 'import-batches');
  });

  it('useBatch: key ["batch", uid, id] → GET one import batch', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(batchFixture);
    const { queryClient } = renderWithUser(() => useBatch('b1'));
    await waitFor(() => {
      expect(queryClient.getQueryData(['batch', ALICE, 'b1'])).toEqual(batchFixture);
    });
    expect(queryKeys(queryClient)).toContainEqual(['batch', ALICE, 'b1']);
    expect(client.apiJson).toHaveBeenCalledWith({ kind: 'finance', userId: ALICE }, 'import-batches/b1');
  });

  it('does not fetch while no user is active (the key keeps the null user)', () => {
    const { queryClient } = renderWithUser(() => useAssets(), null);
    expect(queryKeys(queryClient)).toContainEqual(['assets', null]);
    expect(client.apiJson).not.toHaveBeenCalled();
  });

  it('useMovements keeps the previous list while a filtered re-query is in flight (keepPreviousData)', async () => {
    const neverResolves = new Promise<Movement[]>(() => undefined);
    vi.mocked(client.apiJson)
      .mockResolvedValueOnce([movementFixture])
      .mockReturnValue(neverResolves);

    localStorage.setItem('activeUser', ALICE);
    const queryClient = makeQueryClient();
    const { result, rerender } = renderHook(
      (props?: { filters?: MovementFilterInput }) => useMovements(props?.filters),
      { wrapper: makeWrapper(queryClient) },
    );
    await waitFor(() => {
      expect(result.current.data).toEqual([movementFixture]);
    });

    rerender({ filters: { from: '2026-01-01' } });

    await waitFor(() => {
      expect(result.current.isPlaceholderData).toBe(true);
    });
    expect(result.current.data).toEqual([movementFixture]);
  });
});

// ---------------------------------------------------------------------------
// Mutations — the D5 invalidation map, exactly
// ---------------------------------------------------------------------------

describe('mutation hooks — each invalidates EXACTLY its D5 prefixes', () => {
  const MOVEMENT_KEYS: ReadonlyArray<ReadonlyArray<unknown>> = [
    ['movements', ALICE],
    ['accounts', ALICE],
  ];

  // NOTE: `mutateAsync` resolves after `onSuccess` (the invalidation) has run,
  // so invalidation asserts below are deterministic. React state on
  // `result.current` updates via TanStack's setTimeout(0) notify flush, so
  // state asserts go through `waitFor`, and the returned data is asserted on
  // the `mutateAsync` resolution (which `act` passes through).

  it('useUploadDocument: uploads for the active user, invalidates only ["assets", uid]', async () => {
    vi.mocked(upload.uploadDocument).mockResolvedValue(assetFixture);
    const { result, queryClient } = renderWithUser(() => useUploadDocument());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ file: pdfFile }));
    expect(upload.uploadDocument).toHaveBeenCalledTimes(1);
    expect(upload.uploadDocument).toHaveBeenCalledWith(ALICE, pdfFile, expect.any(Function));
    expect(returned).toEqual(assetFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expect(result.current.data).toEqual(assetFixture);
    expectInvalidatedExactly(queryClient, [['assets', ALICE]]);
  });

  it('useUploadStatement: uploads for the active user/account, invalidates ["batches", uid] + ["batch", uid, id]', async () => {
    vi.mocked(upload.uploadStatement).mockResolvedValue(batchFixture);
    const { result, queryClient } = renderWithUser(() => useUploadStatement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ accountId: 'acc1', file: csvFile }),
    );
    expect(upload.uploadStatement).toHaveBeenCalledWith(ALICE, 'acc1', csvFile, expect.any(Function));
    expect(returned).toEqual(batchFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['batches', ALICE], ['batch', ALICE, 'b1']]);
  });

  it('useCreateAccount: POSTs the account, invalidates only ["accounts", uid]', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(accountFixture);
    const body: CreateAccountRequest = { name: 'Main', type: 'bank', currency: 'INR' };
    const { result, queryClient } = renderWithUser(() => useCreateAccount());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync(body));
    expect(client.apiJson).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'accounts',
      { method: 'POST', body },
    );
    expect(returned).toEqual(accountFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expect(result.current.data).toEqual(accountFixture);
    expectInvalidatedExactly(queryClient, [['accounts', ALICE]]);
  });

  it('useCreateMovement: POSTs the movement, invalidates ["movements", uid] + ["accounts", uid]', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(movementFixture);
    const body: CreateMovementRequest = {
      kind: 'expense',
      amount: '100',
      currency: 'INR',
      occurred_on: '2026-08-20',
      description: 'groceries',
      source_account_id: 'acc1',
    };
    const { result, queryClient } = renderWithUser(() => useCreateMovement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync(body));
    expect(client.apiJson).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'movements',
      { method: 'POST', body },
    );
    expect(returned).toEqual(movementFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('usePatchDescription: PATCHes the description, invalidates movements + accounts', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(movementFixture);
    const { result, queryClient } = renderWithUser(() => usePatchDescription());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ movementId: 'mv1', description: 'corrected' }),
    );
    expect(client.apiJson).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'movements/mv1',
      { method: 'PATCH', body: { description: 'corrected' } },
    );
    expect(returned).toEqual(movementFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useDeleteMovement: DELETEs the movement, invalidates movements + accounts', async () => {
    vi.mocked(client.apiVoid).mockResolvedValue(undefined);
    const { result, queryClient } = renderWithUser(() => useDeleteMovement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ movementId: 'mv1' }));
    expect(client.apiVoid).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'movements/mv1',
      { method: 'DELETE' },
    );
    expect(returned).toBeUndefined();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expect(result.current.data).toBeUndefined();
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useLinkMovement: POSTs document_id, invalidates movements + accounts', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(movementFixture);
    const { result, queryClient } = renderWithUser(() => useLinkMovement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ movementId: 'mv1', documentId: 'd1' }),
    );
    expect(client.apiJson).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'movements/mv1/link',
      { method: 'POST', body: { document_id: 'd1' } },
    );
    expect(returned).toEqual(movementFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useUnlinkMovement: DELETEs the link, invalidates movements + accounts', async () => {
    vi.mocked(client.apiVoid).mockResolvedValue(undefined);
    const { result, queryClient } = renderWithUser(() => useUnlinkMovement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ movementId: 'mv1' }));
    expect(client.apiVoid).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'movements/mv1/link',
      { method: 'DELETE' },
    );
    expect(returned).toBeUndefined();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useCommitBatch: POSTs the commit, invalidates batches + batch + movements + accounts', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(commitSummaryFixture);
    const { result, queryClient } = renderWithUser(() => useCommitBatch());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ batchId: 'b1' }));
    expect(client.apiJson).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'import-batches/b1/commit',
      { method: 'POST' },
    );
    expect(returned).toEqual(commitSummaryFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [
      ['batches', ALICE],
      ['batch', ALICE, 'b1'],
      ['movements', ALICE],
      ['accounts', ALICE],
    ]);
  });

  it('useDiscardBatch: POSTs the discard, invalidates batches + batch (NOT movements/accounts)', async () => {
    vi.mocked(client.apiJson).mockResolvedValue(discardedBatchFixture);
    const { result, queryClient } = renderWithUser(() => useDiscardBatch());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ batchId: 'b1' }));
    expect(client.apiJson).toHaveBeenCalledWith(
      { kind: 'finance', userId: ALICE },
      'import-batches/b1/discard',
      { method: 'POST' },
    );
    expect(returned).toEqual(discardedBatchFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['batches', ALICE], ['batch', ALICE, 'b1']]);
  });

  it('does not invalidate when a mutation fails', async () => {
    vi.mocked(client.apiJson).mockRejectedValue(
      new ApiError(400, 'That request was invalid. Check the details and try again.', 'name is required'),
    );
    const { result, queryClient } = renderWithUser(() => useCreateAccount());
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    await act(async () => {
      await expect(
        result.current.mutateAsync({ name: 'Main', type: 'bank', currency: 'INR' }),
      ).rejects.toBeInstanceOf(ApiError);
    });
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(invalidate).not.toHaveBeenCalled();
  });

  it('useCommitBatch does not invalidate anything when the commit fails', async () => {
    vi.mocked(client.apiJson).mockRejectedValue(
      new ApiError(409, 'This clashes with the current state. Refresh and try again.', 'batch already committed'),
    );
    const { result, queryClient } = renderWithUser(() => useCommitBatch());
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    await act(async () => {
      await expect(result.current.mutateAsync({ batchId: 'b1' })).rejects.toBeInstanceOf(ApiError);
    });
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(invalidate).not.toHaveBeenCalled();
  });

  it('a mutation with no active user does not call the client', async () => {
    const { result } = renderWithUser(() => useCreateAccount(), null);
    await act(async () => {
      await expect(
        result.current.mutateAsync({ name: 'Main', type: 'bank', currency: 'INR' }),
      ).rejects.toThrow();
    });
    expect(client.apiJson).not.toHaveBeenCalled();
  });
});
