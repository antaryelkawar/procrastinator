import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useMediaQuery } from '@/features/docs/composer/use-media-query';
import { ReviewChips, type ReviewChipField } from '@/features/docs/composer/review-chips';
import { useCreateAccount } from '@/features/finance/hooks';

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

/** Account types the backend `createAccount` accepts (closed enum). */
const ACCOUNT_TYPES: readonly string[] = ['bank', 'wallet', 'cash', 'credit_card'];

/**
 * AccountComposerFlow (task 14.5 / asset-management-v2) — the directive-first
 * account creation flow wired to the /finance/accounts corner [+] and to the
 * "Add account" AddButton.
 *
 * This is a MANUAL describe → review flow: there is NO LLM ingest/extraction
 * step, so this component deliberately does NOT import or call
 * `useIngestFile` / `useIngestText` (nor `toReviewChips`). The four account
 * fields (name / type / currency / institution) are fixed account attributes,
 * built by hand from the describe text, and passed directly to the shared
 * `<ReviewChips />` as visible chips. Confirming creates the account through
 * the existing `useCreateAccount` mutation (`type`, not `account_type`, on the
 * wire — see `CreateAccountRequest`). The optional Note textarea has no
 * dedicated account field in `CreateAccountRequest`, so it is wired to the
 * persisted optional free-text `external_descriptor` slot (empty → omitted);
 * see `handleConfirm`.
 *
 * The dialog/sheet is CONTROLLED by the caller; this flow owns ALL of its
 * internal state (step, describe text, note, chips, finalize flag) so state
 * survives the wrapper swap.
 */
export function AccountComposerFlow({
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
  const [submitted, setSubmitted] = useState(false);
  const [finalizing, setFinalizing] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

  const createAccount = useCreateAccount();

  /** Close the whole flow (controlled) after any finalization/dismiss. */
  const close = useCallback(
    () => {
      onOpenChange(false);
    },
    [onOpenChange],
  );

  /**
   * Step 1 → Step 2. Advance to the review-chips view. The four account chips
   * are built by hand from the describe text (no ingestion). The describe text
   * is KEPT on the chip (name is derived from it); only the inline error is
   * cleared on entry.
   */
  const handleContinue = useCallback(() => {
    setError(null);
    setSubmitted(true);
  }, []);

  /**
   * Step 2 → create. Map the confirmed chip values to the wire request.
   * `name`, `account_type`, and `currency` are required; an empty institution
   * is omitted (sent as `undefined`).
   */
  const handleConfirm = useCallback(
    (values: Readonly<Record<string, string>>) => {
      const name = (values.name ?? '').trim();
      const accountType = (values.account_type ?? '').trim().toLowerCase();
      const currency = (values.currency ?? '').trim();
      const institution = (values.institution ?? '').trim();
      // The optional Note has no dedicated field on the account wire
      // (create_account_request = name/type/currency/institution/external_descriptor).
      // external_descriptor is the persisted optional free-text slot, so the note
      // maps there; an empty note is omitted (sent as undefined).
      const noteValue = note.trim() === '' ? undefined : note.trim();

      if (!name || !accountType || !currency) {
        setError('Name, Type, and Currency are required');
        return;
      }
      if (!ACCOUNT_TYPES.includes(accountType)) {
        setError(`Type must be one of: ${ACCOUNT_TYPES.join(', ')}.`);
        return;
      }

      setFinalizing(true);
      setError(null);
      createAccount.mutate(
        {
          name,
          type: accountType,
          currency,
          institution: institution || undefined,
          external_descriptor: noteValue,
        },
        {
          onSuccess: () => {
            toast.success('Account created');
            close();
          },
          onError: (e) => {
            setError(friendlyMessage(e));
            setFinalizing(false);
          },
        },
      );
    },
    [createAccount, close, note],
  );

  const accountFields: ReviewChipField[] = useMemo(
    () => [
      { key: 'name', label: 'Name', value: describeText.trim() },
      { key: 'account_type', label: 'Type', value: '' },
      { key: 'currency', label: 'Currency', value: '' },
      { key: 'institution', label: 'Institution', value: '' },
    ],
    [describeText],
  );

  const hasDescribe = describeText.trim() !== '';

  const footer = (
    <DialogFooter>
      <Button type="button" variant="ghost" onClick={close} className="min-h-11">
        Cancel
      </Button>
      <Button
        type="button"
        onClick={() => void handleContinue()}
        disabled={!hasDescribe}
        className="min-h-11"
      >
        Continue
      </Button>
    </DialogFooter>
  );

  // ---- Step 1: describe surface -----------------------------------------
  const describeSurface = (
    <div className="w-full space-y-4">
      {/* describe input — the FIRST, most prominent interaction */}
      <div className="space-y-2">
        <label htmlFor="describe-input" className="text-sm font-medium">
          Describe the account
        </label>
        <Textarea
          id="describe-input"
          aria-label="Describe the account"
          placeholder="Describe the account — name, institution, anything you remember"
          value={describeText}
          onChange={(e) => setDescribeText(e.target.value)}
        />
      </div>

      {/* optional free-text note */}
      <div className="space-y-2">
        <label htmlFor="describe-note" className="flex items-center gap-1 text-sm font-medium">
          Note{' '}
          <span className="font-normal text-muted-foreground">(optional)</span>
        </label>
        <Textarea
          id="describe-note"
          aria-label="Note (optional)"
          placeholder="Add a note (optional)"
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
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <ReviewChips
        fields={accountFields}
        onConfirm={handleConfirm}
        onCancel={close}
        busy={finalizing}
        title="Review before you save"
      />
      <p className="text-xs text-muted-foreground">
        Type: bank, wallet, cash, or credit card.
      </p>
    </div>
  );

  const body = submitted ? chipsStep : describeSurface;

  const content = isDesktop ? (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add an account</DialogTitle>
          <DialogDescription>{submitted ? '' : 'Describe the account, then review the details.'}</DialogDescription>
        </DialogHeader>
        {body}
        {!submitted ? footer : null}
      </DialogContent>
    </Dialog>
  ) : (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="bottom" className="max-h-[90vh] overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Add an account</SheetTitle>
          <SheetDescription>{submitted ? '' : 'Describe the account, then review the details.'}</SheetDescription>
        </SheetHeader>
        <div className="mt-2 px-4">{body}</div>
        {!submitted ? <SheetFooter>{footer}</SheetFooter> : null}
      </SheetContent>
    </Sheet>
  );

  return content;
}
