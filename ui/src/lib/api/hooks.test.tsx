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
// Orphan mutations + shared plumbing live in ./hooks; the feature-facing hooks
// are co-located with their features (asset-management-v2 modular-structure).
import {
  useDeleteAsset,
  useMergeAsset,
  usePatchAsset,
  useUnlinkMovement,
  useUploadDocument,
} from './hooks';
import { useAccounts, useAssetDocuments, useAssets } from '@/features/docs/hooks';
import { useAsset } from '@/features/assets/use-asset';
import {
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
  useUploadStatement,
} from '@/features/finance/hooks';
import { useAdd, useRestoreAsset } from '@/features/add/hooks';
import { useSearch } from '@/features/search/use-search';
import type { MovementFilterInput, SearchFilterInput } from './hooks';
import { ApiError } from './errors';
import type {
  Account,
  AddItemOutcome,
  Asset,
  CommitSummary,
  CreateAccountRequest,
  CreateMovementInput,
  Document,
  ImportBatch,
  Movement,
  SearchResultsPage,
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
    deleteAsset: vi.fn(),
    restoreAsset: vi.fn(),
    mergeAsset: vi.fn(),
    patchAsset: vi.fn(),
    addItems: vi.fn(),
    quickSearch: vi.fn(),
    search: vi.fn(),
  };
});

vi.mock('./upload', () => ({
  uploadDocument: vi.fn(),
  uploadStatement: vi.fn(),
}));

const ALICE = 'alice';

const assetFixture: Asset = {
  id: 'a1',
  data: {
    brand: 'Dell',
    model: 'XPS 13',
    serial_number: 'SN-1',
    purchase_date: '2026-01-15T00:00:00Z',
    warranty_end: '2026-09-01T00:00:00Z',
    price: '39999.99',
    currency: 'INR',
    metadata: {},
  },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z',
};

const documentFixture: Document = {
  id: 'd1',
  source_id: 's1',
  data: {
    doc_type: 'invoice',
  },
  source_filename: 'invoice.pdf',
  source_uploaded_at: '2026-08-01T10:00:00Z',
  status: 'processed',
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z',
};

const accountFixture: Account = {
  id: 'acc1',
  data: {
    name: 'Main',
    account_type: 'bank',
    currency: 'INR',
    balance: '150',
  },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z',
};

const movementFixture: Movement = {
  id: 'mv1',
  source_account_id: 'acc1',
  data: {
    kind: 'expense',
    amount: '100',
    currency: 'INR',
    occurred_on: '2026-08-20',
    recorded_at: '2026-08-20T09:00:00Z',
    description: 'groceries',
    origin: 'manual',
    link_conflicting: false,
  },
  created_at: '2026-08-20T09:00:00Z',
  updated_at: '2026-08-20T09:00:00Z',
};

const batchFixture: ImportBatch = {
  id: 'b1',
  account_id: 'acc1',
  source: {
    id: 's1',
    filename: 'statement.csv',
    content_type: 'text/csv',
    size: 1024,
    sha256: 'abc123',
    uploaded_at: '2026-08-20T09:00:00Z',
  },
  data: {
    state: 'preview',
    filename: 'statement.csv',
    format: 'csv',
    line_count_valid: 8,
    line_count_duplicate: 1,
    line_count_possible_dup: 0,
    line_count_error: 1,
    lines: null,
  },
  created_at: '2026-08-20T09:00:00Z',
  updated_at: '2026-08-20T09:00:00Z',
};

const discardedBatchFixture: ImportBatch = {
  ...batchFixture,
  data: { ...batchFixture.data, state: 'discarded' },
};
const commitSummaryFixture: CommitSummary = { created: 8, skipped: 1 };
const pdfFile = new File(['invoice'], 'invoice.pdf', { type: 'application/pdf' });
const csvFile = new File(['date,amount'], 'statement.csv', { type: 'text/csv' });

const restoredAssetFixture: Asset = { ...assetFixture };
const patchedAssetFixture: Asset = {
  ...assetFixture,
  data: { ...assetFixture.data, name: 'Microwave Oven' },
};

const addOutcomesFixture: AddItemOutcome[] = [
  { kind: 'asset_committed', asset_id: 'a1' },
  { kind: 'statement_preview', import_batch_id: 'b1' },
];

