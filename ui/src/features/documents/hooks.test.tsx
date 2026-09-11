/**
 * Documents-feature data-layer hook tests (task 8.1).
 *
 * Pattern: `lib/api/hooks.test.tsx` — mock `@/lib/api/client`, render the
 * hooks through `QueryClientProvider` + `ActiveUserProvider`, and assert the
 * query keys + the client calls + the D5 invalidation prefixes.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider, useActiveUser } from '@/context/active-user';
import * as client from '@/lib/api/client';
import {
  useDeleteDocument,
  useDocuments,
  useReprocessDocument,
} from './hooks';
import { ApiError } from '@/lib/api/errors';
import type { DocumentRow } from '@/lib/api/client';

vi.mock('@/context/active-user', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/context/active-user')>();
  return { ...actual, useActiveUser: vi.fn() };
});

vi.mock('@/lib/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/client')>();
  return {
    ...actual,
    listDocuments: vi.fn(),
    deleteDocument: vi.fn(),
    reprocessDocument: vi.fn(),
  };
});

const ALICE = 'alice';

function makeDocumentRow(overrides: Partial<DocumentRow> = {}): DocumentRow {
  return {
    id: 'd1',
    asset_id: null,
    source_id: 's1',
    source_filename: 'invoice.pdf',
    source_uploaded_at: '2026-08-01T10:00:00Z',
    owner_household_id: null,
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-01T10:00:00Z',
    data: { doc_type: 'invoice' },
    status: 'processed',
    ...overrides,
  } as unknown as DocumentRow;
}

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

describe('useDocuments', () => {
  it('no filters: key ["documents", uid, {status:undefined,q:undefined}] → listDocuments with no params', async () => {
    const rows = [makeDocumentRow()];
    vi.mocked(client.listDocuments).mockResolvedValue(rows);
    const { queryClient } = renderWithUser(() => useDocuments());
    await waitFor(() => {
      expect(queryClient.getQueryData(['documents', ALICE, { status: undefined, q: undefined }])).toEqual(rows);
    });
    expect(queryKeys(queryClient)).toContainEqual([
      'documents',
      ALICE,
      { status: undefined, q: undefined },
    ]);
    // No filters → the hook passes `filters` through as `undefined` (the
    // client drops undefined/empty params from the query string).
    expect(client.listDocuments).toHaveBeenCalledWith(ALICE, undefined);
  });

  it('status filter: key carries {status, q:undefined} and the wire params carry only status', async () => {
    vi.mocked(client.listDocuments).mockResolvedValue([]);
    const { queryClient } = renderWithUser(() => useDocuments({ status: 'failed' }));
    await waitFor(() => {
      expect(queryClient.getQueryData(['documents', ALICE, { status: 'failed', q: undefined }])).toEqual([]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['documents', ALICE, { status: 'failed', q: undefined }]);
    expect(client.listDocuments).toHaveBeenCalledWith(ALICE, { status: 'failed', q: undefined });
  });

  it('q filter: key carries {status:undefined, q} and the wire params carry only q', async () => {
    vi.mocked(client.listDocuments).mockResolvedValue([]);
    const { queryClient } = renderWithUser(() => useDocuments({ q: 'invoice' }));
    await waitFor(() => {
      expect(queryClient.getQueryData(['documents', ALICE, { status: undefined, q: 'invoice' }])).toEqual([]);
    });
    expect(queryKeys(queryClient)).toContainEqual(['documents', ALICE, { status: undefined, q: 'invoice' }]);
    expect(client.listDocuments).toHaveBeenCalledWith(ALICE, { status: undefined, q: 'invoice' });
  });

  it('both filters: key carries {status, q}', async () => {
    vi.mocked(client.listDocuments).mockResolvedValue([]);
    const { queryClient } = renderWithUser(() => useDocuments({ status: 'in_review', q: 'washer' }));
    await waitFor(() => {
      expect(
        queryClient.getQueryData(['documents', ALICE, { status: 'in_review', q: 'washer' }]),
      ).toEqual([]);
    });
    expect(queryKeys(queryClient)).toContainEqual([
      'documents',
      ALICE,
      { status: 'in_review', q: 'washer' },
    ]);
    expect(client.listDocuments).toHaveBeenCalledWith(ALICE, { status: 'in_review', q: 'washer' });
  });

  it('is disabled when no user is active', async () => {
    const { result } = renderWithUser(() => useDocuments(), null);
    await new Promise((resolve) => setTimeout(resolve, 50));
    // Disabled: the query never fetches (idle) and no request goes out.
    expect(result.current.fetchStatus).toBe('idle');
    expect(result.current.data).toBeUndefined();
    expect(client.listDocuments).not.toHaveBeenCalled();
  });
});

describe('useDeleteDocument', () => {
  it('DELETEs the document and invalidates exactly ["documents", uid]', async () => {
    vi.mocked(client.deleteDocument).mockResolvedValue(undefined);
    const { result, queryClient } = renderWithUser(() => useDeleteDocument());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () => result.current.mutateAsync({ documentId: 'd1' }));
    expect(client.deleteDocument).toHaveBeenCalledWith(ALICE, 'd1');
    expect(returned).toBeUndefined();
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['documents', ALICE]]);
  });

  it('does not invalidate when the delete fails', async () => {
    vi.mocked(client.deleteDocument).mockRejectedValue(
      new ApiError(409, 'That request was invalid.', 'document is outside the retention window'),
    );
    const { result, queryClient } = renderWithUser(() => useDeleteDocument());
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    await act(async () => {
      await expect(result.current.mutateAsync({ documentId: 'd1' })).rejects.toBeInstanceOf(ApiError);
    });
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(invalidate).not.toHaveBeenCalled();
  });
});

describe('useReprocessDocument', () => {
  it('POSTs the reprocess (comment passed when non-empty) and invalidates ["documents", uid]', async () => {
    vi.mocked(client.reprocessDocument).mockResolvedValue(makeDocumentRow({ status: 'in_review' }));
    const { result, queryClient } = renderWithUser(() => useReprocessDocument());
    vi.spyOn(queryClient, 'invalidateQueries');
    const returned = await act(async () =>
      result.current.mutateAsync({ documentId: 'd1', comment: 'it is a microwave' }),
    );
    expect(client.reprocessDocument).toHaveBeenCalledWith(ALICE, 'd1', 'it is a microwave');
    expect(returned).toEqual(makeDocumentRow({ status: 'in_review' }));
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['documents', ALICE]]);
  });

  it('omits the comment argument when it is undefined', async () => {
    vi.mocked(client.reprocessDocument).mockResolvedValue(makeDocumentRow({ status: 'in_review' }));
    const { result, queryClient } = renderWithUser(() => useReprocessDocument());
    vi.spyOn(queryClient, 'invalidateQueries');
    await act(async () => result.current.mutateAsync({ documentId: 'd2' }));
    expect(client.reprocessDocument).toHaveBeenCalledWith(ALICE, 'd2', undefined);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
    expectInvalidatedExactly(queryClient, [['documents', ALICE]]);
  });

  it('surfaces a 409 (in-flight) as a rejected mutation without invalidating', async () => {
    vi.mocked(client.reprocessDocument).mockRejectedValue(
      new ApiError(409, 'This clashes with the current state. Refresh and try again.', 'reprocess in flight'),
    );
    const { result, queryClient } = renderWithUser(() => useReprocessDocument());
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    await act(async () => {
      await expect(
        result.current.mutateAsync({ documentId: 'd1', comment: 'again' }),
      ).rejects.toMatchObject({ status: 409 });
    });
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(invalidate).not.toHaveBeenCalled();
  });
});
