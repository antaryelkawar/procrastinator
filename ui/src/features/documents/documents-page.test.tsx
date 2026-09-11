/**
 * Documents list page tests (task 8.1).
 *
 * Pattern: `asset-list-page.test.tsx` — mock `react-router` (`useNavigate`)
 * and the feature hooks, render inside `MemoryRouter`, and drive the
 * Radix-powered controls with `fireEvent`/`userEvent` + `flushMacrotasks`.
 *
 * The Radix `DropdownMenu` open/close sequence is slow under jsdom (~17–20s per
 * interaction — a known Radix/React-19/jsdom limitation, see the per-test
 * comments). The menu-driven tests therefore carry a generous 60s timeout with
 * headroom over that runtime so they stay green under full-suite parallel load
 * (a tighter 20s boundary flaked randomly across runs).
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';

import { DocumentsPage } from './documents-page';
import * as docHooks from './hooks';
import * as apiHooks from '@/lib/api/hooks';
import * as sonner from 'sonner';
import * as composerHooks from '@/features/docs/composer/hooks';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider } from '@/context/active-user';
import type { DocumentRow } from '@/lib/api/client';
import type { DuplicateReport } from '@/lib/api/generated/orval/procrastinator';
import { ApiError } from '@/lib/api/errors';
import { MemoryRouter } from 'react-router';

const mockNavigate = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('./hooks', () => ({
  useDocuments: vi.fn(),
  useDeleteDocument: vi.fn(),
  useReprocessDocument: vi.fn(),
}));

vi.mock('@/lib/api/hooks', () => ({
  usePatchAsset: vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  }),
}));

vi.mock('@/features/docs/composer/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/features/docs/composer/hooks')>('@/features/docs/composer/hooks');
  return {
    ...actual,
    useIngestFile: vi.fn(),
    useIngestText: vi.fn(),
    useReprocessDocument: vi.fn(),
    useKeepDocument: vi.fn(),
  };
});

vi.mock('@/context/active-user', async () => {
  const actual = await vi.importActual<typeof import('@/context/active-user')>('@/context/active-user');
  return { ...actual, useActiveUser: vi.fn(() => ({ activeUser: 'alice' })) };
});

/**
 * Flush pending macrotasks so Radix's open-menu `setTimeout` callbacks (focus
 * management, pointer-grace timers) fire and the event loop drains.
 */
function flushMacrotasks(ms = 500): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function makeRow(overrides: Partial<DocumentRow> = {}): DocumentRow {
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

function mockDocuments(data: DocumentRow[] | undefined, isLoading = false): void {
  vi.mocked(docHooks.useDocuments).mockReturnValue({
    data,
    isLoading,
    error: null,
  } as never);
}

/**
 * Shared render helper (task 14.6) wrapping the page in the providers the
 * shared AddComposer requires (`useIngestFile`/`useIngestText`/`useActiveUser`).
 * Mirrors `renderLanding` in `features/landing/landing-page.test.tsx`.
 */
function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ActiveUserProvider queryClient={queryClient}>
          <DocumentsPage />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const DUPLICATE_REPORT: DuplicateReport = {
  code: 'duplicate',
  existing_document_id: 'doc-1',
  existing_source_filename: 'existing-invoice.pdf',
  existing_source_uploaded_at: '2026-09-01T10:00:00Z',
  existing_asset_id: 'a1',
  prompt: {
    reprocess_uri: '/api/users/alice/documents/doc-1/reprocess',
    keep_uri: '/api/users/alice/documents/doc-1/keep',
    expires_at: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
    timeout_toast: 'no response — keeping existing document',
  },
};

function makeFile(name: string, type: string): File {
  return new File(['fake-bytes'], name, { type });
}

async function pickFile(selector: string, file: File): Promise<void> {
  const input = document.querySelector(selector) as HTMLInputElement | null;
  if (input === null) {
    throw new Error(`file input not found: ${selector}`);
  }
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  await act(async () => {
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

function makeUseIngestFile(result: unknown, mutate = vi.fn()) {
  const mutateAsync = vi.fn(async (_vars: unknown) => {
    mutate(_vars);
    return result;
  });
  return { mutate: mutateAsync, mutateAsync, isPending: false, isSuccess: false, error: null };
}

function makeUseIngestText(result: unknown, mutate = vi.fn()) {
  const mutateAsync = vi.fn(async (_vars: unknown) => {
    mutate(_vars);
    return result;
  });
  return { mutate: mutateAsync, mutateAsync, isPending: false, isSuccess: false, error: null };
}

function makeUseChoice(mutate = vi.fn()) {
  const mutateAsync = vi.fn(async (_vars: unknown) => {
    mutate(_vars);
    return { id: 'd1' };
  });
  return { mutate: mutateAsync, mutateAsync, isPending: false, isSuccess: false, error: null };
}

function mockSuccessMutations(): void {
  vi.mocked(docHooks.useDeleteDocument).mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
  } as never);
  vi.mocked(docHooks.useReprocessDocument).mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
  } as never);
  vi.mocked(apiHooks.usePatchAsset).mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
  } as never);
}

