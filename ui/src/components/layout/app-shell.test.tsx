/**
 * Task 3.2 — app shell, router, and shared feedback (design D7/D9).
 *
 * Verifies (per the task):
 *  - nav to all six screens without a reload (client-side routing)
 *  - deep-link render of `/assets/:id` (and `/finance/import/:batchId`)
 *  - mobile nav reachable via the Sheet
 *  - ConfirmDialog confirm/cancel
 * plus: active-user switcher applies, not-found route, feedback components,
 * and axe-clean rendering of the shell, the open sheet, and the open dialog.
 */
import { useState } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';
import { AppRoutes } from '@/router';
import { ActiveUserProvider } from '@/context/active-user';
import { ConfirmDialog } from '@/components/confirm-dialog';
import * as hooks from '@/lib/api/hooks';

vi.mock('@/lib/api/hooks', () => ({
  useAssets: vi.fn(),
  useAsset: vi.fn(),
  useAssetDocuments: vi.fn(),
  useMovements: vi.fn(),
  useCreateMovement: vi.fn(),
  useBatches: vi.fn(),
  useBatch: vi.fn(),
  useCommitBatch: vi.fn(),
  useUploadStatement: vi.fn(),
  useDiscardBatch: vi.fn(),
  useAccounts: vi.fn(),
  useCreateAccount: vi.fn(),
  useQuickSearch: vi.fn(),
  useSearch: vi.fn(),
  useReviews: vi.fn(),
  useApproveReview: vi.fn(),
  useRejectReview: vi.fn(),
  useAdd: vi.fn(),
  useRestoreAsset: vi.fn(),
}));
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { Loading } from '@/components/feedback/loading';

function renderApp(initialPath = '/'): ReturnType<typeof render> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <ActiveUserProvider queryClient={queryClient}>
          <AppRoutes />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** Caller-side harness: a trigger button that opens the shared dialog. */
