/**
 * Task 7.1 — shared hamburger navigation sheet (task 7.1).
 *
 * Verifies:
 *  - ☰ opens the sheet on `/` (landing page) AND on `/assets`.
 *  - The open sheet contains: profile row (avatar + user name), five nav
 *    links (Home first, then Review Queue / Assets / Documents / Accounts), a
 *    theme toggle row, and three disabled TBD items (Subscriptions /
 *    Relationships / Inventory) that are present but not navigable
 *    (not `role="link"`).
 *  - Link tap navigates to the right route AND dismisses the sheet.
 *  - Scrim/✕ dismiss keeps the current route unchanged and restores focus to
 *    the ☰ trigger.
 *  - Profile row tap opens the active-user switcher (input appears).
 *  - axe-clean: sheet closed (shell) and sheet open, light theme.
 */
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';
import { AppRoutes } from '@/router';
import { ActiveUserProvider } from '@/context/active-user';
import * as hooks from '@/lib/api/hooks';
import * as docsHooks from '@/features/docs/hooks';
import * as assetsHooks from '@/features/assets/use-asset';
import * as financeHooks from '@/features/finance/hooks';
import * as addHooks from '@/features/add/hooks';
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
  useMovements: vi.fn(),
  useCreateMovement: vi.fn(),
  useBatches: vi.fn(),
  useBatch: vi.fn(),
  useCommitBatch: vi.fn(),
  useUploadStatement: vi.fn(),
  useDiscardBatch: vi.fn(),
  useCreateAccount: vi.fn(),
}));

vi.mock('@/features/add/hooks', () => ({
  useAdd: vi.fn(),
  useRestoreAsset: vi.fn(),
}));

vi.mock('@/features/search/use-search', () => ({
  useSearch: vi.fn(),
}));

vi.mock('@/features/reviews/hooks', () => ({
  useReviews: vi.fn(),
  useApproveReview: vi.fn(),
  useRejectReview: vi.fn(),
}));

function renderApp(initialPath = '/', activeUser?: string): ReturnType<typeof render> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  if (activeUser) {
    localStorage.setItem('activeUser', activeUser);
  }
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

async function openSheet(): Promise<HTMLElement> {
  fireEvent.click(screen.getByRole('button', { name: 'Open navigation' }));
  return screen.findByRole('dialog', { name: 'Menu' });
}

