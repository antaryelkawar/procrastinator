import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Camera, Trash2, Upload } from 'lucide-react';
import { toast } from 'sonner';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useMediaQuery } from './use-media-query';
import { DuplicatePromptDialog } from './duplicate-prompt-dialog';
import { useIngestFile, useIngestText } from './hooks';
import type { DuplicateReport } from '@/lib/api/generated/orval/procrastinator';

export interface AddComposerProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly title?: string;
}

interface Attachment {
  id: string;
  file: File;
}

/** Extract a friendly message from an unknown error value. */
function friendlyMessage(error: unknown): string {
  if (error && typeof error === 'object' && 'message' in error && (error as { message?: unknown }).message) {
    return String((error as { message: unknown }).message);
  }
  return 'Something went wrong. Try again.';
}

/**
 * AddComposer (task 14.1 / design D11 — revision): a single, simple "Add"
 * surface — one primary upload control (click to browse or drag-and-drop), a
 * camera action, and ONE optional free-text note that doubles as the text-only
 * content when no file is attached. It is *not* a structured field-by-field
 * form.
 *
 * It is a *controlled* dialog/sheet — the caller owns the trigger; this
 * component owns ALL of its internal state (note, file attachments, duplicate
 * report, submitting/error flags) so state survives the wrapper swap.
 */