/**
 * Open the "…" actions menu of the row containing `filename`.
 *
 * The Radix DropdownMenu opens on pointerDown (the trigger's `onPointerDown`).
 * In jsdom the portal content + Radix's focus-management / pointer-grace timers
 * render on the macrotask queue, so we flush the macrotasks FIRST (menu is
 * mounted and Radix's timers settle) and then wait for the Delete item with
 * `findByRole` (retries until the menuitem is present) rather than a fixed
 * `waitFor` over a non-retrying assertion. A single `pointerDown` is used — a
 * follow-up `click` would toggle the already-open menu back closed.
 */
async function openRowMenu(filename: string): Promise<void> {
  const row = screen.getByText(filename).closest('tr');
  const trigger = row?.querySelector('button');
  expect(trigger).not.toBeNull();
  fireEvent.pointerDown(trigger as Element);
  await flushMacrotasks(500);
  await screen.findByRole('menuitem', { name: /delete/i }, { timeout: 4000 });
}

/**
 * Click a menu item by name. `userEvent.click` is used (not `fireEvent.click`)
 * because Radix's `onSelect` is driven by the full pointer/keyboard sequence
 * (`pointerdown` → `pointerup` → `click`), which `fireEvent.click` alone does
 * not reproduce reliably in jsdom.
 */
async function clickMenu(user: ReturnType<typeof userEvent.setup>, name: string | RegExp): Promise<void> {
  await user.click(await screen.findByRole('menuitem', { name }, { timeout: 4000 }));
}

