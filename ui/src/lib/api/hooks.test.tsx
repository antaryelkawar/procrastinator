import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider, useActiveUser } from '../../context/active-user';
import * as client from './client';

vi.mock('../../context/active-user', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../context/active-user')>();
  return { ...actual, useActiveUser: vi.fn() };
});
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
  CreateMovementInput,
  Document,
  ImportBatch,
  Movement,
} from './schema';

vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>();
  return {
    ...actual,
    listAssets: vi.fn(),
    getAsset: vi.fn(),
    listAssetDocuments: vi.fn(),
    listAccounts: vi.fn(),
    getAccount: vi.fn(),
    createAccount: vi.fn(),
    listMovements: vi.fn(),
    getMovement: vi.fn(),
    createMovement: vi.fn(),
    patchMovement: vi.fn(),
    deleteMovement: vi.fn(),
    linkMovement: vi.fn(),
    unlinkMovement: vi.fn(),
    listImportBatches: vi.fn(),
    getImportBatch: vi.fn(),
    commitImportBatch: vi.fn(),
    discardImportBatch: vi.fn(),
    listHouseholds: vi.fn(),
    getHousehold: vi.fn(),
    createHousehold: vi.fn(),
    addHouseholdMember: vi.fn(),
  };
});

vi.mock('./upload', () => ({
  uploadDocument: vi.fn(),
  uploadStatement: vi.fn(),
}));

const ALICE = 'alice';

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
};

const documentFixture: Document = {
  id: 'd1',
  doc_type: 'invoice',
  source_filename: 'invoice.pdf',
  source_uploaded_at: '2026-08-01T10:00:00Z',
  created_at: '2026-08-01T10:00:00Z',
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

function renderWithUser<T, P = void>(
  hook: (props: P) => T,
  user: string | null = ALICE,
  initialProps?: P,
): ReturnType<typeof renderHook<T, P>> & { queryClient: QueryClient } {
  const queryClient = makeQueryClient();
  vi.mocked(useActiveUser).mockReturnValue({
    activeUser: user,
    setActiveUser: () => false,
    clearActiveUser: () => undefined,
  });

  const rendered = renderHook(hook, {
    initialProps,
    wrapper: makeWrapper(queryClient),
  });
  return { queryClient, ...rendered };
}

function queryKeys(queryClient: QueryClient): ReadonlyArray<ReadonlyArray<unknown>> {
  return queryClient.getQueryCache().getAll().map((query) => query.queryKey);
}

function invalidatedKeys(queryClient: QueryClient): ReadonlyArray<ReadonlyArray<unknown>> {
  const calls = vi.mocked(queryClient.invalidateQueries).mock.calls;
  return calls.map((call) => (call[0] as { queryKey: ReadonlyArray<unknown> }).queryKey);
}

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
  vi.clearAllMocks();
});