function ConfirmHarness() {
  const [open, setOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  return (
    <div>
      <Button onClick={() => setOpen(true)}>Open delete dialog</Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title="Delete movement?"
        description="The movement is removed permanently. This cannot be undone."
        confirmLabel="Delete movement"
        destructive
        onConfirm={() => {
          setConfirmed(true);
          setOpen(false);
        }}
      />
      {confirmed ? <p role="status">Movement deleted</p> : null}
    </div>
  );
}

describe('app shell + router', () => {
  beforeEach(() => {
    vi.mocked(hooks.useAssets).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(hooks.useAsset).mockReturnValue({ data: { id: 'inv-42', doc_type: 'invoice' }, isLoading: false, error: null } as any);
    vi.mocked(hooks.useAssetDocuments).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(hooks.useAccounts).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(hooks.useCreateAccount).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    // MovementsPage (task 4.5) consumes useMovements; keep the nav-reachability
    // render from throwing when no user is active.
    vi.mocked(hooks.useMovements).mockReturnValue({ data: [], isLoading: false } as any);
    vi.mocked(hooks.useCreateMovement).mockReturnValue({ isPending: false } as any);
    vi.mocked(hooks.useBatches).mockReturnValue({ data: [], isLoading: false, isError: false } as any);
    vi.mocked(hooks.useBatch).mockImplementation((batchId: string) => {
      if (!batchId) return { data: null, isLoading: false, isError: false } as any;
      return { data: { id: 'batch-7' }, isLoading: false, isError: false } as any;
    });
    vi.mocked(hooks.useUploadStatement).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(hooks.useDiscardBatch).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(hooks.useCommitBatch).mockReturnValue({ mutate: vi.fn(), isPending: false, isSuccess: false, data: undefined } as any);
    vi.mocked(hooks.useQuickSearch).mockReturnValue({ data: { results: [] }, isLoading: false } as any);
    vi.mocked(hooks.useReviews).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(hooks.useApproveReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(hooks.useRejectReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    // AddPage (task 8.2) consumes useAdd + useRestoreAsset; keep the nav
    // "reaches every primary screen" render from throwing when no user is active.
    vi.mocked(hooks.useAdd).mockReturnValue({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, isSuccess: false, data: undefined } as any);
    vi.mocked(hooks.useRestoreAsset).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  });
  it('redirects "/" to the asset list without a reload', () => {
    renderApp('/');
    expect(screen.getByRole('heading', { level: 1, name: 'Assets' })).toBeInTheDocument();
  });

  it('reaches every primary screen from the nav without a reload', () => {
    renderApp('/assets');
    const steps: ReadonlyArray<readonly [link: string, heading: string]> = [
      ['Add', 'Add'],
      ['Accounts', 'Accounts'],
      ['Movements', 'Money movements'],
      ['Import', 'Import Statement'],
      ['Import history', 'No import history'],
    ];
    for (const [link, heading] of steps) {
      fireEvent.click(screen.getByRole('link', { name: link }));
      expect(screen.getByRole('heading', { name: heading })).toBeInTheDocument();
    }
    // and back to the asset list
    fireEvent.click(screen.getByRole('link', { name: 'Assets' }));
    expect(screen.getByRole('heading', { level: 1, name: 'Assets' })).toBeInTheDocument();
  });

  it('deep-links /assets/:id to the asset detail screen', () => {
    renderApp('/assets/inv-42');
    expect(screen.getByRole('heading', { level: 1, name: /Asset/i })).toBeInTheDocument();
  });

  it('deep-links /finance/import/:batchId to the batch detail screen', () => {
    renderApp('/finance/import/batch-7');
    expect(screen.getByRole('heading', { level: 1, name: 'Batch detail' })).toBeInTheDocument();
  });

  it('renders a not-found state for unknown routes with a way back', () => {
    renderApp('/no/such/page');
    expect(screen.getByRole('heading', { level: 1, name: 'Page not found' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('link', { name: 'Go to assets' }));
    expect(screen.getByRole('heading', { level: 1, name: 'Assets' })).toBeInTheDocument();
  });

  it('navigates via the mobile sheet and closes the sheet on navigation', async () => {
    renderApp('/assets');
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation menu' }));
    const sheet = await screen.findByRole('dialog', { name: 'Navigation' });
    fireEvent.click(within(sheet).getByRole('link', { name: 'Movements' }));
    expect(screen.getByRole('heading', { level: 1, name: 'Money movements' })).toBeInTheDocument();
    expect(screen.queryByRole('dialog', { name: 'Navigation' })).not.toBeInTheDocument();
  });

  it('shows the active user switcher and applies a user switch', async () => {
    renderApp('/assets');
    const input = screen.getByLabelText('Active user', { selector: '#user-switcher-sidebar' });
    fireEvent.change(input, { target: { value: 'bob' } });
    const form = input.closest('form');
    expect(form).not.toBeNull();
    fireEvent.click(within(form as HTMLElement).getByRole('button', { name: 'Set active user' }));
    await waitFor(() => expect(localStorage.getItem('activeUser')).toBe('bob'));
    expect((input as HTMLInputElement).value).toBe('bob');
  });

  it('is axe-clean on the shell', async () => {
    const { container } = renderApp('/assets');
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });

  it('is axe-clean with the mobile sheet open', async () => {
    renderApp('/assets');
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation menu' }));
    const sheet = await screen.findByRole('dialog', { name: 'Navigation' });
    const results = await axe.run(sheet);
    expect(results).toHaveNoViolations();
  });

  it('is axe-clean with the confirm dialog open', async () => {
    render(<ConfirmHarness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open delete dialog' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete movement?' });
    const results = await axe.run(dialog);
    expect(results).toHaveNoViolations();
  });
});

describe('ConfirmDialog', () => {
  it('confirm runs the action and closes the dialog', async () => {
    render(<ConfirmHarness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open delete dialog' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete movement?' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete movement' }));
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Movement deleted'),
    );
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('cancel closes the dialog without running the action', async () => {
    render(<ConfirmHarness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open delete dialog' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete movement?' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(screen.queryByText('Movement deleted')).not.toBeInTheDocument();
  });
});

describe('shared feedback components', () => {
  it('ErrorState shows the message and offers retry', () => {
    const onRetry = vi.fn();
    render(<ErrorState message="The server could not be reached." onRetry={onRetry} />);
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText('The server could not be reached.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('Loading announces a status and EmptyState guides the user', () => {
    render(
      <div>
        <Loading label="Loading assets" />
        <EmptyState title="No assets yet" description="Upload a document to create your first asset.">
          <Button>Upload a document</Button>
        </EmptyState>
      </div>,
    );
    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.getByText('Loading assets…')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'No assets yet' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Upload a document' })).toBeInTheDocument();
  });

  it('feedback states are axe-clean', async () => {
    const { container } = render(
      <div>
        <Loading label="Loading" />
        <ErrorState message="Something failed." onRetry={() => undefined} />
        <EmptyState title="Nothing here yet" />
      </div>,
    );
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