describe('DocumentsPage', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    vi.mocked(sonner.toast).mockReset();
    mockSuccessMutations();
  });

  it('renders loading state', () => {
    mockDocuments(undefined, true);
    renderPage();
    expect(screen.getByText(/loading/i)).toBeDefined();
  });

  it('renders error state', () => {
    vi.mocked(docHooks.useDocuments).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Network error'),
    } as never);
    renderPage();
    expect(screen.getByText(/failed to load documents/i)).toBeDefined();
  });

  it('renders rows with filename + upload date and is axe-clean', async () => {
    mockDocuments([
      makeRow({ id: 'd1', source_filename: 'invoice_microwave.pdf', source_uploaded_at: '2026-08-01T10:00:00Z' }),
      makeRow({ id: 'd2', source_filename: 'warranty_washer.pdf', source_uploaded_at: '2026-08-02T10:00:00Z', status: 'failed' }),
    ]);
    const { container } = renderPage();

    expect(screen.getByText('invoice_microwave.pdf')).toBeInTheDocument();
    expect(screen.getByText('2026-08-01')).toBeInTheDocument();
    expect(screen.getByText('warranty_washer.pdf')).toBeInTheDocument();
    expect(screen.getByText('2026-08-02')).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Filename' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Uploaded' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Status' })).toBeInTheDocument();

    expect(await axe(container)).toHaveNoViolations();
  });

  it('renders a status badge for each of the four statuses', () => {
    mockDocuments([
      makeRow({ id: 'd1', status: 'processed' }),
      makeRow({ id: 'd2', status: 'in_review' }),
      makeRow({ id: 'd3', status: 'failed' }),
      makeRow({ id: 'd4', status: 'asset_less' }),
    ]);
    renderPage();
    expect(screen.getByText('processed')).toBeInTheDocument();
    expect(screen.getByText('in review')).toBeInTheDocument();
    expect(screen.getByText('failed')).toBeInTheDocument();
    expect(screen.getByText('no asset')).toBeInTheDocument();
  });

  it('processed row with a linked asset shows an asset link to /assets/{id}', () => {
    mockDocuments([
      makeRow({ id: 'd1', status: 'processed', asset_id: 'a1', source_filename: 'invoice.pdf' }),
      makeRow({ id: 'd2', status: 'asset_less', asset_id: null, source_filename: 'note.txt' }),
    ]);
    renderPage();

    const link = screen.getByRole('link', { name: 'View asset' });
    expect(link).toHaveAttribute('href', '/assets/a1');
    // The unlinked row shows the muted dash, not a link.
    expect(screen.queryAllByRole('link', { name: 'View asset' })).toHaveLength(1);
    const unlinkedRow = screen.getByText('note.txt').closest('tr');
    expect(unlinkedRow?.textContent).toContain('—');
  });

  it('status filter: selecting a status calls useDocuments with that status', async () => {
    mockDocuments([makeRow()]);
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole('combobox', { name: 'Status' }));
    await user.click(await screen.findByRole('option', { name: 'failed' }));

    await waitFor(() => {
      expect(docHooks.useDocuments).toHaveBeenLastCalledWith({ status: 'failed', q: undefined });
    });
  });

  it('filename search: typing passes q to useDocuments', async () => {
    mockDocuments([makeRow()]);
    const user = userEvent.setup();
    renderPage();

    await user.type(screen.getByRole('searchbox'), 'wash');
    await waitFor(() => {
      expect(docHooks.useDocuments).toHaveBeenLastCalledWith({ status: undefined, q: 'wash' });
    });
  });

  it('empty state renders the CTA linking to /', () => {
    mockDocuments([]);
    renderPage();
    expect(screen.getByText(/no documents found/i)).toBeInTheDocument();
    const cta = screen.getByRole('link', { name: /add something/i });
    expect(cta).toHaveAttribute('href', '/');
  });

  it('processed row menu offers Edit asset + Open asset and NO Reprocess', async () => {
    mockDocuments([
      makeRow({ id: 'd1', status: 'processed', asset_id: 'a1', source_filename: 'invoice.pdf' }),
    ]);
    renderPage();

    await openRowMenu('invoice.pdf');
    expect(screen.getByRole('menuitem', { name: /edit asset/i })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: /open asset/i })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: /delete/i })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: /reprocess/i })).not.toBeInTheDocument();
  }, 60000);

  it('failed row menu offers Reprocess + Delete and no asset items', async () => {
    mockDocuments([
      makeRow({ id: 'd4', status: 'failed', asset_id: null, source_filename: 'scanned.pdf' }),
    ]);
    renderPage();

    await openRowMenu('scanned.pdf');
    expect(screen.getByRole('menuitem', { name: /^reprocess$/i })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: /delete/i })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: /open asset/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: /edit asset/i })).not.toBeInTheDocument();
  }, 60000);

  it('in_review row menu shows Reprocess disabled + Delete', async () => {
    mockDocuments([
      makeRow({ id: 'd3', status: 'in_review', asset_id: null, source_filename: 'pending.jpg' }),
    ]);
    renderPage();

    await openRowMenu('pending.jpg');
    const reprocess = screen.getByRole('menuitem', { name: /reprocess \(in review/i });
    // Radix marks a disabled menu item with `aria-disabled` (not the `disabled`
    // attribute), so assert on the ARIA flag rather than `toBeDisabled()`.
    expect(reprocess).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByRole('menuitem', { name: /delete/i })).toBeInTheDocument();
  }, 60000);

  it('Delete: confirm dialog → confirm → useDeleteDocument called with the id + toast', async () => {
    const user = userEvent.setup();
    const deleteAsync = vi.fn().mockResolvedValue(undefined);
    vi.mocked(docHooks.useDeleteDocument).mockReturnValue({
      mutateAsync: deleteAsync,
      isPending: false,
    } as never);
    mockDocuments([
      makeRow({ id: 'd4', status: 'failed', source_filename: 'scanned.pdf' }),
    ]);
    renderPage();

    await openRowMenu('scanned.pdf');
    await clickMenu(user, /delete/i);
    // ConfirmDialog is built on Radix AlertDialog → role="alertdialog".
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText(/will be removed/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await waitFor(() => {
      expect(deleteAsync).toHaveBeenCalledWith({ documentId: 'd4' });
    });
    expect(sonner.toast).toHaveBeenCalled();
  }, 60000);

  it('Reprocess (failed row): comment dialog → submit with a comment → useReprocessDocument called', async () => {
    const user = userEvent.setup();
    const reprocessAsync = vi.fn().mockResolvedValue({});
    vi.mocked(docHooks.useReprocessDocument).mockReturnValue({
      mutateAsync: reprocessAsync,
      isPending: false,
    } as never);
    mockDocuments([
      makeRow({ id: 'd4', status: 'failed', source_filename: 'scanned.pdf' }),
    ]);
    renderPage();

    await openRowMenu('scanned.pdf');
    await clickMenu(user, /^reprocess$/i);
    const comment = await screen.findByPlaceholderText(/add a note/i);
    await user.clear(comment);
    await user.type(comment, 'it is a microwave');
    fireEvent.click(screen.getByRole('button', { name: /^reprocess$/i }));

    await waitFor(() => {
      expect(reprocessAsync).toHaveBeenCalledWith({
        documentId: 'd4',
        comment: 'it is a microwave',
      });
    });
    expect(sonner.toast).toHaveBeenCalledWith('Reprocessing document');
  }, 60000);

  it('Reprocess (failed row): empty note → comment omitted from the call', async () => {
    const user = userEvent.setup();
    const reprocessAsync = vi.fn().mockResolvedValue({});
    vi.mocked(docHooks.useReprocessDocument).mockReturnValue({
      mutateAsync: reprocessAsync,
      isPending: false,
    } as never);
    mockDocuments([
      makeRow({ id: 'd4', status: 'failed', source_filename: 'scanned.pdf' }),
    ]);
    renderPage();

    await openRowMenu('scanned.pdf');
    await clickMenu(user, /^reprocess$/i);
    await screen.findByPlaceholderText(/add a note/i);
    fireEvent.click(screen.getByRole('button', { name: /^reprocess$/i }));

    await waitFor(() => {
      expect(reprocessAsync).toHaveBeenCalledWith({ documentId: 'd4', comment: undefined });
    });
  }, 60000);

  it('Reprocess 409: friendly toast, dialog stays open', async () => {
    const user = userEvent.setup();
    const reprocessAsync = vi
      .fn()
      .mockRejectedValue(new ApiError(409, 'This clashes with the current state.', 'reprocess in flight'));
    vi.mocked(docHooks.useReprocessDocument).mockReturnValue({
      mutateAsync: reprocessAsync,
      isPending: false,
    } as never);
    mockDocuments([
      makeRow({ id: 'd4', status: 'failed', source_filename: 'scanned.pdf' }),
    ]);
    renderPage();

    await openRowMenu('scanned.pdf');
    await clickMenu(user, /^reprocess$/i);
    await screen.findByPlaceholderText(/add a note/i);
    fireEvent.click(screen.getByRole('button', { name: /^reprocess$/i }));

    await waitFor(() => {
      expect(sonner.toast).toHaveBeenCalledWith(
        'Already reprocessing',
        expect.objectContaining({ description: expect.stringMatching(/already/i) }),
      );
    });
  }, 60000);

  it('Edit asset (processed row): dialog → save → usePatchAsset called with assetId + body', async () => {
    const user = userEvent.setup();
    const patchAsync = vi.fn().mockResolvedValue({});
    vi.mocked(apiHooks.usePatchAsset).mockReturnValue({
      mutateAsync: patchAsync,
      isPending: false,
    } as never);
    mockDocuments([
      makeRow({ id: 'd1', status: 'processed', asset_id: 'a1', source_filename: 'invoice.pdf' }),
    ]);
    renderPage();

    await openRowMenu('invoice.pdf');
    await clickMenu(user, /edit asset/i);

    const name = await screen.findByLabelText('Asset name');
    await user.type(name, 'Microwave Oven');
    fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

    await waitFor(() => {
      expect(patchAsync).toHaveBeenCalledWith({
        assetId: 'a1',
        body: { name: 'Microwave Oven' },
      });
    });
    expect(sonner.toast).toHaveBeenCalledWith('Asset updated');
  }, 60000);

  it('Open asset navigates to /assets/{id}', async () => {
    const user = userEvent.setup();
    mockDocuments([
      makeRow({ id: 'd1', status: 'processed', asset_id: 'a1', source_filename: 'invoice.pdf' }),
    ]);
    renderPage();

    await openRowMenu('invoice.pdf');
    await clickMenu(user, /open asset/i);
    expect(mockNavigate).toHaveBeenCalledWith('/assets/a1');
  }, 60000);
});

