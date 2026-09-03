import type { ReactNode } from 'react';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';

export interface ConfirmDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly title: string;
  readonly description?: ReactNode;
  readonly confirmLabel?: string;
  readonly cancelLabel?: string;
  /**
   * True while the confirmed action is still in flight: the dialog cannot be
   * dismissed (confirm re-dispatches nothing; Escape/overlay/cancel are
   * ignored) until the caller clears it and closes the dialog.
   */
  readonly busy?: boolean;
  /** Render the confirm button with destructive styling. */
  readonly destructive?: boolean;
  /** Invoked when the user confirms. */
  readonly onConfirm: () => void;
}

/**
 * Shared confirmation dialog for user-initiated destructive actions (spec:
 * "Destructive actions always confirm"; design D8: the single shared
 * ConfirmDialog over alert-dialog, used by movement delete and batch
 * commit/discard).
 *
 * Fully controlled: the caller owns `open`/`onOpenChange`. Confirm fires
 * `onConfirm` and then closes — immediately when not `busy`, or when the
 * caller releases `busy` and closes after an in-flight mutation settles.
 * Cancel / Escape / overlay click simply close.
 */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  busy = false,
  destructive = false,
  onConfirm,
}: ConfirmDialogProps) {
  const handleOpenChange = (next: boolean): void => {
    if (next && busy) {
      // Defensive: the trigger is caller-side and the dialog is only ever
      // opened by explicit action; keep the in-flight dialog open.
      return;
    }
    onOpenChange(next);
  };

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent size="sm">
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description !== undefined ? (
            <AlertDialogDescription>{description}</AlertDialogDescription>
          ) : null}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>{cancelLabel}</AlertDialogCancel>
          <AlertDialogAction
            variant={destructive ? 'destructive' : 'default'}
            disabled={busy}
            onClick={onConfirm}
          >
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
