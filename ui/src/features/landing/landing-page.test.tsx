/**
 * Task 7.2/7.3/14.2 — landing page at `/` (design D7, landing-page spec).
 *
 * The landing page renders directly at `/` (no redirect) and carries the five
 * top-to-bottom regions: a centered PROCRASTINATOR hero wordmark (a text brand
 * link to `/`), the large search bar, the centered [+] ingest button (which
 * reveals the unified "Add" composer — task 14.1), a labeled "Insights"
 * placeholder, and a fixed, disabled chat-bar pill.
 *
 * Search is a plain form + useNavigate (mocked here), but the revealed ingest
 * cards consume data hooks (task 7.3), so the render helper provides a
 * QueryClient + ActiveUser context and mocks the cards' ingest hooks.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import axe from 'axe-core';

import { LandingPage } from './landing-page';
import { ActiveUserProvider } from '@/context/active-user';

const mockNavigate = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual<typeof import('react-router')>('react-router');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// The AddComposer (task 14.1) consumes mutation hooks; mock them so the
// landing-level assertions don't depend on the data layer.
vi.mock('@/features/docs/composer/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/features/docs/composer/hooks')>('@/features/docs/composer/hooks');
  return {
    ...actual,
    useIngestFile: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
    useIngestText: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
    useReprocessDocument: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
    useKeepDocument: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, error: null })),
  };
});

vi.mock('@/context/active-user', async () => {
  const actual = await vi.importActual<typeof import('@/context/active-user')>('@/context/active-user');
  return { ...actual, useActiveUser: () => ({ activeUser: 'alice' }) };
});

/**
 * Renders the page at `/` inside the providers the ingest cards require
 * (QueryClient + active user).
 */
function renderLanding(initialPath = '/') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <ActiveUserProvider queryClient={queryClient}>
          <LandingPage />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('LandingPage', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
  });

  it('navigates to /search?q=<query> when the search form is submitted', () => {
    renderLanding();
    const input = screen.getByRole('searchbox', { name: /search/i });
    fireEvent.change(input, { target: { value: 'macbook pro' } });
    // submit on the input bubbles up to the wrapping <form role="search">
    fireEvent.submit(input);
    expect(mockNavigate).toHaveBeenCalledWith('/search?q=macbook%20pro');
  });

  it('submits an empty query as /search with no q param', () => {
    renderLanding();
    fireEvent.submit(screen.getByRole('searchbox', { name: /search/i }));
    expect(mockNavigate).toHaveBeenCalledWith('/search');
  });

  it('renders all five landing regions in order with no persistent navigation', () => {
    renderLanding();
    const wordmark = screen.getByRole('link', { name: /procrastinator/i });
    const searchBox = screen.getByRole('searchbox', { name: /search/i });
    const addButton = screen.getByRole('button', { name: 'Add something' });
    const insights = screen.getByRole('heading', { name: 'Insights' });
    const pill = screen.getByRole('button', { name: /ask about your stuff/i });

    // All five regions are present, top-to-bottom: wordmark → search → [+] →
    // insights → pill.
    expect(wordmark).toBeInTheDocument();
    expect(searchBox).toBeInTheDocument();
    expect(addButton).toBeInTheDocument();
    expect(insights).toBeInTheDocument();
    expect(pill).toBeInTheDocument();

    // DOM order: the wordmark link precedes the search box, which precedes the
    // [+] button, Insights heading, and chat-pill button. `a.compareDocumentPosition(b)`
    // flags DOCUMENT_POSITION_FOLLOWING when b comes after a.
    expect(wordmark.compareDocumentPosition(searchBox) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(searchBox.compareDocumentPosition(addButton) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(addButton.compareDocumentPosition(insights) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(insights.compareDocumentPosition(pill) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    // No persistent sidebar / nav landmark anywhere on the page.
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    // The app shell (hamburger + nav sheet) is not the landing page's concern
    expect(screen.queryByRole('button', { name: 'Open navigation' })).not.toBeInTheDocument();

    // No leftover 3-card ingest grid: the ingest-card actions only appear once
    // the [+] composer is open, never on the closed landing page.
    expect(screen.queryByRole('button', { name: 'Add from camera' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Upload a file' })).not.toBeInTheDocument();
  });

  it('renders the PROCRASTINATOR hero wordmark as a link to /', () => {
    renderLanding();
    const wordmark = screen.getByRole('link', { name: /procrastinator/i });
    expect(wordmark).toBeInTheDocument();
    expect(wordmark).toHaveAttribute('href', '/');
    // Text brand mark (task 14.2), not an image: the visible text is present.
    expect(screen.getByText(/procrastinator/i)).toBeInTheDocument();
    // A ≥44px tap target (min-h-11 + padding).
    expect(wordmark).toHaveClass('min-h-11');
  });

  it('shows a labeled Insights placeholder section', () => {
    renderLanding();
    expect(screen.getByRole('heading', { name: 'Insights' })).toBeInTheDocument();
    expect(screen.getByText(/insights widgets will land here later/i)).toBeInTheDocument();
  });

  it('renders the chat-bar pill as present but disabled (not interactive)', () => {
    renderLanding();
    const pill = screen.getByRole('button', { name: /ask about your stuff/i });
    expect(pill).toBeInTheDocument();
    expect(pill).toBeDisabled();
  });

  it('reveals the composer on [+] and hides it on toggle', () => {
    renderLanding();
    const addButton = screen.getByRole('button', { name: 'Add something' });

    // Hidden before the first click
    expect(screen.queryByText('Camera/Image')).not.toBeInTheDocument();
    expect(addButton).toHaveAttribute('aria-expanded', 'false');

    // [+] reveals the composer (task 14.1)
    fireEvent.click(addButton);
    expect(addButton).toHaveAttribute('aria-expanded', 'true');
    // optional note field
    expect(screen.getByRole('textbox', { name: /note/i })).toBeInTheDocument();
    // the attach-strip actions
    expect(screen.getByRole('button', { name: 'Add from camera' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Upload a file' })).toBeInTheDocument();

    // Clicking again hides the composer
    fireEvent.click(addButton);
    expect(addButton).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('textbox', { name: /note/i })).not.toBeInTheDocument();
  });

  it('is axe-clean', async () => {
    const { container } = renderLanding();
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });
});
