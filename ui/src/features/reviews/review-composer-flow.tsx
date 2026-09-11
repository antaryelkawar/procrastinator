import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Camera, Trash2, Upload } from 'lucide-react';
import { toast } from 'sonner';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useMediaQuery } from '@/features/docs/composer/use-media-query';
import { useIngestFile, useIngestText } from '@/features/docs/composer/hooks';
import { ReviewChips, toReviewChips } from '@/features/docs/composer/review-chips';
import { DuplicatePromptDialog } from '@/features/docs/composer/duplicate-prompt-dialog';
import * as client from '@/lib/api/client';
import { useActiveUser } from '@/context/active-user';
import type { DuplicateReport, IngestReview } from '@/lib/api/generated/orval/procrastinator';

interface Attachment {
  id: string;
  file: File;
}

/**
 * Asset fields the existing patch endpoint can carry (`PatchAssetRequest`).
 * purchase_date / warranty_end are NOT patchable, so they are intentionally
 * absent — the extraction pipeline owns those.
 */
const PATCH_KEYS = [
  'name',
  'asset_category',
  'brand',
  'model',
  'serial_number',
  'price',
  'currency',
] as const;

/** Extract a friendly message from an unknown error value. */
function friendlyMessage(error: unknown): string {
  if (
    error &&
    typeof error === 'object' &&
    'message' in error &&
    (error as { message?: unknown }).message
  ) {
    return String((error as { message: unknown }).message);
  }
  return 'Something went wrong. Try again.';
}

/**
 * ReviewComposerFlow (task 14.5 / design D11, review-ingest spec) — the
 * directive-first review-ingest composition flow wired to the
 * `/ingest/reviews` corner [+] (the page's "Add review" control).
 *
 * The FIRST interaction is ONE describe-input — no candidate/merchant/amount
 * form. Submit runs extraction through the EXISTING ingest path (text via
 * useIngestText, file via useIngestFile); there is no dedicated create-endpoint.
 * Extraction then yields one of:
 *
 *   - `held_for_review` → fetch the review (getReview) and advance to the
 *     review-chips view; confirming the chips finalizes through approveReview
 *     (then patchAsset when the confirmed values differ from the candidate).
 *   - `asset_committed` → an asset was created directly, so toast + close
 *     (there is no review to confirm, so no chips and no patch).
 *   - `duplicate` (text only) → surface it directly.
 *
 * The file path additionally yields a 409 structured `DuplicateReport`, which
 * renders the shared DuplicatePromptDialog (the duplicate-serial path).
 *
 * The dialog/sheet is CONTROLLED by the caller; this flow owns ALL of its
 * internal state (step, note, describe text, attachments, extract/refresh/finalize
 * flags, duplicate report) so state survives the wrapper swap.
 */