export function AddComposer({ open, onOpenChange, title = 'Add something' }: AddComposerProps) {
  const [note, setNote] = useState('');
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [duplicateReport, setDuplicateReport] = useState<DuplicateReport | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);

  const isDesktop = useMediaQuery('(min-width: 640px)');

  // Focus restore (WCAG 2.4.3): the trigger is an external button (not a
  // DialogTrigger), so Radix can't auto-restore focus. Capture the focused
  // element in a layout effect (runs before Radix's passive focus management)
  // while open, then return focus to it when the composer closes.
  const lastFocused = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    if (open) {
      lastFocused.current =
        document.activeElement instanceof HTMLElement ? document.activeElement : null;
    }
  }, [open]);
  useEffect(() => {
    if (!open) {
      const el = lastFocused.current;
      if (el && document.contains(el) && typeof el.focus === 'function') {
        el.focus();
      }
      lastFocused.current = null;
    }
  }, [open]);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const cameraInputRef = useRef<HTMLInputElement>(null);

  const ingestFile = useIngestFile();
  const ingestText = useIngestText();

  const hasSomething = useCallback((): boolean => {
    return attachments.length > 0 || note.trim() !== '';
  }, [attachments, note]);

  const reset = useCallback(() => {
    setNote('');
    setAttachments([]);
    setError(null);
  }, []);

  const removeAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  }, []);

  const addFiles = useCallback((files: File[]) => {
    const clean = files.filter((f) => f instanceof File && f.name !== '');
    if (clean.length === 0) return;
    setAttachments((prev) => [...prev, ...clean.map((file) => ({ id: crypto.randomUUID(), file }))]);
  }, []);

  const handleCameraChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      if (file !== undefined) {
        addFiles([file]);
        event.target.value = '';
      }
    },
    [addFiles],
  );

  const handleFileChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(event.target.files ?? []);
      addFiles(files);
      event.target.value = '';
    },
    [addFiles],
  );

  const handleSubmit = useCallback(async () => {
    if (submitting) return;
    if (!hasSomething()) return;

    const noteValue = note.trim() === '' ? undefined : note.trim();

    setSubmitting(true);
    setError(null);
    try {
      // File path (covers both the drag/drop file chips and the camera chip).
      if (attachments.length > 0) {
        // Ingest EVERY attachment — each file gets its own upload. (CRIT-D2-01:
        // the previous loop `return`ed after the first attachment, silently
        // dropping the rest behind a false "Added item" toast.)
        const doneIds = new Set<string>();
        let heldCount = 0;
        let committedCount = 0;
        let duplicate: DuplicateReport | null = null;
        const failures: string[] = [];

        for (const a of attachments) {
          try {
            const result = await ingestFile.mutateAsync({ file: a.file, note: noteValue });
            if (result.kind === 'duplicate') {
              if (duplicate === null) duplicate = result.report;
              // Keep this chip: it resolves through the duplicate dialog.
            } else if (result.kind === 'held') {
              doneIds.add(a.id);
              heldCount += 1;
            } else {
              doneIds.add(a.id);
              committedCount += 1;
            }
          } catch (e) {
            failures.push(friendlyMessage(e));
            // Keep the chip so a failed upload can be retried.
          }
        }

        // Remove only the chips that were ingested (committed or held); keep
        // the failed/duplicate chips so they can be retried.
        if (doneIds.size > 0) {
          setAttachments((prev) => prev.filter((a) => !doneIds.has(a.id)));
        }

        // Surface every failure — no silent drop, no false success.
        if (failures.length > 0) {
          setError(
            failures.length === 1 ? failures[0] : `${failures.length} files could not be uploaded.`,
          );
        }

        // A 409 duplicate takes priority: surface the reprocess/keep dialog for
        // the offending file without claiming the rest were added.
        if (duplicate !== null) {
          setDuplicateReport(duplicate);
        }

        // Aggregate success feedback — only on a clean batch (no dup, no
        // failure) so a partial submission never masquerades as full success.
        if (failures.length === 0 && duplicate === null && committedCount + heldCount > 0) {
          if (committedCount > 0) {
            toast.success(committedCount === 1 ? 'Added item' : `Added ${committedCount} items`);
          }
          if (heldCount > 0) {
            toast(heldCount === 1 ? 'Held for review' : `${heldCount} held for review`);
          }
          setNote('');
        }
        return;
      }

      // Text-only path.
      if (noteValue === undefined) return;

      await ingestText.mutateAsync({ text: noteValue });
      toast.success('Added item');
      reset();
    } catch (e) {
      setError(friendlyMessage(e));
    } finally {
      setSubmitting(false);
    }
  }, [submitting, hasSomething, attachments, note, ingestFile, ingestText, reset]);

  const footer = (
    <DialogFooter>
      <Button
        onClick={() => void handleSubmit()}
        disabled={!hasSomething() || submitting}
        aria-busy={submitting}
        className="min-h-11"
      >
        {submitting ? 'Adding…' : 'Add'}
      </Button>
    </DialogFooter>
  );

  const uploadSurface = (
    <button
      type="button"
      className={`flex w-full items-center gap-3 rounded-lg border px-4 py-3 text-sm text-muted-foreground transition-colors ${
        dragging
          ? 'border-ring ring-2 ring-ring'
          : 'border-dashed border-input hover:border-ring hover:text-foreground'
      } focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`}
      aria-label="Upload a file"
      onClick={() => fileInputRef.current?.click()}
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) addFiles(files);
      }}
    >
      <Upload className="size-5 shrink-0" aria-hidden="true" />
      <span>Drop files here, or click to browse</span>
    </button>
  );

  const body = (
    <div className="w-full space-y-4">
      {/* 1 + 2. Upload surface + camera action (same row) */}
      <div className="flex items-stretch gap-2">
        <div className="flex-1">{uploadSurface}</div>
        <button
          type="button"
          aria-label="Add from camera"
          onClick={() => cameraInputRef.current?.click()}
          className="flex size-11 shrink-0 items-center justify-center rounded-lg border border-input bg-muted/50 text-muted-foreground transition-colors hover:border-ring hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Camera className="size-5" aria-hidden="true" />
        </button>
      </div>

      {/* Hidden file pickers */}
      <input
        ref={cameraInputRef}
        type="file"
        accept="image/*"
        capture="environment"
        className="hidden"
        data-testid="composer-camera-input"
        onChange={handleCameraChange}
      />
      <input
        ref={fileInputRef}
        type="file"
        multiple
        className="hidden"
        data-testid="composer-file-input"
        onChange={handleFileChange}
      />

      {/* Removable file attachments */}
      {attachments.length > 0 ? (
        <ul className="flex flex-wrap gap-2">
          {attachments.map((a) => (
            <li
              key={a.id}
              className="flex items-center gap-2 rounded-full border border-input bg-muted/50 px-3 py-1"
            >
              <span className="max-w-[200px] truncate text-sm" title={a.file.name}>
                {a.file.name}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={`Remove ${a.file.name}`}
                onClick={() => removeAttachment(a.id)}
              >
                <Trash2 className="size-4" aria-hidden="true" />
              </Button>
            </li>
          ))}
        </ul>
      ) : null}

      {/* 3. Optional free-text note (replaces the describe textarea + paste input) */}
      <div className="space-y-2">
        <label htmlFor="composer-note" className="flex items-center gap-1 text-sm font-medium">
          Note{' '}
          <span className="font-normal text-muted-foreground">(optional)</span>
        </label>
        <Textarea
          id="composer-note"
          placeholder="Describe it, or add a note for the scanner (optional)"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>

      {/* Error state */}
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  );

  const content = isDesktop ? (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>Upload anything, or add a note.</DialogDescription>
        </DialogHeader>
        {body}
        {footer}
      </DialogContent>
    </Dialog>
  ) : (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="bottom" className="max-h-[90vh] overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>Upload anything, or add a note.</SheetDescription>
        </SheetHeader>
        <div className="mt-2 px-4">{body}</div>
        <SheetFooter>{footer}</SheetFooter>
      </SheetContent>
    </Sheet>
  );

  // The DuplicatePromptDialog is rendered as a SIBLING (not nested) so the
  // two Radix overlays don't fight over focus/portal ownership.
  return (
    <>
      {content}
      {duplicateReport !== null ? (
        <DuplicatePromptDialog
          report={duplicateReport}
          onClose={() => setDuplicateReport(null)}
          onKept={() => {
            toast('Kept existing document');
            setDuplicateReport(null);
          }}
          onReprocessed={() => {
            toast('Reprocessing document…');
            setDuplicateReport(null);
          }}
        />
      ) : null}
    </>
  );
}
