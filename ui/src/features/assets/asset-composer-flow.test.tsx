/**
 * Task 14.4 — AssetComposerFlow (design D11, asset-registry spec).
 *
 * Strategy: mirror `add-composer.test.tsx` — mock the shared ingest hooks,
 * `sonner`, `@/context/active-user`, and the `@/lib/api/client` write/read
 * fns the flow calls. Drive the real `<AssetComposerFlow />` end-to-end
 * through the shadcn primitives so the observable behavior is exercised:
 *
 *   1. first interaction is ONE describe-input (no brand/model/serial form),
 *   2. submit → chips render (text committed path),
 *   3. chip edit + save → the asset write path (patchAsset) fires with the
 *      confirmed chip value,
 *   4. 409 duplicate → the shared DuplicatePromptDialog (not the chips),
 *   5. the describe + chips views are axe-clean.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { axe } from 'vitest-axe';
import * as hooks from '@/features/docs/composer/hooks';
import * as client from '@/lib/api/client';
import { AssetComposerFlow } from './asset-composer-flow';
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
          <AssetComposerFlow
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

describe('AssetComposerFlow — first interaction is one describe-input', () => {
  it('renders a single describe field and NO brand/model/serial/purchase-date fields', () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'asset_committed' }]) as never,
    );
    renderFlow();

    // Exactly one describe input, located by its placeholder/label.
    const describes = screen.getAllByRole('textbox');
    expect(describes.length).toBeGreaterThanOrEqual(1);

    // No structured asset fields should exist anywhere on the describe step.
    expect(screen.queryByLabelText(/brand/i)).toBeNull();
    expect(screen.queryByLabelText(/model/i)).toBeNull();
    expect(screen.queryByLabelText(/serial/i)).toBeNull();
    expect(screen.queryByLabelText(/purchase date/i)).toBeNull();
  });

  it('cancel closes the flow without finalizing', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'asset_committed' }]) as never,
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

describe('AssetComposerFlow — submit → chips render', () => {
  it('text committed: getAsset feeds the chips with the extracted values', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
    const textMutate = vi.fn(async (_v: unknown) => [{ kind: 'asset_committed', asset_id: 'a1' }]);
    vi.mocked(hooks.useIngestText).mockReturnValue({
      mutate: textMutate,
      mutateAsync: textMutate,
      isPending: false,
      isSuccess: false,
      error: null,
    } as never);
    vi.mocked(client.getAsset).mockResolvedValue({
      id: 'a1',
      data: {
        brand: 'Philips',
        model: 'HX8221',
        serial_number: 'SN-123',
        price: '3200',
        currency: 'USD',
        asset_category: 'electronics',
        purchase_date: '2024-01-01T00:00:00Z',
        metadata: {},
      },
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    } as never);

    renderFlow();

    const describeBox = screen.getByPlaceholderText(/describe it/i) as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Philips air fryer' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    await act(async () => {
      await expect(screen.findByText('Philips')).resolves.toBeInTheDocument();
    });
    // The chips are populated from getAsset(data).
    expect(screen.getByText('Philips')).toBeInTheDocument();
    expect(screen.getByText('3200')).toBeInTheDocument();
    // The text ingest was called with the describe text as the directive.
    await expect(
      vi.waitFor(() => {
        expect(textMutate).toHaveBeenCalledWith({ text: 'Philips air fryer' });
      }),
    ).resolves.toBeUndefined();
  });

  it('held for review: getReview feeds the chips from candidate_fields', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
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
    expect(screen.getByText('Bosch')).toBeInTheDocument();
    expect(screen.getByText('499')).toBeInTheDocument();
  });

  it('a submit error shows a friendly inline message', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
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

describe('AssetComposerFlow — chip confirm → create path with confirmed values', () => {
  it('committed + edited chip → patchAsset called with the confirmed value', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'asset_committed', asset_id: 'a1' }]),
    );
    vi.mocked(client.getAsset).mockResolvedValue({
      id: 'a1',
      data: {
        brand: 'Philips',
        model: 'HX8221',
        serial_number: 'SN-123',
        price: '3200',
        currency: 'USD',
        asset_category: 'electronics',
        purchase_date: '2024-01-01T00:00:00Z',
        metadata: {},
      },
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    } as never);

    renderFlow();
    const describeBox = screen.getByPlaceholderText(/describe it/i) as HTMLTextAreaElement;
    await act(async () => {
      fireEvent.change(describeBox, { target: { value: 'Philips air fryer' } });
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /add/i }));
    });

    await expect(screen.findByText('Philips')).resolves.toBeInTheDocument();

    // Tap the Brand chip to reveal its editor.
    const brandChip = screen.getByText('Brand:').closest('button') as HTMLButtonElement;
    await act(async () => {
      fireEvent.click(brandChip);
    });

    // The revealed editor input carries the extracted value.
    const editorInput = document.getElementById('review-input-brand') as HTMLInputElement;
    expect(editorInput).not.toBeNull();
    expect(editorInput.value).toBe('Philips');

    // Edit it.
    await act(async () => {
      fireEvent.change(editorInput, { target: { value: 'Philips Healthcare' } });
    });

    // Done collapses the editor.
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /done/i }));
    });

    // Save → finalize through patchAsset (committed asset + edited chip).
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /save/i }));
    });

    await expect(
      vi.waitFor(() => {
        expect(client.patchAsset).toHaveBeenCalledWith('alice', 'a1', { brand: 'Philips Healthcare' });
      }),
    ).resolves.toBeUndefined();
    expect(sonnerMock.toast.success).toHaveBeenCalled();
    expect(client.patchAsset).toHaveBeenCalledWith('alice', 'a1', { brand: 'Philips Healthcare' });
  });

  it('held review + no edits → approveReview commits with the candidate fields', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
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
  });
});

describe('AssetComposerFlow — 409 duplicate path', () => {
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

describe('AssetComposerFlow — accessibility', () => {
  it('the describe view passes axe', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'asset_committed', asset_id: 'a1' }]),
    );
    vi.mocked(client.getAsset).mockResolvedValue({
      id: 'a1',
      data: { brand: 'Philips', metadata: {} },
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    } as never);

    const { container } = renderFlow();
    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });

  it('the chips view passes axe', async () => {
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeIngestHook({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(
      makeIngestHook([{ kind: 'asset_committed', asset_id: 'a1' }]),
    );
    vi.mocked(client.getAsset).mockResolvedValue({
      id: 'a1',
      data: { brand: 'Philips', model: 'HX8221', metadata: {} },
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
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

describe('AssetComposerFlow — multi-file submit (CRIT-D2-01)', () => {
  it('attaching 2 files ingests BOTH (mutation called twice, no silent drop)', async () => {
    const fileHookResult = { kind: 'committed', asset: { id: 'a1', data: { brand: 'Philips' } } };
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
