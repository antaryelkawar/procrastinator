/**
 * Task 13.4 — shared top-right [+] context control (app-chrome spec "Context [+]
 * on every non-landing view", design D10). The section pages that consume it
 * (assets/documents/finance/reviews) are wired in 14.x; this test covers the
 * shared control itself plus the landing-page exclusion.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { axe } from 'vitest-axe';

import { AddButton } from './add-button';
import { LandingPage } from '@/features/landing/landing-page';
import { ActiveUserProvider } from '@/context/active-user';

// The four per-section labels from the app-chrome spec table.
const SECTION_LABELS = ['Add asset', 'Add document', 'Add account', 'Add review'] as const;

/**
 * Task 13.4 chrome assertions for the fixed top-right floating [+]. jsdom does
 * not resolve Tailwind utility classes to computed styles or layout (boxes are
 * 0×0), so the positioning/sizing contract is asserted via the class list
 * (`fixed right-4 top-4` for placement, `size-12` = 48px for the ≥44px hit
 * target) — same convention as `app-shell.test.tsx:95-106`.
 */
function expectTopRightAddButton(button: HTMLElement): void {
  const classes = button.className ?? '';
  expect(classes).toContain('fixed');
  // Top-right placement (fixed at top-4/right-4 = 16px inset).
  expect(classes).toContain('right-4');
  expect(classes).toContain('top-4');
  // z-40: above content and the landing chat pill (z-30), below the nav sheet
  // scrim/panel (z-50).
  expect(classes).toContain('z-40');
  // Hit target ≥ 44×44: size-12 = 48px.
  expect(classes).toContain('size-12');
}

/**
 * Renders the real landing page at `/` inside the providers it requires
 * (QueryClient + active user). Modeled on `landing-page.test.tsx:54-67`; no
 * ingest-hook or `useNavigate` mocks needed — the ingest cards region is not
 * rendered on initial mount (`ingestOpen` starts false) and `useNavigate` works
 * for real inside `MemoryRouter`.
 */
function renderLanding() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/']}>
        <ActiveUserProvider queryClient={queryClient}>
          <LandingPage />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AddButton', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it.each(SECTION_LABELS)('renders with accessible name %s', (label) => {
    render(<AddButton label={label} onAdd={vi.fn()} />);
    expect(screen.getByRole('button', { name: label })).toBeInTheDocument();
  });

  it('is a fixed top-right button with a ≥44px hit target', () => {
    render(<AddButton label="Add asset" onAdd={vi.fn()} />);
    const button = screen.getByRole('button', { name: 'Add asset' });
    expectTopRightAddButton(button);
  });

  it('invokes onAdd exactly once when clicked', () => {
    const onAdd = vi.fn();
    render(<AddButton label="Add asset" onAdd={onAdd} />);
    fireEvent.click(screen.getByRole('button', { name: 'Add asset' }));
    expect(onAdd).toHaveBeenCalledTimes(1);
  });

  it('merges non-conflicting extra className via cn', () => {
    render(<AddButton label="Add asset" onAdd={vi.fn()} className="mt-2" />);
    const button = screen.getByRole('button', { name: 'Add asset' });
    expect(button).toHaveClass('mt-2');
    // ...and the core chrome contract is preserved.
    expectTopRightAddButton(button);
  });

  it('keeps the ≥44px/fixed chrome authoritative over conflicting className', () => {
    // A 14.x consumer passing a conflicting size/positioning utility must not
    // break the shared chrome contract: the core chrome wins via tailwind-merge
    // (size-6→size-12, top-0→top-4 are the same conflict groups as the chrome).
    render(<AddButton label="Add asset" onAdd={vi.fn()} className="size-6 top-0" />);
    const button = screen.getByRole('button', { name: 'Add asset' });
    expect(button).not.toHaveClass('size-6');
    expect(button).not.toHaveClass('top-0');
    expectTopRightAddButton(button);
  });

  it('the landing page keeps its centered [+] and shows no top-right [+]', () => {
    const { container } = renderLanding();
    // The landing's big CENTERED [+] is present.
    expect(screen.getByRole('button', { name: 'Add something' })).toBeInTheDocument();
    // No top-right corner [+]: no button carries the fixed right-4 chrome.
    expect(container.querySelector('button[class*="right-4"]')).toBeNull();
    // And no section-labeled add button anywhere.
    expect(screen.queryByRole('button', { name: /add (asset|document|account|review)/i })).toBeNull();
  });

  it('is axe-clean', async () => {
    const { container } = render(<AddButton label="Add asset" onAdd={vi.fn()} />);
    expect(await axe(container)).toHaveNoViolations();
  });
});