const searchPageFixture: SearchResultsPage = {
  results: [],
  total: 0,
  page: 1,
  page_size: 20,
};

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
    vi.mocked(upload.uploadDocument).mockResolvedValue({ kind: 'committed', asset: assetFixture });
    const { result, queryClient } = renderWithUser(() => useUploadDocument());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ file: pdfFile }));
    expect(upload.uploadDocument).toHaveBeenCalledWith(ALICE, pdfFile, expect.any(Function));
    expect(returned).toEqual({ kind: 'committed', asset: assetFixture });
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

  it('useDeleteAsset: DELETEs the asset, invalidates ["assets", uid] + ["asset", uid, id]', async () => {
    vi.mocked(client.deleteAsset).mockResolvedValue(undefined);
    const { result, queryClient } = renderWithUser(() => useDeleteAsset());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ assetId: 'a1' }));
    expect(client.deleteAsset).toHaveBeenCalledWith(ALICE, 'a1');
    expect(returned).toBeUndefined();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['assets', ALICE], ['asset', ALICE, 'a1']]);
  });

  it('useRestoreAsset: POSTs the restore, invalidates ["assets", uid] + ["asset", uid, id]', async () => {
    vi.mocked(client.restoreAsset).mockResolvedValue(restoredAssetFixture);
    const { result, queryClient } = renderWithUser(() => useRestoreAsset());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ assetId: 'a1' }));
    expect(client.restoreAsset).toHaveBeenCalledWith(ALICE, 'a1');
    expect(returned).toEqual(restoredAssetFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['assets', ALICE], ['asset', ALICE, 'a1']]);
  });

  it('useMergeAsset: POSTs duplicate_asset_id, invalidates assets + survivor + duplicate', async () => {
    vi.mocked(client.mergeAsset).mockResolvedValue(assetFixture);
    const { result, queryClient } = renderWithUser(() => useMergeAsset());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ assetId: 'a1', duplicateAssetId: 'a2' }),
    );
    expect(client.mergeAsset).toHaveBeenCalledWith(ALICE, 'a1', { duplicate_asset_id: 'a2' });
    expect(returned).toEqual(assetFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [
      ['assets', ALICE],
      ['asset', ALICE, 'a1'],
      ['asset', ALICE, 'a2'],
    ]);
  });

  it('usePatchAsset: PATCHes the asset, invalidates ["assets", uid] + ["asset", uid, id]', async () => {
    vi.mocked(client.patchAsset).mockResolvedValue(patchedAssetFixture);
    const body = { name: 'Microwave Oven', asset_category: 'appliance' as const };
    const { result, queryClient } = renderWithUser(() => usePatchAsset());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ assetId: 'a1', body }),
    );
    expect(client.patchAsset).toHaveBeenCalledWith(ALICE, 'a1', body);
    expect(returned).toEqual(patchedAssetFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['assets', ALICE], ['asset', ALICE, 'a1']]);
  });

  it('useAdd: POSTs files/text/account, invalidates ["assets", uid] + ["batches", uid]', async () => {
    vi.mocked(client.addItems).mockResolvedValue(addOutcomesFixture);
    const { result, queryClient } = renderWithUser(() => useAdd());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ files: [pdfFile], text: 'Microwave Oven', account_id: 'acc1' }),
    );
    expect(client.addItems).toHaveBeenCalledWith(ALICE, {
      files: [pdfFile],
      text: 'Microwave Oven',
      account_id: 'acc1',
    });
    expect(returned).toEqual(addOutcomesFixture);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['assets', ALICE], ['batches', ALICE]]);
  });

  it('useDeleteAsset: does not invalidate when the delete fails', async () => {
    vi.mocked(client.deleteAsset).mockRejectedValue(
      new ApiError(409, 'That request was invalid.', 'asset is outside the retention window'),
    );
    const { result, queryClient } = renderWithUser(() => useDeleteAsset());
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    await act(async () => {
      await expect(result.current.mutateAsync({ assetId: 'a1' })).rejects.toBeInstanceOf(ApiError);
    });
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(invalidate).not.toHaveBeenCalled();
  });

  it('usePatchAsset: rejects without sending a request when no user is active', async () => {
    const { result } = renderWithUser(() => usePatchAsset(), null);
    await act(async () => {
      await expect(
        result.current.mutateAsync({
          assetId: 'a1',
          body: { asset_category: 'appliance' as const },
        }),
      ).rejects.toThrow('No active user selected');
    });
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(client.patchAsset).not.toHaveBeenCalled();
  });
});

