/**
 * Task 14.5 — AccountComposerFlow (asset-management-v2).
 *
 * Strategy: mirror `asset-composer-flow.test.tsx` — mock the shared `sonner`,
 * `@/context/active-user`, and the `@/features/finance/hooks` mutation the flow
 * calls. Drive the real `<AccountComposerFlow />` end-to-end through the shadcn
 * primitives so the observable behavior is exercised:
 *
 *   1. first interaction is ONE describe-input (no Name/Type/Currency FORM
 *      controls on the describe step),
 *   2. Continue → the four account chips render (Name/Type/Currency/Institution),
 *   3. chip confirm → useCreateAccount().mutate fires with a CreateAccountRequest
 *      (name/type/currency present, institution undefined when empty),
 *   4. empty required field → inline error, mutate NOT called,
 *   5. Cancel → onOpenChange(false), mutate NOT called,
 *   6. the describe + chips views are axe-clean.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { axe } from 'vitest-axe';
import { AccountComposerFlow } from './account-composer-flow';
import { useActiveUser } from '@/context/active-user';
import { ActiveUserProvider } from '@/context/active-user';

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
  }) as unknown as SonnerToastMock,
}));

vi.mock('@/context/active-user', async () => {
  const actual = await vi.importActual<typeof import('@/context/active-user')>('@/context/active-user');
  return { ...actual, useActiveUser: vi.fn() };
});

import * as financeHooks from '@/features/finance/hooks';

vi.mock('@/features/finance/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/features/finance/hooks')>('@/features/finance/hooks');
  return { ...actual, useCreateAccount: vi.fn() };
});

const financeHooksMock = vi.mocked(financeHooks);

const USER_ID = 'alice';

interface RenderOptions {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  onClosed?: () => void;
}

function renderFlow(opts: RenderOptions = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  vi.mocked(useActiveUser).mockReturnValue({
    activeUser: USER_ID,
    setActiveUser: () => false,
    clearActiveUser: () => undefined,
  } as never);
  const onClose = vi.fn();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ActiveUserProvider queryClient={queryClient}>
          <AccountComposerFlow
            open={opts.open ?? true}
            onOpenChange={opts.onOpenChange ?? onClose}
            onClosed={opts.onClosed}
          />
        </ActiveUserProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...utils, onClose, queryClient };
}

describe('AccountComposerFlow — first interaction is one describe-input', () => {
  it('renders a single describe field and NO Name/Type/Currency FORM controls', () => {
    renderFlow();

    // The describe textbox is present (by aria-label / placeholder).
    expect(screen.getByLabelText('Describe the account')).toBeInTheDocument();

    // No structured account FORM fields on the describe step.
    expect(screen.queryByLabelText(/^Name$/i)).toBeNull();
    expect(screen.queryByLabelText(/^Type$/i)).toBeNull();
    expect(screen.queryByLabelText(/^Currency$/i)).toBeNull();
    // The create button from the legacy form is gone too.
    expect(screen.queryByRole('button', { name: /create account/i })).toBeNull();
  });

  it('cancel closes the flow without finalizing', async () => {
    const onOpenChange = vi.fn();
    vi.mocked(financeHooksMock.useCreateAccount).mockReturnValue({ mutate: vi.fn(), isPending: false } as never);
    renderFlow({ onOpenChange });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(financeHooksMock.useCreateAccount().mutate).not.toHaveBeenCalled();
  });

  it('Continue is disabled until the describe box has text', () => {
    renderFlow();
    expect(screen.getByRole('button', { name: /continue/i })).toBeDisabled();
  });
});

describe('AccountComposerFlow — continue → chips render', () => {
  it('advances to the four account chips (Name/Type/Currency/Institution)', async () => {
    renderFlow();

    // Pre-fill describe so the Name chip carries a value.
    const describeBox = screen.getByLabelText('Describe the account') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Chase checking' } });
    });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    });

    // All four fixed account fields render as visible chips.
    expect(screen.getByText('Name:')).toBeInTheDocument();
    expect(screen.getByText('Type:')).toBeInTheDocument();
    expect(screen.getByText('Currency:')).toBeInTheDocument();
    expect(screen.getByText('Institution:')).toBeInTheDocument();
    // The describe text pre-filled the Name chip.
    expect(screen.getByText('Chase checking')).toBeInTheDocument();
  });
});

describe('AccountComposerFlow — chip confirm → create', () => {
  it('mutate called with a CreateAccountRequest (institution omitted when empty)', async () => {
    const mutate = vi.fn((_vars: unknown, opts?: { onSuccess?: () => void; onError?: (e: unknown) => void }) => {
      Promise.resolve().then(() => opts?.onSuccess?.());
      return Promise.resolve({} as never);
    });
    vi.mocked(financeHooks.useCreateAccount).mockReturnValue({ mutate, isPending: false } as never);

    renderFlow();

    const describeBox = screen.getByLabelText('Describe the account') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: '  Chase checking  ' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    });

    // Fill the Type and Currency chip editors.
    const typeChip = screen.getByText('Type:').closest('button') as HTMLButtonElement;
    await act(async () => {
      fireEvent.click(typeChip);
    });
    await act(async () => {
      fireEvent.change(screen.getByLabelText('Type'), { target: { value: 'bank' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /done/i }));
    });

    const currencyChip = screen.getByText('Currency:').closest('button') as HTMLButtonElement;
    await act(async () => {
      fireEvent.click(currencyChip);
    });
    await act(async () => {
      fireEvent.change(screen.getByLabelText('Currency'), { target: { value: 'USD' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /done/i }));
    });

    // Save → create.
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /save/i }));
    });

    await expect(
      vi.waitFor(() => {
        expect(mutate).toHaveBeenCalledTimes(1);
        const callArgs = mutate.mock.calls[0];
        const body = callArgs[0] as { name: string; type: string; currency: string; institution?: string };
        expect(body.name).toBe('Chase checking');
        expect(body.type).toBe('bank');
        expect(body.currency).toBe('USD');
        expect(body.institution).toBeUndefined();
      }),
    ).resolves.toBeUndefined();

    await expect(
      vi.waitFor(() => {
        expect(sonnerMock.toast.success).toHaveBeenCalled();
      }),
    ).resolves.toBeUndefined();
  });

  it('note is captured and sent as external_descriptor (omitted when empty)', async () => {
    const mutate = vi.fn((_vars: unknown, opts?: { onSuccess?: () => void; onError?: (e: unknown) => void }) => {
      Promise.resolve().then(() => opts?.onSuccess?.());
      return Promise.resolve({} as never);
    });
    vi.mocked(financeHooks.useCreateAccount).mockReturnValue({ mutate, isPending: false } as never);

    renderFlow();

    const describeBox = screen.getByLabelText('Describe the account') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Chase checking' } });
    });
    // Fill the optional Note (the describe-step free-text input).
    const noteBox = screen.getByLabelText('Note (optional)') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(noteBox, { target: { value: '  opened for tax savings  ' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    });

    const typeChip = screen.getByText('Type:').closest('button') as HTMLButtonElement;
    await act(async () => { fireEvent.click(typeChip); });
    await act(async () => { fireEvent.change(screen.getByLabelText('Type'), { target: { value: 'bank' } }); });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: /done/i })); });
    const currencyChip = screen.getByText('Currency:').closest('button') as HTMLButtonElement;
    await act(async () => { fireEvent.click(currencyChip); });
    await act(async () => { fireEvent.change(screen.getByLabelText('Currency'), { target: { value: 'USD' } }); });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: /done/i })); });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /save/i }));
    });

    // The trimmed note travels as external_descriptor (the persisted optional
    // free-text slot); it must NOT be silently dropped.
    await expect(
      vi.waitFor(() => {
        expect(mutate).toHaveBeenCalledTimes(1);
        const body = mutate.mock.calls[0][0] as { name: string; type: string; currency: string; external_descriptor?: string };
        expect(body.external_descriptor).toBe('opened for tax savings');
      }),
    ).resolves.toBeUndefined();
  });

  it('empty required field → inline error and mutate NOT called', async () => {
    const mutate = vi.fn();
    vi.mocked(financeHooks.useCreateAccount).mockReturnValue({ mutate, isPending: false } as never);

    renderFlow();

    const describeBox = screen.getByLabelText('Describe the account') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Chase checking' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    });

    // Leave Type and Currency empty, then Save.
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /save/i }));
    });

    expect(await screen.findByText('Name, Type, and Currency are required')).toBeInTheDocument();
    expect(mutate).not.toHaveBeenCalled();
  });

  it('invalid account type (not in the enum) → field-level error and mutate NOT called', async () => {
    const mutate = vi.fn();
    vi.mocked(financeHooks.useCreateAccount).mockReturnValue({ mutate, isPending: false } as never);
    renderFlow();

    const describeBox = screen.getByLabelText('Describe the account') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Chase checking' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    });

    // Type a non-enum type and a valid currency, then Save.
    const typeChip = screen.getByText('Type:').closest('button') as HTMLButtonElement;
    await act(async () => { fireEvent.click(typeChip); });
    await act(async () => { fireEvent.change(screen.getByLabelText('Type'), { target: { value: 'checking' } }); });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: /done/i })); });
    const currencyChip = screen.getByText('Currency:').closest('button') as HTMLButtonElement;
    await act(async () => { fireEvent.click(currencyChip); });
    await act(async () => { fireEvent.change(screen.getByLabelText('Currency'), { target: { value: 'INR' } }); });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: /done/i })); });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /save/i }));
    });

    expect(await screen.findByText(/Type must be one of/i)).toBeInTheDocument();
    expect(mutate).not.toHaveBeenCalled();
  });
});

describe('AccountComposerFlow — accessibility', () => {
  it('the describe view passes axe', async () => {
    const { container } = renderFlow();
    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });

  it('the chips view passes axe', async () => {
    const { container } = renderFlow();
    const describeBox = screen.getByLabelText('Describe the account') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Chase checking' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /continue/i }));
    });
    await expect(screen.findByText('Name:')).resolves.toBeInTheDocument();

    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });
});