describe('query hooks — every key carries the active user id', () => {
  it('useAssets: key ["assets", uid] → GET the user-tenanted assets', async () => {
    vi.mocked(client.listAssets).mockResolvedValue([assetFixture]);
    const { queryClient } = renderWithUser(() => useAssets());
    await waitFor(() => {
      expect(queryClient.getQueryData(['assets', ALICE])).toEqual([assetFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['assets', ALICE]);
    expect(client.listAssets).toHaveBeenCalledWith(ALICE);
  });

  it('useAsset: key ["asset", uid, id] → GET one asset', async () => {
    vi.mocked(client.getAsset).mockResolvedValue(assetFixture);
    const { queryClient } = renderWithUser(() => useAsset('a1'));
    await waitFor(() => {
      expect(queryClient.getQueryData(['asset', ALICE, 'a1'])).toEqual(assetFixture);
    });
    expect(queryKeys(queryClient)).toContainEqual(['asset', ALICE, 'a1']);
    expect(client.getAsset).toHaveBeenCalledWith(ALICE, 'a1');
  });

  it('useAssetDocuments: key ["asset-docs", uid, id] → GET the asset documents', async () => {
    vi.mocked(client.listAssetDocuments).mockResolvedValue([documentFixture]);
    const { queryClient } = renderWithUser(() => useAssetDocuments('a1'));
    await waitFor(() => {
      expect(queryClient.getQueryData(['asset-docs', ALICE, 'a1'])).toEqual([documentFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['asset-docs', ALICE, 'a1']);
    expect(client.listAssetDocuments).toHaveBeenCalledWith(ALICE, 'a1');
  });

  it('useAccounts: key ["accounts", uid] → GET the finance accounts', async () => {
    vi.mocked(client.listAccounts).mockResolvedValue([accountFixture]);
    const { queryClient } = renderWithUser(() => useAccounts());
    await waitFor(() => {
      expect(queryClient.getQueryData(['accounts', ALICE])).toEqual([accountFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['accounts', ALICE]);
    expect(client.listAccounts).toHaveBeenCalledWith(ALICE);
  });

  it('useMovements (no filters): key ["movements", uid, {}] → GET movements', async () => {
    vi.mocked(client.listMovements).mockResolvedValue([movementFixture]);
    const { queryClient } = renderWithUser(() => useMovements());
    await waitFor(() => {
      expect(queryClient.getQueryData(['movements', ALICE, {}])).toEqual([movementFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['movements', ALICE, {}]);
    expect(client.listMovements).toHaveBeenCalledWith(ALICE, {
      account_id: undefined,
      from: undefined,
      to: undefined,
    });
  });

  it('useMovements (all filters): key carries {accountId,from,to}', async () => {
    vi.mocked(client.listMovements).mockResolvedValue([movementFixture]);
    const filters: MovementFilterInput = { accountId: 'acc1', from: '2026-01-01', to: '2026-01-31' };
    const { queryClient } = renderWithUser(() => useMovements(filters));
    await waitFor(() => {
      expect(queryClient.getQueryData(['movements', ALICE, filters])).toEqual([movementFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['movements', ALICE, filters]);
    expect(client.listMovements).toHaveBeenCalledWith(ALICE, {
      account_id: 'acc1',
      from: '2026-01-01',
      to: '2026-01-31',
    });
  });

  it('useMovements (partial filters): only supplied params reach the wire', async () => {
    vi.mocked(client.listMovements).mockResolvedValue([]);
    const { queryClient } = renderWithUser(() => useMovements({ from: '2026-01-01' }));
    await waitFor(() => {
      expect(queryClient.getQueryData(['movements', ALICE, { from: '2026-01-01' }])).toEqual([]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['movements', ALICE, { from: '2026-01-01' }]);
    expect(client.listMovements).toHaveBeenCalledWith(ALICE, {
      account_id: undefined,
      from: '2026-01-01',
      to: undefined,
    });
  });

  it('useBatches: key ["batches", uid] → GET the import batches', async () => {
    vi.mocked(client.listImportBatches).mockResolvedValue([batchFixture]);
    const { queryClient } = renderWithUser(() => useBatches());
    await waitFor(() => {
      expect(queryClient.getQueryData(['batches', ALICE])).toEqual([batchFixture]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['batches', ALICE]);
    expect(client.listImportBatches).toHaveBeenCalledWith(ALICE);
  });

  it('useBatch: key ["batch", uid, id] → GET one import batch', async () => {
    vi.mocked(client.getImportBatch).mockResolvedValue(batchFixture);
    const { queryClient } = renderWithUser(() => useBatch('b1'));
    await waitFor(() => {
      expect(queryClient.getQueryData(['batch', ALICE, 'b1'])).toEqual(batchFixture);
    });
    expect(queryKeys(queryClient)).toContainEqual(['batch', ALICE, 'b1']);
    expect(client.getImportBatch).toHaveBeenCalledWith(ALICE, 'b1');
  });
});

describe('mutation hooks — each invalidates EXACTLY its D5 prefixes', () => {
  const MOVEMENT_KEYS: ReadonlyArray<ReadonlyArray<unknown>> = [
    ['movements', ALICE],
    ['accounts', ALICE],
  ];

  it('useUploadDocument: uploads for the active user, invalidates only ["assets", uid]', async () => {
    vi.mocked(upload.uploadDocument).mockResolvedValue(assetFixture);
    const { result, queryClient } = renderWithUser(() => useUploadDocument());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ file: pdfFile }));
    expect(upload.uploadDocument).toHaveBeenCalledWith(ALICE, pdfFile, expect.any(Function));
    expect(returned).toEqual(assetFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
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
    vi.mocked(client.createAccount).mockResolvedValue(accountFixture);
    const body: CreateAccountRequest = { name: 'Main', type: 'bank', currency: 'INR' };
    const { result, queryClient } = renderWithUser(() => useCreateAccount());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync(body));
    expect(client.createAccount).toHaveBeenCalledWith(ALICE, body);
    expect(returned).toEqual(accountFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['accounts', ALICE]]);
  });

  it('useCreateMovement: POSTs the movement, invalidates ["movements", uid] + ["accounts", uid]', async () => {
    vi.mocked(client.createMovement).mockResolvedValue(movementFixture);
    const body: CreateMovementInput = {
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
    expect(client.createMovement).toHaveBeenCalledWith(ALICE, body);
    expect(returned).toEqual(movementFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('usePatchDescription: PATCHes the description, invalidates movements + accounts', async () => {
    vi.mocked(client.patchMovement).mockResolvedValue(movementFixture);
    const { result, queryClient } = renderWithUser(() => usePatchDescription());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ movementId: 'mv1', description: 'corrected' }),
    );
    expect(client.patchMovement).toHaveBeenCalledWith(ALICE, 'mv1', {
      description: 'corrected',
      amount: '',
      currency: '',
      occurred_on: '',
      kind: '',
      source_account_id: '',
      destination_account_id: '',
    });
    expect(returned).toEqual(movementFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useDeleteMovement: DELETEs the movement, invalidates movements + accounts', async () => {
    vi.mocked(client.deleteMovement).mockResolvedValue();
    const { result, queryClient } = renderWithUser(() => useDeleteMovement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ movementId: 'mv1' }));
    expect(client.deleteMovement).toHaveBeenCalledWith(ALICE, 'mv1');
    expect(returned).toBeUndefined();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useLinkMovement: POSTs document_id, invalidates movements + accounts', async () => {
    vi.mocked(client.linkMovement).mockResolvedValue(movementFixture);
    const { result, queryClient } = renderWithUser(() => useLinkMovement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ movementId: 'mv1', documentId: 'd1' }),
    );
    expect(client.linkMovement).toHaveBeenCalledWith(ALICE, 'mv1', { document_id: 'd1' });
    expect(returned).toEqual(movementFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useUnlinkMovement: DELETEs the link, invalidates movements + accounts', async () => {
    vi.mocked(client.unlinkMovement).mockResolvedValue();
    const { result, queryClient } = renderWithUser(() => useUnlinkMovement());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ movementId: 'mv1' }));
    expect(client.unlinkMovement).toHaveBeenCalledWith(ALICE, 'mv1');
    expect(returned).toBeUndefined();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, MOVEMENT_KEYS);
  });

  it('useCommitBatch: POSTs the commit, invalidates batches + batch + movements + accounts', async () => {
    vi.mocked(client.commitImportBatch).mockResolvedValue(commitSummaryFixture);
    const { result, queryClient } = renderWithUser(() => useCommitBatch());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ batchId: 'b1' }));
    expect(client.commitImportBatch).toHaveBeenCalledWith(ALICE, 'b1');
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

  it('useDiscardBatch: POSTs the discard, invalidates batches + batch', async () => {
    vi.mocked(client.discardImportBatch).mockResolvedValue(discardedBatchFixture);
    const { result, queryClient } = renderWithUser(() => useDiscardBatch());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ batchId: 'b1' }));
    expect(client.discardImportBatch).toHaveBeenCalledWith(ALICE, 'b1');
    expect(returned).toEqual(discardedBatchFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['batches', ALICE], ['batch', ALICE, 'b1']]);
  });

  it('does not invalidate when a mutation fails', async () => {
    vi.mocked(client.createAccount).mockRejectedValue(
      new ApiError(400, 'That request was invalid.', 'name is required'),
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
});
