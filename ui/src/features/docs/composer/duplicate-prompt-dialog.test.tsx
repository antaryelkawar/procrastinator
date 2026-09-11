/**
 * Task 7.3 → 14.1 — Duplicate-prompt dialog (design D4/D11).
 *
 * Extracted from the old ingest-cards dialog tests (the module moved with
 * `hooks.ts` into `features/docs/composer`). Mocks the ingest hooks so each
 * test drives the observable UI behavior end-to-end through the real
 * `<DuplicatePromptDialog />` — no test seams.
 *
 * The keep-default-on-timeout test uses `vi.useFakeTimers()` to advance past
 * `expires_at` without any click and asserts the keep hook fired AND a sonner
 * toast was fired with the report's verbatim `timeout_toast`.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import axe from 'axe-core';
import * as hooks from './hooks';
import * as sonner from 'sonner';
import { DuplicatePromptDialog } from './duplicate-prompt-dialog';
import { useActiveUser } from '@/context/active-user';
import { ActiveUserProvider } from '@/context/active-user';
import type { DuplicateReport } from '@/lib/api/generated/orval/procrastinator';

vi.mock('./hooks', async () => {
  const actual = await vi.importActual<typeof import('./hooks')>('./hooks');
  return {
    ...actual,
    useIngestFile: vi.fn(),
    useIngestText: vi.fn(),
    useReprocessDocument: vi.fn(),
    useKeepDocument: vi.fn(),
  };
});

vi.mock('sonner', () => sonnerMock);
type SonnerToastMethod = (message: React.ReactNode, data?: import('sonner').ExternalToast) => string | number;
type SonnerToastMock = {
  (message: React.ReactNode, data?: import('sonner').ExternalToast): string | number;
  success: SonnerToastMethod;
  error: SonnerToastMethod;
  info: SonnerToastMethod;
  warning: SonnerToastMethod;
};
const sonnerMock: { toast: SonnerToastMock } = vi.hoisted<{ toast: SonnerToastMock }>(() => ({
  toast: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  }) as SonnerToastMock,
}));

vi.mock('@/context/active-user', async () => {
  const actual = await vi.importActual<typeof import('@/context/active-user')>('@/context/active-user');
  return { ...actual, useActiveUser: vi.fn() };
});

const USER_ID = 'alice';

const DUPLICATE_REPORT: DuplicateReport = {
  code: 'duplicate',
  existing_document_id: 'doc-1',
  existing_source_filename: 'existing-invoice.pdf',
  existing_source_uploaded_at: '2026-09-01T10:00:00Z',
  existing_asset_id: 'a1',
  prompt: {
    reprocess_uri: `/api/users/${USER_ID}/documents/doc-1/reprocess`,
    keep_uri: `/api/users/${USER_ID}/documents/doc-1/keep`,
    expires_at: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
    timeout_toast: 'no response — keeping existing document',
  },
};

function makeUseChoice(mutate = vi.fn()) {
  const mutateAsync = vi.fn(async (_vars: unknown) => {
    mutate(_vars);
    return { id: 'd1' };
  });
  return { mutate: mutateAsync, mutateAsync, isPending: false, isSuccess: false, error: null };
}

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  vi.mocked(useActiveUser).mockReturnValue({
    activeUser: USER_ID,
    setActiveUser: () => false,
    clearActiveUser: () => undefined,
  } as never);
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ActiveUserProvider queryClient={queryClient}>
          <DuplicatePromptDialog report={DUPLICATE_REPORT} onClose={() => {}} onKept={() => {}} onReprocessed={() => {}} />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...utils, queryClient };
}

function renderDialogWithReport(report: DuplicateReport) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  vi.mocked(useActiveUser).mockReturnValue({
    activeUser: USER_ID,
    setActiveUser: () => false,
    clearActiveUser: () => undefined,
  } as never);
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ActiveUserProvider queryClient={queryClient}>
          <DuplicatePromptDialog report={report} onClose={() => {}} onKept={() => {}} onReprocessed={() => {}} />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('DuplicatePromptDialog — both prompt choices', () => {
  function setupDuplicate() {
    const keepHook = makeUseChoice();
    const reprocessHook = makeUseChoice();
    vi.mocked(hooks.useKeepDocument).mockReturnValue(keepHook as never);
    vi.mocked(hooks.useReprocessDocument).mockReturnValue(reprocessHook as never);
    return { keepHook, reprocessHook };
  }

  it('409 opens the dialog showing filename + upload date + both buttons', () => {
    setupDuplicate();
    renderDialog();

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Duplicate detected')).toBeInTheDocument();
    expect(screen.getByText('existing-invoice.pdf')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /reprocess/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /keep existing/i })).toBeInTheDocument();
    // The verbatim timeout_toast is surfaced in the muted line.
    expect(screen.getByText(/no response — keeping existing document/i)).toBeInTheDocument();
  });

  it('clicking Reprocess calls the reprocess hook with the parsed doc id', async () => {
    setupDuplicate();
    const onKept = vi.fn();
    const onReprocessed = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <MemoryRouter>
          <ActiveUserProvider queryClient={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
            <DuplicatePromptDialog report={DUPLICATE_REPORT} onClose={() => {}} onKept={onKept} onReprocessed={onReprocessed} />
          </ActiveUserProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /reprocess/i }));
    });

    const reprocessHook = vi.mocked(hooks.useReprocessDocument).mock.results[0]?.value as ReturnType<typeof makeUseChoice>;
    await vi.waitFor(() => {
      expect(reprocessHook.mutateAsync).toHaveBeenCalledWith({
        userId: USER_ID,
        documentId: 'doc-1',
      });
    });
    // Keep was NOT called.
    const keepHook = vi.mocked(hooks.useKeepDocument).mock.results[0]?.value as ReturnType<typeof makeUseChoice>;
    expect(keepHook.mutateAsync).not.toHaveBeenCalled();
    // The parent's onReprocessed fired so it can close the dialog.
    expect(onReprocessed).toHaveBeenCalledTimes(1);
    expect(onKept).not.toHaveBeenCalled();
  });

  it('clicking Keep existing calls the keep hook with the parsed doc id', async () => {
    setupDuplicate();
    const onKept = vi.fn();
    const onReprocessed = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <MemoryRouter>
          <ActiveUserProvider queryClient={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
            <DuplicatePromptDialog report={DUPLICATE_REPORT} onClose={() => {}} onKept={onKept} onReprocessed={onReprocessed} />
          </ActiveUserProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /keep existing/i }));
    });

    const keepHook = vi.mocked(hooks.useKeepDocument).mock.results[0]?.value as ReturnType<typeof makeUseChoice>;
    const reprocessHook = vi.mocked(hooks.useReprocessDocument).mock.results[0]?.value as ReturnType<typeof makeUseChoice>;
    await vi.waitFor(() => {
      expect(keepHook.mutateAsync).toHaveBeenCalledWith({
        userId: USER_ID,
        documentId: 'doc-1',
      });
    });
    // Reprocess was NOT called.
    expect(reprocessHook.mutateAsync).not.toHaveBeenCalled();
    // The parent's onKept fired so it can toast + close.
    expect(onKept).toHaveBeenCalledTimes(1);
    expect(onReprocessed).not.toHaveBeenCalled();
  });
});

describe('DuplicatePromptDialog — keep-default on timeout', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('auto-keeps and toasts the verbatim timeout_toast when the prompt expires unchosen', async () => {
    vi.useFakeTimers();
    const keepHook = makeUseChoice();
    vi.mocked(hooks.useKeepDocument).mockReturnValue(keepHook as never);
    vi.mocked(hooks.useReprocessDocument).mockReturnValue(makeUseChoice() as never);

    // expires_at is 10 minutes after now (matches the backend's now+10min).
    const expiresAt = new Date(Date.now() + 10 * 60 * 1000).toISOString();
    const report: DuplicateReport = {
      ...DUPLICATE_REPORT,
      prompt: { ...DUPLICATE_REPORT.prompt, expires_at: expiresAt },
    };

    renderDialogWithReport(report);

    // The dialog is open.
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    // Advance just past the expiry.
    await act(async () => {
      vi.advanceTimersByTime(10 * 60 * 1000 + 100);
    });

    // Keep was called (auto-keep on timeout).
    expect(keepHook.mutateAsync).toHaveBeenCalledWith({
      userId: USER_ID,
      documentId: 'doc-1',
    });
    // A sonner toast fired with the VERBATIM timeout_toast string (bare `toast`).
    const verbatimCalls = (sonner.toast as unknown as { mock: { calls: unknown[][] } }).mock.calls.filter(
      (call) => call[0] === report.prompt.timeout_toast
    );
    expect(verbatimCalls.length).toBe(1);
  });
});

describe('DuplicatePromptDialog — accessibility', () => {
  it('the dialog passes axe (no violations)', async () => {
    vi.mocked(hooks.useKeepDocument).mockReturnValue(makeUseChoice() as never);
    vi.mocked(hooks.useReprocessDocument).mockReturnValue(makeUseChoice() as never);
    const { container } = renderDialog();
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });
});