describe('useSearch — structured filters in the key, mapped to wire params', () => {
  const FULL_FILTERS: SearchFilterInput = {
    category: 'appliance',
    brand: 'LG',
    purchaseFrom: '2026-01-01',
    purchaseTo: '2026-12-31',
    warrantyStatus: 'expiring_within:90',
    hasDocuments: true,
    docClassification: 'invoice',
  };

  it('no filters: key ["search", uid, q, {}, 1, 20]; wire params carry undefined filter fields', async () => {
    vi.mocked(client.search).mockResolvedValue(searchPageFixture);
    const { queryClient } = renderWithUser(() => useSearch('microwave'));
    await waitFor(() => {
      expect(queryClient.getQueryData(['search', ALICE, 'microwave', {}, 1, 20])).toEqual(
        searchPageFixture,
      );
    });
    expect(queryKeys(queryClient)).toContainEqual(['search', ALICE, 'microwave', {}, 1, 20]);
    expect(client.search).toHaveBeenCalledWith(ALICE, {
      q: 'microwave',
      page: 1,
      page_size: 20,
      category: undefined,
      brand: undefined,
      purchase_from: undefined,
      purchase_to: undefined,
      warranty_status: undefined,
      has_documents: undefined,
      doc_classification: undefined,
    });
  });

  it('filters: normalized filters are in the key and mapped to wire params', async () => {
    vi.mocked(client.search).mockResolvedValue(searchPageFixture);
    const { queryClient } = renderWithUser(() => useSearch('microwave', FULL_FILTERS));
    await waitFor(() => {
      expect(queryClient.getQueryData(['search', ALICE, 'microwave', FULL_FILTERS, 1, 20])).toEqual(
        searchPageFixture,
      );
    });
    expect(queryKeys(queryClient)).toContainEqual(['search', ALICE, 'microwave', FULL_FILTERS, 1, 20]);
    expect(client.search).toHaveBeenCalledWith(ALICE, {
      q: 'microwave',
      page: 1,
      page_size: 20,
      category: 'appliance',
      brand: 'LG',
      purchase_from: '2026-01-01',
      purchase_to: '2026-12-31',
      warranty_status: 'expiring_within:90',
      has_documents: true,
      doc_classification: 'invoice',
    });
  });

  it('changing only the filters changes the key and triggers a new request', async () => {
    vi.mocked(client.search).mockResolvedValue(searchPageFixture);
    interface SearchProps {
      filters?: SearchFilterInput;
    }
    const initialProps: SearchProps = { filters: { category: 'appliance' } };
    const { queryClient, result, rerender } = renderWithUser(
      (props: SearchProps) => useSearch('microwave', props.filters),
      ALICE,
      initialProps,
    );
    await waitFor(() => {
      expect(result.current.data).toEqual(searchPageFixture);
    });
    expect(client.search).toHaveBeenCalledTimes(1);

    rerender({ filters: { brand: 'LG' } });
    await waitFor(() => {
      expect(client.search).toHaveBeenCalledTimes(2);
    });
    // The new key carries the new normalized filters; the old key stays cached.
    expect(queryKeys(queryClient)).toContainEqual(['search', ALICE, 'microwave', { brand: 'LG' }, 1, 20]);
    expect(queryKeys(queryClient)).toContainEqual([
      'search',
      ALICE,
      'microwave',
      { category: 'appliance' },
      1,
      20,
    ]);
  });

  it('back-compat: useSearch(q, page, pageSize) (number 2nd arg) keeps the old behavior', async () => {
    vi.mocked(client.search).mockResolvedValue(searchPageFixture);
    // `useSearch('laptop', 2, 50)` ≡ `useSearch('laptop', undefined, 2, 50)` —
    // the exact call shape search-results-page.tsx uses today.
    const { queryClient } = renderWithUser(() => useSearch('laptop', 2, 50));
    await waitFor(() => {
      expect(queryClient.getQueryData(['search', ALICE, 'laptop', {}, 2, 50])).toEqual(
        searchPageFixture,
      );
    });
    expect(queryKeys(queryClient)).toContainEqual(['search', ALICE, 'laptop', {}, 2, 50]);
    expect(client.search).toHaveBeenCalledWith(ALICE, {
      q: 'laptop',
      page: 2,
      page_size: 50,
      category: undefined,
      brand: undefined,
      purchase_from: undefined,
      purchase_to: undefined,
      warranty_status: undefined,
      has_documents: undefined,
      doc_classification: undefined,
    });
  });
});
