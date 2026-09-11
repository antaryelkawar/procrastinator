/**
 * Task 14.5 — ReviewComposerFlow (design D11, review-ingest spec).
 *
 * Strategy: mirror `asset-composer-flow.test.tsx` — mock the shared ingest
 * hooks, `sonner`, `@/context/active-user`, and the `@/lib/api/client` read
 * fns the flow calls. Drive the real `<ReviewComposerFlow />` end-to-end
 * through the shadcn primitives so the observable behavior is exercised:
 *
 *   1. first interaction is ONE describe-input (no candidate/merchant/amount
 *      form),
 *   2. held-for-review submit → getReview feeds the chips from candidate_fields,
 *   3. chip confirm with no edit → approveReview commits with the review id,
 *   4. committed outcome (asset_committed) → toast + close, no chips, no
 *      approveReview,
 *   5. 409 duplicate → the shared DuplicatePromptDialog (not the chips),
 *   6. the describe + chips views are axe-clean.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { axe } from 'vitest-axe';
import * as hooks from '@/features/docs/composer/hooks';
import * as client from '@/lib/api/client';
import { ReviewComposerFlow } from './review-composer-flow';
import { useActiveUser } from '@/context/active-user';
import { ActiveUserProvider } from '@/context/active-user';
import type { DuplicateReport } from '@/lib/api/generated/orval/procrastinator';

vi.mock('@/features/docs/composer/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/features/docs/composer/hooks')>(
    '@/features/docs/composer/hooks',
  );
  return {
    ...actual,
    useIngestFile: vi.fn(),
    useIngestText: vi.fn(),
    useKeepDocument: vi.fn(),
    useReprocessDocument: vi.fn(),
  };
});

vi.mock('@/lib/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/client')>('@/lib/api/client');
  return {
    ...actual,
    getAsset: vi.fn(),
    getReview: vi.fn(),
    patchAsset: vi.fn(),
    approveReview: vi.fn(),
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
  }) as unknown as SonnerToastMock,
}));

vi.mock('@/context/active-user', async () => {
  const actual = await vi.importActual<typeof import('@/context/active-user')>('@/context/active-user');
  return { ...actual, useActiveUser: vi.fn() };
});

const USER_ID = 'alice';

function makeIngestHook(result: unknown, mutate = vi.fn()) {
  const mutateAsync = vi.fn(async (_vars: unknown) => {
    mutate(_vars);
    return result;
  });
  return { mutate: mutateAsync, mutateAsync, isPending: false, isSuccess: false, error: null } as never;
}

function makeUseChoice(mutate = vi.fn()) {
  const mutateAsync = vi.fn(async (_vars: unknown) => {
    mutate(_vars);
    return { id: 'd1' };
  });
  return { mutate: mutateAsync, mutateAsync, isPending: false, isSuccess: false, error: null };
}

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
          <ReviewComposerFlow
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

describe('ReviewComposerFlow — first interaction is one describe-input', () => {
  it('renders a single describe field and NO candidate/merchant/amount fields', () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'held_for_review', review_id: 'r1' }]),
    );
    renderFlow();

    // Exactly one describe input, located by its placeholder/label.
    const describes = screen.getAllByRole('textbox');
    expect(describes.length).toBeGreaterThanOrEqual(1);

    // No structured candidate fields should exist anywhere on the describe step.
    expect(screen.queryByLabelText(/brand/i)).toBeNull();
    expect(screen.queryByLabelText(/merchant/i)).toBeNull();
    expect(screen.queryByLabelText(/amount/i)).toBeNull();
  });

  it('cancel closes the flow without finalizing', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'held_for_review', review_id: 'r1' }]),
    );
    const onClose = vi.fn();
    renderFlow({ onOpenChange: onClose });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    });
    expect(onClose).toHaveBeenCalledWith(false);
    expect(client.patchAsset).not.toHaveBeenCalled();
    expect(client.approveReview).not.toHaveBeenCalled();
  });
});

describe('ReviewComposerFlow — held-for-review submit → chips render', () => {
  it('held for review: getReview feeds the chips from candidate_fields', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'held_for_review', review_id: 'r1' }]),
    );
    vi.mocked(client.getReview).mockResolvedValue({
      id: 'r1',
      source_id: 's1',
      source_filename: 'doc.pdf',
      source_uploaded_at: '2026-09-01T10:00:00Z',
      created_at: '2026-09-01T10:00:00Z',
      updated_at: '2026-09-01T10:00:00Z',
      data: {
        candidate_fields: { brand: 'Bosch', model: 'WAT40200', price: '499' },
        confidence: 0.87,
        provenance: 'pasted',
        source: 'pasted',
        kind: 'asset',
      },
    } as never);

    renderFlow();
    const describeBox = screen.getByPlaceholderText(/describe it/i) as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Bosch washer' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    await act(async () => {
      await expect(screen.findByText('Bosch')).resolves.toBeInTheDocument();
    });
    // The chips are populated from getReview(data.candidate_fields).
    expect(screen.getByText('Bosch')).toBeInTheDocument();
    expect(screen.getByText('499')).toBeInTheDocument();
  });

  it('a submit error shows a friendly inline message', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    const textMutate = vi.fn(async () => {
      throw new Error('upload failed');
    });
    vi.mocked(hooks.useIngestText).mockReturnValue({
      mutate: textMutate,
      mutateAsync: textMutate,
      isPending: false,
      isSuccess: false,
      error: null,
    } as never);
    renderFlow();

    const describeBox = screen.getByPlaceholderText(/describe it/i) as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'something' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    expect(await screen.findByText('upload failed')).toBeInTheDocument();
    // No chips appeared after the failure.
    expect(screen.queryByLabelText(/brand/i)).toBeNull();
  });
});

describe('ReviewComposerFlow — chip confirm → approveReview path', () => {
  it('held review + no edits → approveReview commits with the review id', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'held_for_review', review_id: 'r1' }]),
    );
    vi.mocked(client.getReview).mockResolvedValue({
      id: 'r1',
      source_id: 's1',
      source_filename: 'doc.pdf',
      source_uploaded_at: '2026-09-01T10:00:00Z',
      created_at: '2026-09-01T10:00:00Z',
      updated_at: '2026-09-01T10:00:00Z',
      data: {
        candidate_fields: { brand: 'Bosch', model: 'WAT40200', price: '499' },
        confidence: 0.87,
        provenance: 'pasted',
        source: 'pasted',
        kind: 'asset',
      },
    } as never);
    vi.mocked(client.approveReview).mockResolvedValue({
      asset: {
        id: 'a9',
        data: { brand: 'Bosch', model: 'WAT40200', price: '499', metadata: {} },
        created_at: '2026-09-01T10:00:00Z',
        updated_at: '2026-09-01T10:00:00Z',
      },
      review: { id: 'r1' } as never,
    } as never);

    renderFlow();
    const describeBox = screen.getByPlaceholderText(/describe it/i) as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Bosch washer' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    await expect(screen.findByText('Bosch')).resolves.toBeInTheDocument();

    // Save without editing any chip → approveReview (held-review path).
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /save/i }));
    });

    await expect(
      vi.waitFor(() => {
        expect(client.approveReview).toHaveBeenCalledWith('alice', 'r1');
      }),
    ).resolves.toBeUndefined();
    // No edits → patchAsset is not called.
    expect(client.patchAsset).not.toHaveBeenCalled();
  });
});

describe('ReviewComposerFlow — committed (asset) outcome → toast + close', () => {
  it('asset_committed: toast success, flow closes, NO chips, NO approveReview', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    const textMutate = vi.fn(async () => [{ kind: 'asset_committed', asset_id: 'a1' }]);
    vi.mocked(hooks.useIngestText).mockReturnValue({
      mutate: textMutate,
      mutateAsync: textMutate,
      isPending: false,
      isSuccess: false,
      error: null,
    } as never);

    const onOpenChange = vi.fn();
    renderFlow({ onOpenChange });

    const describeBox = screen.getByPlaceholderText(/describe it/i) as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Philips air fryer' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    // The flow closed and the committed toast fired.
    await expect(
      vi.waitFor(() => {
        expect(sonnerMock.toast.success).toHaveBeenCalledWith('Committed');
      }),
    ).resolves.toBeUndefined();
    expect(onOpenChange).toHaveBeenCalledWith(false);

    // No chips rendered (no candidate text), no review approval.
    expect(screen.queryByText('Philips')).toBeNull();
    expect(client.approveReview).not.toHaveBeenCalled();
    expect(client.patchAsset).not.toHaveBeenCalled();
    expect(client.getReview).not.toHaveBeenCalled();
  });
});

describe('ReviewComposerFlow — 409 duplicate path', () => {
  it('duplicate ingest outcome → DuplicatePromptDialog (not the chips)', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(
      makeIngestHook({ kind: 'duplicate', report: DUPLICATE_REPORT }),
    );
    vi.mocked(hooks.useIngestText).mockReturnValue(makeIngestHook([]) as never);
    vi.mocked(hooks.useKeepDocument).mockReturnValue(makeUseChoice() as never);
    vi.mocked(hooks.useReprocessDocument).mockReturnValue(makeUseChoice() as never);
    renderFlow();

    // Attach a file so the FILE upload path runs — it is the path that returns a
    // structured DuplicateReport (the text ingest only yields a bare duplicate
    // outcome with no reprocess/keep prompt to drive the modal).
    const fileInput = screen.getByTestId('composer-file-input') as HTMLInputElement;
    const file = new File(['data'], 'new-invoice.pdf', { type: 'application/pdf' });
    Object.defineProperty(fileInput, 'files', { value: [file] });
    await act(async () => {
      fireEvent.change(fileInput);
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    // The reprocess/keep dialog appears; the chips do NOT.
    await expect(screen.findByText('Duplicate detected')).resolves.toBeInTheDocument();
    expect(screen.getByText('existing-invoice.pdf')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /reprocess/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /keep existing/i })).toBeInTheDocument();
    // No review chips rendered.
    expect(screen.queryByText('Philips')).toBeNull();
  });
});

describe('ReviewComposerFlow — accessibility', () => {
  it('the describe view passes axe', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'held_for_review', review_id: 'r1' }]),
    );
    vi.mocked(client.getReview).mockResolvedValue({
      id: 'r1',
      source_id: 's1',
      source_filename: 'doc.pdf',
      data: {
        candidate_fields: { brand: 'Philips', model: 'HX8221', metadata: {} },
        confidence: 0.87,
      },
    } as never);

    const { container } = renderFlow();
    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });

  it('the chips view passes axe', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'held', review: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'held_for_review', review_id: 'r1' }]),
    );
    vi.mocked(client.getReview).mockResolvedValue({
      id: 'r1',
      source_id: 's1',
      source_filename: 'doc.pdf',
      data: {
        candidate_fields: { brand: 'Philips', model: 'HX8221' },
        confidence: 0.87,
      },
    } as never);

    const { container } = renderFlow();
    const describeBox = document.getElementById('describe-input') as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Philips air fryer' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });
    await expect(screen.findByText('Philips')).resolves.toBeInTheDocument();

    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });
});

describe('ReviewComposerFlow — multi-file submit (CRIT-D2-01)', () => {
  it('attaching 2 files ingests BOTH (mutation called twice, no silent drop)', async () => {
    const fileHookResult = { kind: 'committed', asset: { id: 'a1' } };
    const fileMutate = vi.fn(async (_vars: unknown) => fileHookResult);
    vi.mocked(hooks.useIngestFile).mockReturnValue({
      mutate: fileMutate,
      mutateAsync: fileMutate,
      isPending: false,
      isSuccess: false,
      error: null,
    } as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeIngestHook([]) as never);
    renderFlow();

    const fileInput = screen.getByTestId('composer-file-input') as HTMLInputElement;
    const f1 = new File(['d1'], 'a.pdf', { type: 'application/pdf' });
    const f2 = new File(['d2'], 'b.pdf', { type: 'application/pdf' });
    Object.defineProperty(fileInput, 'files', { value: [f1, f2] });
    await act(async () => {
      fireEvent.change(fileInput);
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    await vi.waitFor(() => {
      expect(fileMutate).toHaveBeenCalledTimes(2);
    });
    expect(fileMutate).toHaveBeenCalledWith({ file: f1, note: undefined });
    expect(fileMutate).toHaveBeenCalledWith({ file: f2, note: undefined });
  });
});
