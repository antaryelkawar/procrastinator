/**
 * Duplicate-prompt dialog (task 7.3 / design D4).
 *
 * Rendered from a `DuplicateReport` returned by a 409 document upload. It shows
 * the existing document's filename + upload date, an optional "View existing
 * asset" link, and two choices: **Reprocess** (primary, POST `{reprocess_uri}`)
 * and **Keep existing** (secondary, POST `{keep_uri}`).
 *
 * Timeout behavior (client-side, verifiable with fake timers): a timer is
 * started for `expires_at - now` when the dialog mounts. If the user has not
 * chosen by the expiry, the dialog automatically calls the keep endpoint AND
 * fires a sonner toast whose text is the report's verbatim `timeout_toast`
 * string (do not reword).
 */
import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { useKeepDocument, useReprocessDocument } from './hooks';
import { parseDocumentChoiceUri } from '@/lib/api/client';
import type { DuplicateReport } from '@/lib/api/generated/orval/procrastinator';

export interface DuplicatePromptDialogProps {
  /** The structured 409 report driving the dialog. */
  report: DuplicateReport;
  /** Called after a choice (or timeout keep) resolves, or when the user closes. */
  onClose: () => void;
  /** Called after the keep endpoint resolves (user choice or timeout). */
  onKept: () => void;
  /** Called after the reprocess endpoint resolves. */
  onReprocessed: () => void;
}

/** Format an ISO date-time as a locale date (e.g. "Sep 1, 2026"). */
function formatDate(iso: string): string {
  try {
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'long' }).format(new Date(iso));
  } catch {
    return iso;
  }
}

export function DuplicatePromptDialog({
  report,
  onClose,
  onKept,
  onReprocessed,
}: DuplicatePromptDialogProps) {
  const keep = useKeepDocument();
  const reprocess = useReprocessDocument();
  const [deciding, setDeciding] = useState(false);

  // Resolve the target document + user once from the report's URIs.
  const keepTarget = useRef(parseDocumentChoiceUri(report.prompt.keep_uri, 'keep'));
  const reprocessTarget = useRef(parseDocumentChoiceUri(report.prompt.reprocess_uri, 'reprocess'));

  const doKeep = useCallback(async () => {
    const target = keepTarget.current;
    if (target === null) {
      onClose();
      return;
    }
    setDeciding(true);
    try {
      await keep.mutateAsync({ userId: target.userId, documentId: target.documentId });
      onKept();
    } catch {
      // Keep failed (network/404 etc.) — surface the error and let the user retry.
      setDeciding(false);
      toast.error('Couldn’t keep the existing document. Try again.');
    }
  }, [keep, onClose, onKept]);

  const doReprocess = useCallback(async () => {
    const target = reprocessTarget.current;
    if (target === null) {
      onClose();
      return;
    }
    setDeciding(true);
    try {
      await reprocess.mutateAsync({ userId: target.userId, documentId: target.documentId });
      onReprocessed();
    } catch {
      setDeciding(false);
      toast.error('Couldn’t reprocess the document. Try again.');
    }
  }, [reprocess, onClose, onReprocessed]);

  // Timeout: auto-keep when the prompt expires and the user hasn't chosen.
  useEffect(() => {
    const expiry = new Date(report.prompt.expires_at).getTime();
    const delay = expiry - Date.now();
    if (!Number.isFinite(delay)) {
      return;
    }
    const timer = window.setTimeout(() => {
      void doKeep();
      // Verbatim timeout toast — the report's own copy, not reworded.
      toast(report.prompt.timeout_toast, {
        description: 'No response before the prompt expired.',
      });
    }, Math.max(delay, 0));
    return () => window.clearTimeout(timer);
  }, [doKeep, report.prompt.expires_at, report.prompt.timeout_toast]);

  const isBusy = deciding || keep.isPending || reprocess.isPending;

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Duplicate detected</DialogTitle>
          <DialogDescription>
            A document matching this upload already exists.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-1">
          <p className="text-sm font-medium">{report.existing_source_filename}</p>
          <p className="text-xs text-muted-foreground">
            Uploaded {formatDate(report.existing_source_uploaded_at)}
          </p>
          {report.existing_asset_id ? (
            <Link
              to={`/assets/${report.existing_asset_id}`}
              className="text-sm font-medium text-primary underline-offset-4 hover:underline"
            >
              View existing asset
            </Link>
          ) : null}
        </div>

        <p className="text-xs text-muted-foreground">
          No response? {report.prompt.timeout_toast} (at {formatDate(report.prompt.expires_at)})
        </p>

        <DialogFooter>
          <Button variant="outline" onClick={() => void doKeep()} disabled={isBusy}>
            Keep existing
          </Button>
          <Button onClick={() => void doReprocess()} disabled={isBusy}>
            {reprocess.isPending ? 'Reprocessing…' : 'Reprocess'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
