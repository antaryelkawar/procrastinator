/**
 * Task 14.1 — AddComposer (design D11, revised: "upload anything + optional
 * text" model).
 *
 * Strategy: mock the ingest hooks (`./hooks`) so each test drives the observable
 * UI behavior end-to-end through the real `<AddComposer />` and the shadcn
 * primitives — no test seams. The mock `mutateAsync` returns the discriminated
 * union the real hooks resolve (committed / held / duplicate), so the
 * duplicate-report → dialog flow is exercised through the real
 * `DuplicatePromptDialog`.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import axe from 'axe-core';
import * as hooks from './hooks';
import { AddComposer } from './add-composer';
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

function renderComposer(props: { open?: boolean; onOpenChange?: (open: boolean) => void } = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  vi.mocked(useActiveUser).mockReturnValue({
    activeUser: USER_ID,
    setActiveUser: () => false,
    clearActiveUser: () => undefined,
  } as never);
  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ActiveUserProvider queryClient={queryClient}>
            <AddComposer
              open={props.open ?? true}
              onOpenChange={props.onOpenChange ?? (() => undefined)}
            />
          </ActiveUserProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    ),
    queryClient,
  };
}

describe('AddComposer — upload surface', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeUseIngestFile({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([{ kind: 'asset_committed' }]) as never);
  });

  it('renders the upload surface, camera action, and one note field (no structured form)', () => {
    renderComposer();
    expect(screen.getByRole('button', { name: 'Upload a file' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add from camera' })).toBeInTheDocument();
    // The single optional note field (located by its label).
    expect(screen.getByLabelText(/note/i)).toBeInTheDocument();
    // No brand/model/serial form fields.
    expect(screen.queryByLabelText(/brand/i)).toBeNull();
    expect(screen.queryByLabelText(/model/i)).toBeNull();
    expect(screen.queryByLabelText(/serial/i)).toBeNull();
  });

  it('the Add button is disabled when there is nothing to submit', () => {
    renderComposer();
    const addBtn = screen.getByRole('button', { name: 'Add' });
    expect(addBtn).toBeDisabled();
  });
});

describe('AddComposer — attachment chips', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeUseIngestFile({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([{ kind: 'asset_committed' }]) as never);
  });

  it('picking a file adds a removable chip with the filename', async () => {
    renderComposer();
    const file = makeFile('invoice.pdf', 'application/pdf');
    await pickFile('[data-testid="composer-file-input"]', file);
    expect(screen.getByText('invoice.pdf')).toBeInTheDocument();
    const removeBtn = screen.getByRole('button', { name: /remove invoice\.pdf/i });
    await act(async () => {
      fireEvent.click(removeBtn);
    });
    expect(screen.queryByText('invoice.pdf')).not.toBeInTheDocument();
  });

  it('camera pick adds a chip', async () => {
    renderComposer();
    const file = makeFile('photo.jpg', 'image/jpeg');
    await pickFile('[data-testid="composer-camera-input"]', file);
    expect(screen.getByText('photo.jpg')).toBeInTheDocument();
  });

  it('drag-drop on the upload surface adds a chip', async () => {
    renderComposer();
    const file = makeFile('dragfile.pdf', 'application/pdf');
    const uploadSurface = screen.getByRole('button', { name: 'Upload a file' });
    await act(async () => {
      fireEvent.drop(uploadSurface, {
        dataTransfer: { files: [file] },
      } as unknown as DragEvent);
    });
    expect(screen.getByText('dragfile.pdf')).toBeInTheDocument();
  });
});

describe('AddComposer — note travels as directive (file path)', () => {
  it('useIngestFile is called with { file, note: <typed note> }', async () => {
    const fileHook = makeUseIngestFile({ kind: 'committed', asset: {} });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    renderComposer();

    const file = makeFile('invoice.pdf', 'application/pdf');
    await pickFile('[data-testid="composer-file-input"]', file);
    const noteInput = screen.getByLabelText(/note/i) as HTMLTextAreaElement;
    fireEvent.change(noteInput, { target: { value: 'scanner note' } });
    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await vi.waitFor(() => {
      expect(fileHook.mutateAsync).toHaveBeenCalledWith({ file, note: 'scanner note' });
    });
  });

  it('submits an empty note as undefined when the note field is empty', async () => {
    const fileHook = makeUseIngestFile({ kind: 'committed', asset: {} });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    renderComposer();

    const file = makeFile('invoice.pdf', 'application/pdf');
    await pickFile('[data-testid="composer-file-input"]', file);
    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await vi.waitFor(() => {
      expect(fileHook.mutateAsync).toHaveBeenCalledWith({ file, note: undefined });
    });
  });
});

describe('AddComposer — submitting flag lifecycle (file path)', () => {
  it('re-enables the Add button after a committed file upload', async () => {
    const fileHook = makeUseIngestFile({ kind: 'committed', asset: {} });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    renderComposer();

    const file = makeFile('invoice.pdf', 'application/pdf');
    await pickFile('[data-testid="composer-file-input"]', file);
    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await vi.waitFor(() => {
      // While submitting the button reads 'Adding…'; on success the submitting
      // flag must clear so the label reverts to 'Add' and aria-busy is false.
      const after = screen.getByRole('button', { name: 'Add' });
      expect(after).toHaveAttribute('aria-busy', 'false');
      expect(fileHook.mutateAsync).toHaveBeenCalledTimes(1);
    });
  });

  it('clears the submitting flag after a held file upload', async () => {
    const fileHook = makeUseIngestFile({ kind: 'held', review: {} });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    renderComposer();

    const file = makeFile('invoice.pdf', 'application/pdf');
    await pickFile('[data-testid="composer-file-input"]', file);
    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await vi.waitFor(() => {
      const after = screen.getByRole('button', { name: 'Add' });
      expect(after).toHaveAttribute('aria-busy', 'false');
      expect(fileHook.mutateAsync).toHaveBeenCalledTimes(1);
    });
  });
});

describe('AddComposer — text-only ingest', () => {
  it('useIngestText is called with the note text and useIngestFile is not called', async () => {
    const textHook = makeUseIngestText([{ kind: 'asset_committed', asset_id: 'a1' }]);
    const fileHook = makeUseIngestFile({ kind: 'committed', asset: {} });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(textHook as never);
    renderComposer();

    const noteInput = screen.getByLabelText(/note/i) as HTMLTextAreaElement;
    fireEvent.change(noteInput, { target: { value: '2020 Toyota Prius' } });
    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await vi.waitFor(() => {
      expect(textHook.mutateAsync).toHaveBeenCalledWith({ text: '2020 Toyota Prius' });
      expect(fileHook.mutateAsync).not.toHaveBeenCalled();
    });
  });
});

describe('AddComposer — duplicate (409) opens the dialog', () => {
  it('attaching a file then submitting with a duplicate result opens the dialog', async () => {
    const fileHook = makeUseIngestFile({ kind: 'duplicate', report: DUPLICATE_REPORT });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    vi.mocked(hooks.useKeepDocument).mockReturnValue(makeUseChoice() as never);
    vi.mocked(hooks.useReprocessDocument).mockReturnValue(makeUseChoice() as never);
    renderComposer();

    const file = makeFile('photo.jpg', 'image/jpeg');
    await pickFile('[data-testid="composer-file-input"]', file);
    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await screen.findByRole('dialog');
    expect(screen.getByText('Duplicate detected')).toBeInTheDocument();
    expect(screen.getByText('existing-invoice.pdf')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /reprocess/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /keep existing/i })).toBeInTheDocument();
    // The file hook fired exactly once for the duplicate.
    expect(fileHook.mutateAsync).toHaveBeenCalledTimes(1);
  });
});

describe('AddComposer — accessibility', () => {
  it('the open composer body passes axe (no violations)', async () => {
    const { container } = renderComposer();
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });
});

describe('AddComposer — focus restore', () => {
  it('returns focus to the external trigger when the composer closes', () => {
    vi.clearAllMocks();
    vi.mocked(hooks.useIngestFile).mockReturnValue(makeUseIngestFile({ kind: 'committed', asset: {} }) as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([{ kind: 'asset_committed' }]) as never);

    const setOpen = vi.fn();
    const focusSpy = vi.fn();
    // Harness: an external trigger + the controlled composer.
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <MemoryRouter>
          <ActiveUserProvider queryClient={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
            <button id="composer-trigger" onClick={() => setOpen(false)}>Add something</button>
            <AddComposer open onOpenChange={setOpen} />
          </ActiveUserProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    // Spy on the trigger's focus() BEFORE effects run (the restore on close
    // calls el.focus()).
    const trigger = document.getElementById('composer-trigger') as HTMLButtonElement;
    (trigger as HTMLButtonElement).focus = focusSpy as unknown as () => void;

    // Simulate the user having clicked the trigger to open it: the trigger is
    // the focused element when the composer opens, so it must be captured and
    // restored on close.
    act(() => {
      trigger.focus();
    });

    // On open, focus has moved into the composer (inside the dialog/sheet).
    const noteBox = document.getElementById('composer-note');
    expect(noteBox).not.toBeNull();

    // Close the composer and flush effects.
    act(() => {
      setOpen(false);
    });

    // Focus was restored to the external trigger.
    expect(focusSpy).toHaveBeenCalledTimes(1);
  });
});

describe('AddComposer — multi-file submit (CRIT-D2-01)', () => {
  function makeFiles(count: number): File[] {
    return Array.from({ length: count }, (_, i) => makeFile(`doc-${i + 1}.pdf`, 'application/pdf'));
  }

  async function pickFiles(selector: string, files: File[]): Promise<void> {
    const input = document.querySelector(selector) as HTMLInputElement | null;
    if (input === null) throw new Error(`file input not found: ${selector}`);
    Object.defineProperty(input, 'files', { value: files, configurable: true });
    await act(async () => {
      input.dispatchEvent(new Event('change', { bubbles: true }));
    });
  }

  it('attaching 2 files ingests BOTH (mutation called twice, no false single-add)', async () => {
    const fileHook = makeUseIngestFile({ kind: 'committed', asset: {} });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    renderComposer();

    const [f1, f2] = makeFiles(2);
    await pickFiles('[data-testid="composer-file-input"]', [f1, f2]);
    expect(screen.getByText('doc-1.pdf')).toBeInTheDocument();
    expect(screen.getByText('doc-2.pdf')).toBeInTheDocument();

    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await vi.waitFor(() => {
      expect(fileHook.mutateAsync).toHaveBeenCalledTimes(2);
    });
    expect(fileHook.mutateAsync).toHaveBeenCalledWith({ file: f1, note: undefined });
    expect(fileHook.mutateAsync).toHaveBeenCalledWith({ file: f2, note: undefined });
  });

  it('2 files added, 1 removed → only the remaining file is ingested (called once)', async () => {
    const fileHook = makeUseIngestFile({ kind: 'committed', asset: {} });
    vi.mocked(hooks.useIngestFile).mockReturnValue(fileHook as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    renderComposer();

    const [f1, f2] = makeFiles(2);
    await pickFiles('[data-testid="composer-file-input"]', [f1, f2]);

    // Remove the first chip; only the second remains at submit.
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /remove doc-1\.pdf/i }));
    });
    expect(screen.queryByText('doc-1.pdf')).not.toBeInTheDocument();
    expect(screen.getByText('doc-2.pdf')).toBeInTheDocument();

    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    await vi.waitFor(() => {
      expect(fileHook.mutateAsync).toHaveBeenCalledTimes(1);
    });
    expect(fileHook.mutateAsync).toHaveBeenCalledWith({ file: f2, note: undefined });
  });

  it('a duplicate among 2 files surfaces the dialog AND ingests the other (no silent drop)', async () => {
    const committed = { kind: 'committed', asset: {} };
    const duplicateResult = { kind: 'duplicate', report: DUPLICATE_REPORT };
    let call = 0;
    const fileMutate = vi.fn(async (_vars: unknown) => {
      call += 1;
      return call === 1 ? committed : duplicateResult;
    });
    vi.mocked(hooks.useIngestFile).mockReturnValue({
      mutate: fileMutate,
      mutateAsync: fileMutate,
      isPending: false,
      isSuccess: false,
      error: null,
    } as never);
    vi.mocked(hooks.useIngestText).mockReturnValue(makeUseIngestText([]) as never);
    vi.mocked(hooks.useKeepDocument).mockReturnValue(makeUseChoice() as never);
    vi.mocked(hooks.useReprocessDocument).mockReturnValue(makeUseChoice() as never);
    renderComposer();

    const [f1, f2] = makeFiles(2);
    await pickFiles('[data-testid="composer-file-input"]', [f1, f2]);
    const addBtn = screen.getByRole('button', { name: 'Add' });
    await act(async () => {
      fireEvent.click(addBtn);
    });

    // Both files were ingested (no silent drop) and the dup dialog opened.
    await screen.findByRole('dialog');
    expect(screen.getByText('Duplicate detected')).toBeInTheDocument();
    await vi.waitFor(() => {
      expect(fileMutate).toHaveBeenCalledTimes(2);
    });
    // The committed file's chip is removed; the dup chip + dialog remain.
    expect(screen.queryByText('doc-1.pdf')).not.toBeInTheDocument();
  });
});