describe('DocumentsPage — [+] → AddComposer (task 14.6)', () => {
  beforeEach(() => {
    mockSuccessMutations();
    vi.clearAllMocks();
    vi.mocked(composerHooks.useIngestFile).mockReturnValue(makeUseIngestFile({ kind: 'committed', asset: {} }) as never);
    vi.mocked(composerHooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    vi.mocked(composerHooks.useKeepDocument).mockReturnValue(makeUseChoice() as never);
    vi.mocked(composerHooks.useReprocessDocument).mockReturnValue(makeUseChoice() as never);
  });

  it('renders the top-right [+] (Add document) and opens the shared composer', async () => {
    mockDocuments([makeRow()]);
    const user = userEvent.setup();
    renderPage();
    const addBtn = screen.getByRole('button', { name: 'Add document' });
    expect(addBtn).toBeInTheDocument();
    // Composer closed initially.
    expect(screen.queryByRole('button', { name: 'Upload a file' })).not.toBeInTheDocument();
    await user.click(addBtn);
    // Composer opened: upload surface + optional note appear.
    expect(await screen.findByRole('button', { name: 'Upload a file' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: /note/i })).toBeInTheDocument();
  });

  it('submitting a duplicate upload opens the reprocess/keep prompt', async () => {
    const fileHook = makeUseIngestFile({ kind: 'duplicate', report: DUPLICATE_REPORT });
    vi.mocked(composerHooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(composerHooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    vi.mocked(composerHooks.useKeepDocument).mockReturnValue(makeUseChoice() as never);
    vi.mocked(composerHooks.useReprocessDocument).mockReturnValue(makeUseChoice() as never);
    mockDocuments([makeRow()]);
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole('button', { name: 'Add document' }));
    await pickFile('[data-testid="composer-file-input"]', makeFile('photo.jpg', 'image/jpeg'));
    const addBtn = await screen.findByRole('button', { name: 'Add' });
    await user.click(addBtn);
    expect(await screen.findByText('Duplicate detected')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /reprocess/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /keep existing/i })).toBeInTheDocument();
  });
});
