/**
 * Task 3.2 — app shell, router, and shared feedback (design D7/D9).
 * Task 13.2 — chrome rewrite: no top bar; sole chrome is a fixed top-left
 * floating ☰ on every view.
 *
 * Verifies (per the task):
 *  - nav to all six screens without a reload (client-side routing)
 *  - deep-link render of `/assets/:id`
 *  - mobile nav reachable via the Sheet
 *  - ConfirmDialog confirm/cancel
 *  - no top bar (header/banner) and the fixed top-left ☰ chrome (task 13.2)
 * plus: active-user switcher applies, not-found route, feedback components,
 * and axe-clean rendering of the shell, the open sheet, and the open dialog.
 */
import { useState } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';
import { AppRoutes } from '@/router';
import { ActiveUserProvider } from '@/context/active-user';
import { ConfirmDialog } from '@/features/docs/confirm-dialog';
import * as hooks from '@/lib/api/hooks';
import * as docsHooks from '@/features/docs/hooks';
import * as assetsHooks from '@/features/assets/use-asset';
import * as financeHooks from '@/features/finance/hooks';
import * as reviewsHooks from '@/features/reviews/hooks';

vi.mock('@/lib/api/hooks', () => ({
  useQuickSearch: vi.fn(),
}));

vi.mock('@/features/docs/hooks', () => ({
  useAssets: vi.fn(),
  useAssetDocuments: vi.fn(),
  useAccounts: vi.fn(),
}));

vi.mock('@/features/assets/use-asset', () => ({
  useAsset: vi.fn(),
}));

vi.mock('@/features/finance/hooks', () => ({
  useCreateAccount: vi.fn(),
}));

vi.mock('@/features/search/use-search', () => ({
  useSearch: vi.fn(),
}));

vi.mock('@/features/reviews/hooks', () => ({
  useReviews: vi.fn(),
  useApproveReview: vi.fn(),
  useRejectReview: vi.fn(),
}));

vi.mock('@/features/docs/composer/hooks', () => ({
  useIngestFile: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
  useIngestText: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
  useReprocessDocument: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
  useKeepDocument: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
}));

// The app shell mounts the sonner Toaster (task 7.3); mock it so no real
// portal/timers run in jsdom.
vi.mock('sonner', () => ({ Toaster: () => null, toast: vi.fn() }));
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

/**
 * Task 13.2 chrome assertions for the fixed top-left floating ☰. jsdom does
 * not resolve Tailwind utility classes to computed styles or layout (boxes are
 * 0×0), so the positioning/sizing contract is asserted via the class list
 * (`fixed left-4 top-4` for placement, `size-12` = 48px for the ≥44px hit
 * target) — same convention as `theme-toggle.test.tsx`.
 */