export function ReviewComposerFlow({
  open,
  onOpenChange,
  onClosed,
}: {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** Called (in addition to `onOpenChange(false)`) after the overlay unmounts. */
  readonly onClosed?: () => void;
}) {
  const [describeText, setDescribeText] = useState('');
  const [note, setNote] = useState('');
  const [attachments, setAttachments] = useState<Attachment[]>([]);

  const [loading, setLoading] = useState(false);
  const [finalizing, setFinalizing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Chips state.
  const [fields, setFields] = useState<ReturnType<typeof toReviewChips>>([]);
  const [optionalFields, setOptionalFields] = useState<ReturnType<typeof toReviewChips>>([]);

  // Commit target resolved at submit time (from the ingest outcome).
  const [reviewId, setReviewId] = useState<string | null>(null);

  const [duplicateReport, setDuplicateReport] = useState<DuplicateReport | null>(null);

  const isDesktop = useMediaQuery('(min-width: 640px)');

  // Focus restore (WCAG 2.4.3): the trigger is external (not a DialogTrigger),
  // so Radix can't auto-restore focus. Capture in a layout effect (runs before
  // Radix's passive focus management) while open, return on close.
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
      onClosed?.();
    }
  }, [open, onClosed]);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const cameraInputRef = useRef<HTMLInputElement>(null);

  const ingestFile = useIngestFile();
  const ingestText = useIngestText();
  const user = useActiveUser();

  /**
   * Clear ONLY the describe-step inputs. The extracted chips (`fields`) and the
   * commit target (`reviewId`) are intentionally KEPT so the flow advances to
   * the review-chips step instead of falling back to the describe surface.
   * (A full reset is unnecessary — the flow is controlled and remounts clean on
   * each open.)
   */
  const clearDescribe = useCallback(() => {
    setDescribeText('');
    setNote('');
    setAttachments([]);
    setError(null);
  }, []);

  /** Close the whole flow (controlled) after any finalization/dismiss. */
  const close = useCallback(
    () => {
      onOpenChange(false);
    },
    [onOpenChange],
  );

  const removeAttachment = useCallback((id: string) => {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  }, []);

  const addFiles = useCallback((files: File[]) => {
    const clean = files.filter((f) => f instanceof File && f.name !== '');
    if (clean.length === 0) return;
    setAttachments((prev) => [...prev, ...clean.map((file) => ({ id: crypto.randomUUID(), file }))]);
  }, []);

  const handleFileChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(event.target.files ?? []);
      addFiles(files);
      event.target.value = '';
    },
    [addFiles],
  );

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

  /**
   * Step 1 → Step 2. Run extraction through the EXISTING ingest path and
   * advance to the review-chips view. No new endpoints.
   */
  const handleSubmit = useCallback(async () => {
    if (loading) return;

    const describeValue = describeText.trim();
    const noteValue = note.trim() === '' ? undefined : note.trim();
    // Nothing to submit: the describe text is the directive; at least one file
    // also qualifies. This keeps the FIRST input meaningful.
    if (describeValue === '' && attachments.length === 0) return;

    setLoading(true);
    setError(null);
    try {
      // File path (covers the camera chip + the browse/drag drop chips).
      if (attachments.length > 0) {
        // Ingest EVERY attachment — each file gets its own upload. (CRIT-D2-01:
        // the previous loop `return`ed after the first attachment, silently
        // dropping the rest behind a false "Committed" toast.)
        const doneIds = new Set<string>();
        let firstHeld: IngestReview | null = null;
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
              if (firstHeld === null) firstHeld = result.review;
            } else {
              doneIds.add(a.id);
              committedCount += 1;
            }
          } catch (e) {
            failures.push(friendlyMessage(e));
            // Keep the chip so a failed upload can be retried.
          }
        }

        // Remove the chips that were ingested (committed/held) so a failed/dup
        // chip can be retried without re-submitting the ones already through.
        if (doneIds.size > 0) {
          setAttachments((prev) => prev.filter((a) => !doneIds.has(a.id)));
        }

        // Surface every failure — no silent drop, no false success.
        if (failures.length > 0) {
          setError(
            failures.length === 1 ? failures[0] : `${failures.length} files could not be uploaded.`,
          );
        }

        // A 409 duplicate takes priority: surface the reprocess/keep dialog
        // without advancing to chips or closing.
        if (duplicate !== null) {
          setDuplicateReport(duplicate);
          return;
        }

        // A clean batch: a held review needs confirmation → advance to the chips
        // step for the FIRST held result (committed files, if any, are done).
        if (failures.length === 0 && firstHeld !== null) {
          setReviewId(firstHeld.id);
          setFields(toReviewChips(firstHeld.data.candidate_fields, firstHeld.data.confidence));
          setOptionalFields([]);
          clearDescribe();
          return;
        }

        // A clean batch with only committed files: an asset was created directly
        // for each — there is no review to confirm, so toast + close.
        if (failures.length === 0 && committedCount > 0) {
          toast.success(committedCount === 1 ? 'Committed' : `${committedCount} committed`);
          clearDescribe();
          close();
          return;
        }

        // All failed (or nothing to advance to): stay with the error set above.
        return;
      }

      // Text-only path: one outcome list.
      if (describeValue === '') return;

      const outcomes = await ingestText.mutateAsync({ text: describeValue });
      const outcome = outcomes[0];
      if (outcome === undefined) {
        toast.error('Could not ingest. Try again.');
        return;
      }
      if (outcome.kind === 'duplicate') {
        // Text ingest returns a duplicate outcome (not a report); there is no
        // DuplicateReport to drive the modal, so surface it directly.
        toast.error('A matching item already exists.');
        return;
      }
      if (outcome.kind === 'held_for_review') {
        if (outcome.review_id == null) {
          throw new Error('No review id returned.');
        }
        const reviewId = outcome.review_id;
        setReviewId(reviewId);
        const review = await client.getReview(user.activeUser ?? '', reviewId);
        const chips = toReviewChips(review.data.candidate_fields, review.data.confidence);
        setFields(chips);
        setOptionalFields([]);
        clearDescribe();
        return;
      }
      if (outcome.kind === 'asset_committed') {
        // An asset was created directly — there is no review to confirm, so
        // toast + close (no chips, no patch).
        toast.success('Committed');
        clearDescribe();
        setLoading(false);
        close();
        return;
      }
      // statement_preview / failed → friendly error, no chips.
      toast.error('Could not ingest. Try again.');
    } catch (e) {
      setError(friendlyMessage(e));
    } finally {
      setLoading(false);
    }
  }, [loading, describeText, note, attachments, ingestFile, ingestText, clearDescribe, close]);

  /**
   * Step 2 → finalize. Held review → approveReview (candidate_fields = the
   * chips), then patch any user corrections onto the committed asset. No new
   * endpoints.
   */
  const handleConfirm = useCallback(
    async (values: Readonly<Record<string, string>>) => {
      setFinalizing(true);
      setError(null);
      try {
        // Baseline = the extracted candidate values (before any user edit),
        // keyed by field.key. A confirmed value that differs from it is a user
        // correction and the ONLY thing carried to the write path: unedited
        // values already equal the candidate data, so they need no write. This
        // is what makes the persisted payload exactly the confirmed chips.
        const original = Object.fromEntries(fields.map((f) => [f.key, f.value]));
        const body: Record<string, string> = {};
        for (const k of PATCH_KEYS) {
          const v = values[k];
          if (typeof v === 'string' && v !== '' && v !== original[k]) {
            body[k] = v;
          }
        }

        if (reviewId !== null) {
          // Held review: approve to commit (candidate_fields = the chips),
          // then patch any user corrections onto the committed asset.
          const response = await client.approveReview(user.activeUser ?? '', reviewId);
          if (Object.keys(body).length > 0) {
            await client.patchAsset(user.activeUser ?? '', response.asset.id, body as never);
          }
          toast.success('Review approved');
          close();
          return;
        }

        // Fallback (should not happen): nothing to finalize.
        toast.success('Review approved');
        close();
      } catch (e) {
        setError(friendlyMessage(e));
      } finally {
        setFinalizing(false);
      }
    },
    [reviewId, fields, user.activeUser, close],
  );

  const hasContent = describeText.trim() !== '' || attachments.length > 0;

  const footer = (
    <DialogFooter>
      <Button type="button" variant="ghost" onClick={close} className="min-h-11">
        Cancel
      </Button>
      <Button
        type="button"
        onClick={() => void handleSubmit()}
        disabled={!hasContent || loading}
        aria-busy={loading}
        className="min-h-11"
      >
        {loading ? 'Ingesting…' : 'Add'}
      </Button>
    </DialogFooter>
  );

  // ---- Step 1: describe surface -----------------------------------------
  const describeSurface = (
    <div className="w-full space-y-4">
      {/* describe input — the FIRST, most prominent interaction */}
      <div className="space-y-2">
        <label htmlFor="describe-input" className="text-sm font-medium">
          Describe it
        </label>
        <Textarea
          id="describe-input"
          aria-label="Describe it"
          placeholder="Describe it — or attach a document"
          value={describeText}
          onChange={(e) => setDescribeText(e.target.value)}
        />
      </div>

      {/* attach strip: camera / pick file (drag-drop + browse) + note */}
      <div className="flex items-stretch gap-2">
        <button
          type="button"
          aria-label="Attach a file"
          onClick={() => fileInputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
          }}
          onDrop={(e) => {
            e.preventDefault();
            const files = Array.from(e.dataTransfer.files);
            if (files.length > 0) addFiles(files);
          }}
          className="flex flex-1 items-center gap-2 rounded-lg border border-input bg-muted/50 px-4 py-3 text-sm text-muted-foreground transition-colors hover:border-ring hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Upload aria-hidden className="size-5 shrink-0" />
          <span>Attach a file</span>
        </button>
        <button
          type="button"
          aria-label="Take a photo"
          onClick={() => cameraInputRef.current?.click()}
          className="flex size-11 shrink-0 items-center justify-center rounded-lg border border-input bg-muted/50 text-muted-foreground transition-colors hover:border-ring hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Camera aria-hidden className="size-5" />
        </button>
      </div>

      {/* Hidden file pickers (the attach-strip buttons open these). */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        className="hidden"
        data-testid="composer-file-input"
        onChange={handleFileChange}
      />
      <input
        ref={cameraInputRef}
        type="file"
        accept="image/*"
        capture="environment"
        className="hidden"
        data-testid="composer-camera-input"
        onChange={handleCameraChange}
      />

      {/* Removable file attachments (chips) */}
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
                <Trash2 aria-hidden className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      ) : null}

      {/* optional free-text note (the directive that travels with the upload) */}
      <div className="space-y-2">
        <label htmlFor="describe-note" className="flex items-center gap-1 text-sm font-medium">
          Note{' '}
          <span className="font-normal text-muted-foreground">(optional)</span>
        </label>
        <Textarea
          id="describe-note"
          aria-label="Note (optional)"
          placeholder="Add a note for the extractor (optional)"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>

      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  );

  // ---- Step 2: review chips ----------------------------------------------
  const chipsStep = (
    <div className="w-full space-y-4">
      <ReviewChips
        fields={fields}
        optionalFields={optionalFields}
        onConfirm={handleConfirm}
        onCancel={close}
        busy={finalizing}
        title="Review before you save"
      />
    </div>
  );

  const body = fields.length > 0 ? chipsStep : describeSurface;

  const content = isDesktop ? (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add a review</DialogTitle>
          <DialogDescription>{fields.length > 0 ? '' : 'Describe it, or attach a document.'}</DialogDescription>
        </DialogHeader>
        {body}
        {fields.length === 0 ? footer : null}
      </DialogContent>
    </Dialog>
  ) : (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="bottom" className="max-h-[90vh] overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Add a review</SheetTitle>
          <SheetDescription>{fields.length > 0 ? '' : 'Describe it, or attach a document.'}</SheetDescription>
        </SheetHeader>
        <div className="mt-2 px-4">{body}</div>
        {fields.length === 0 ? <SheetFooter>{footer}</SheetFooter> : null}
      </SheetContent>
    </Sheet>
  );

  // A SIBLING (not nested) DuplicatePromptDialog so the two Radix overlays
  // don't fight over focus/portal ownership.
  return (
    <>
      {content}
      {duplicateReport !== null ? (
        <DuplicatePromptDialog
          report={duplicateReport}
          onClose={() => {
            setDuplicateReport(null);
            setLoading(false);
          }}
          onKept={() => {
            toast('Kept existing document');
            setDuplicateReport(null);
            close();
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