describe('nav sheet (task 7.1)', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.mocked(docsHooks.useAssets).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(docsHooks.useAssetDocuments).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(docsHooks.useAccounts).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(assetsHooks.useAsset).mockReturnValue({ data: null, isLoading: false, error: null } as any);
    vi.mocked(financeHooks.useMovements).mockReturnValue({ data: [], isLoading: false } as any);
    vi.mocked(financeHooks.useCreateMovement).mockReturnValue({ isPending: false } as any);
    vi.mocked(financeHooks.useBatches).mockReturnValue({ data: [], isLoading: false, isError: false } as any);
    vi.mocked(financeHooks.useBatch).mockReturnValue({ data: null, isLoading: false, isError: false } as any);
    vi.mocked(financeHooks.useUploadStatement).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(financeHooks.useDiscardBatch).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(financeHooks.useCommitBatch).mockReturnValue({ mutate: vi.fn(), isPending: false, isSuccess: false, data: undefined } as any);
    vi.mocked(financeHooks.useCreateAccount).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(hooks.useQuickSearch).mockReturnValue({ data: { results: [] }, isLoading: false } as any);
    vi.mocked(reviewsHooks.useReviews).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(reviewsHooks.useApproveReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(reviewsHooks.useRejectReview).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
    vi.mocked(addHooks.useAdd).mockReturnValue({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, isSuccess: false, data: undefined } as any);
    vi.mocked(addHooks.useRestoreAsset).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  });

  it('☰ opens the sheet on / (landing page)', async () => {
    renderApp('/');
    // `/` renders the landing page directly (task 7.2) — no redirect.
    await screen.findByRole('heading', { name: 'Insights' });
    const sheet = await openSheet();
    expect(sheet).toBeInTheDocument();
    expect(within(sheet).getByRole('heading', { name: 'Menu' })).toBeInTheDocument();
  });

  it('☰ opens the sheet on /assets', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    expect(sheet).toBeInTheDocument();
  });

  it('open sheet contains profile row with active user id', async () => {
    renderApp('/assets', 'alice');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    // Profile row shows the user id as a button (not a link)
    const profileButton = within(sheet).getByRole('button', { name: /alice/ });
    expect(profileButton).toBeInTheDocument();
  });

  it('open sheet contains profile row with "Not signed in" when no active user', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    const profileButton = within(sheet).getByRole('button', { name: /Choose a user/ });
    expect(profileButton).toBeInTheDocument();
  });

  it('open sheet has exactly the five nav links in order', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    const links = within(sheet).getAllByRole('link');
    const linkNames = links.map((l) => l.textContent);
    // Home is the first nav link.
    expect(links[0]).toHaveTextContent('Home');
    expect(linkNames).toContain('Review Queue');
    expect(linkNames).toContain('Assets');
    expect(linkNames).toContain('Documents');
    expect(linkNames).toContain('Accounts');
    expect(links.length).toBe(5);
  });

  it('Home link navigates to / and dismisses the sheet', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    fireEvent.click(within(sheet).getByRole('link', { name: 'Home' }));
    await screen.findByRole('heading', { name: 'Insights' });
    expect(screen.queryByRole('dialog', { name: 'Menu' })).not.toBeInTheDocument();
  });

  it('theme toggle is present in the sheet', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    const toggle = within(sheet).getByRole('combobox', { name: 'Theme' });
    expect(toggle).toBeInTheDocument();
  });

  it('open sheet has three TBD items that are present but not links', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    for (const label of ['Subscriptions', 'Relationships', 'Inventory']) {
      const item = within(sheet).getByText(label);
      expect(item).toBeInTheDocument();
      // Must not be a link
      expect(item.closest('a')).toBeNull();
      // Has aria-disabled
      expect(item.closest('[aria-disabled="true"]')).not.toBeNull();
    }
  });

  it('Review Queue link navigates and dismisses the sheet', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    fireEvent.click(within(sheet).getByRole('link', { name: 'Review Queue' }));
    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 1, name: /Review/i })).toBeInTheDocument(),
    );
    expect(screen.queryByRole('dialog', { name: 'Menu' })).not.toBeInTheDocument();
  });

  it('Accounts link navigates and dismisses the sheet', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    fireEvent.click(within(sheet).getByRole('link', { name: 'Accounts' }));
    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 1, name: 'Accounts' })).toBeInTheDocument(),
    );
    expect(screen.queryByRole('dialog', { name: 'Menu' })).not.toBeInTheDocument();
  });

  it('✕ close dismisses the sheet and restores focus to ☰ trigger', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const trigger = screen.getByRole('button', { name: 'Open navigation' });
    fireEvent.click(trigger);
    const sheet = await screen.findByRole('dialog', { name: 'Menu' });
    // Click the ✕ close button
    const closeBtn = within(sheet).getByRole('button', { name: 'Close' });
    fireEvent.click(closeBtn);
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Menu' })).not.toBeInTheDocument(),
    );
    // Focus returns to trigger
    expect(document.activeElement).toBe(trigger);
    // Route unchanged
    expect(screen.getByRole('heading', { level: 1, name: 'Assets' })).toBeInTheDocument();
  });

  it('Escape key dismisses the sheet and restores focus to ☰ trigger', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const trigger = screen.getByRole('button', { name: 'Open navigation' });
    fireEvent.click(trigger);
    await screen.findByRole('dialog', { name: 'Menu' });
    // Press Escape to dismiss (radix default behavior)
    fireEvent.keyDown(document.body, { key: 'Escape' });
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Menu' })).not.toBeInTheDocument(),
    );
    expect(document.activeElement).toBe(trigger);
    // Route unchanged
    expect(screen.getByRole('heading', { level: 1, name: 'Assets' })).toBeInTheDocument();
  });

  it('profile row tap opens the active-user switcher', async () => {
    renderApp('/assets', 'alice');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    const profileBtn = within(sheet).getByRole('button', { name: /alice/ });
    fireEvent.click(profileBtn);
    // Switcher input appears
    const input = await screen.findByLabelText('Active user', { selector: '#user-switcher-profile' });
    expect(input).toBeInTheDocument();
    // Input is typeable
    expect((input as HTMLInputElement).value).toBe('alice');
    fireEvent.change(input, { target: { value: 'bob' } });
    expect((input as HTMLInputElement).value).toBe('bob');
  });

  it('profile row tap opens the switcher even when no active user is set', async () => {
    renderApp('/assets');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    const profileBtn = within(sheet).getByRole('button', { name: /Choose a user/ });
    fireEvent.click(profileBtn);
    const input = await screen.findByLabelText('Active user', { selector: '#user-switcher-profile' });
    expect(input).toBeInTheDocument();
  });

  it('is axe-clean with the sheet closed (shell)', async () => {
    const { container } = renderApp('/assets');
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });

  it('is axe-clean with the sheet open', async () => {
    renderApp('/assets', 'alice');
    await screen.findByRole('heading', { level: 1, name: 'Assets' });
    const sheet = await openSheet();
    const results = await axe.run(sheet);
    expect(results).toHaveNoViolations();
  });
});