function expectChromeButton(trigger: HTMLElement): void {
  const classes = trigger.className ?? '';
  expect(classes).toContain('fixed');
  // Top-left placement (fixed at top-4/left-4 = 16px inset).
  expect(classes).toContain('left-4');
  expect(classes).toContain('top-4');
  // z-40: above content (z-0) and the landing chat pill (z-30), below the
  // nav sheet scrim/panel (z-50).
  expect(classes).toContain('z-40');
  // Hit target ≥ 44×44: size-12 = 48px.
  expect(classes).toContain('size-12');
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
    vi.mocked(docsHooks.useAssets).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(assetsHooks.useAsset).mockReturnValue({ data: { id: 'inv-42', data: { doc_type: 'invoice', metadata: {} }, created_at: '', updated_at: '' }, isLoading: false, error: null } as any);
    vi.mocked(docsHooks.useAssetDocuments).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(docsHooks.useAccounts).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(financeHooks.useCreateAccount).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(hooks.useQuickSearch).mockReturnValue({ data: { results: [] }, isLoading: false } as any);
    vi.mocked(reviewsHooks.useReviews).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(reviewsHooks.useApproveReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(reviewsHooks.useRejectReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  });

  it('renders the landing page at "/" without redirecting to Assets', () => {
    renderApp('/');
    // The landing page renders directly at / (task 7.2) — no redirect to /assets.
    expect(screen.getByRole('heading', { name: 'Insights' })).toBeInTheDocument();
    expect(screen.getByRole('searchbox', { name: /search/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add something' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { level: 1, name: 'Assets' })).not.toBeInTheDocument();
  });

  it('shows the hamburger trigger on the landing page at /', () => {
    renderApp('/');
    // The hamburger is mounted by AppShell (not the landing page itself) and is
    // present on every route, including the landing page.
    expect(screen.getByRole('button', { name: 'Open navigation' })).toBeInTheDocument();
    // …and the landing page's regions render alongside it.
    expect(screen.getByRole('heading', { name: 'Insights' })).toBeInTheDocument();
  });

  it('has no top bar on / or /assets', () => {
    renderApp('/assets');
    // The old sticky <header> top bar is gone (task 13.2).
    expect(document.querySelector('header')).toBeNull();
    // A top-level <header> maps to the banner landmark.
    expect(screen.queryByRole('banner')).not.toBeInTheDocument();

    renderApp('/');
    expect(document.querySelector('header')).toBeNull();
    expect(screen.queryByRole('banner')).not.toBeInTheDocument();
  });

  it('☰ is a fixed top-left floating button (≥44px) on /', () => {
    renderApp('/');
    const trigger = screen.getByRole('button', { name: 'Open navigation' });
    expectChromeButton(trigger);
  });

  it('☰ is a fixed top-left floating button (≥44px) on /assets', () => {
    renderApp('/assets');
    const trigger = screen.getByRole('button', { name: 'Open navigation' });
    expectChromeButton(trigger);
  });

  it('reaches every primary screen from the nav sheet without a reload', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });

    // Open sheet, click Accounts
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation' }));
    let sheet = await screen.findByRole('dialog', { name: 'Menu' });
    fireEvent.click(within(sheet).getByRole('link', { name: 'Accounts' }));
    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 1, name: 'Accounts' })).toBeInTheDocument(),
    );

    // Open sheet again, click Assets to go back
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation' }));
    sheet = await screen.findByRole('dialog', { name: 'Menu' });
    fireEvent.click(within(sheet).getByRole('link', { name: 'Assets' }));
    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 1, name: 'Assets' })).toBeInTheDocument(),
    );
  });

  it('deep-links /assets/:id to the asset detail screen', () => {
    renderApp('/assets/inv-42');
    expect(screen.getByRole('heading', { level: 1, name: /Asset/i })).toBeInTheDocument();
  });

  it('renders a not-found state for unknown routes with a way back', () => {
    renderApp('/no/such/page');
    expect(screen.getByRole('heading', { level: 1, name: 'Page not found' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('link', { name: 'Go to assets' }));
    expect(screen.getByRole('heading', { level: 1, name: 'Assets' })).toBeInTheDocument();
  });

  it('navigates via the nav sheet and closes the sheet on navigation', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation' }));
    const sheet = await screen.findByRole('dialog', { name: 'Menu' });
    fireEvent.click(within(sheet).getByRole('link', { name: 'Accounts' }));
    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 1, name: 'Accounts' })).toBeInTheDocument(),
    );
    expect(screen.queryByRole('dialog', { name: 'Menu' })).not.toBeInTheDocument();
  });

  it('shows the active user switcher in the nav sheet and applies a user switch', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation' }));
    const sheet = await screen.findByRole('dialog', { name: 'Menu' });
    // Open the profile panel (tapping the profile row)
    const profileBtn = within(sheet).getByRole('button', { name: /Choose a user/ });
    fireEvent.click(profileBtn);
    const input = await screen.findByLabelText('Active user', { selector: '#user-switcher-profile' });
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

  it('is axe-clean with the nav sheet open', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    fireEvent.click(screen.getByRole('button', { name: 'Open navigation' }));
    const sheet = await screen.findByRole('dialog', { name: 'Menu' });
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
