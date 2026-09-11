/**
 * Unified Add surface (task 8.2).
 *
 * One surface, one dropzone, one paste box, one account selector, and a single
 * "Add" CTA. Every item the user drops or pastes is sent through `useAdd`,
 * which POSTs a multipart body and returns a uniform per-item outcome array.
 * The page renders those outcomes in a single summary with per-item
 * navigation:
 *
 *   asset_committed   → /assets/{asset_id}
 *   held_for_review   → /ingest/reviews
 *   duplicate         → /assets/{duplicate_asset_id} (+ Restore when the
 *                       existing asset is soft-deleted)
 *   statement_preview → /finance/import/{import_batch_id}
 *   failed            → no link, the `reason` text
 *
 * Statement files (CSV) require an account: `statement.Service.Upload` is
 * keyed on `account_id`. The client enforces this up front — a CSV in the
 * batch with no account selected shows a validation error and does not call
 * `mutate`.
 */
import { useState } from 'react';
import { Link } from 'react-router';
import { Upload } from '@/features/docs/upload';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useAdd, useRestoreAsset } from '@/features/add/hooks';
import { useAccounts } from '@/features/docs/hooks';
import type { AddItemOutcome } from '@/lib/api/schema';
import type { UploadAccept } from '@/features/docs/upload';

/** Common document + image types the add surface accepts. */
const ACCEPT: UploadAccept = {
  'application/pdf': ['.pdf'],
  'image/png': ['.png'],
  'image/jpeg': ['.jpg', '.jpeg'],
  'text/csv': ['.csv'],
};

/** The human-readable label for each outcome kind. */
const OUTCOME_LABELS: Record<AddItemOutcome['kind'], string> = {
  asset_committed: 'Committed as asset',
  held_for_review: 'Held for review',
  duplicate: 'Duplicate',
  statement_preview: 'Statement preview',
  failed: 'Failed',
};

/** Badge variant per outcome kind (subtle, color-coded). */
const OUTCOME_VARIANTS: Record<AddItemOutcome['kind'], 'default' | 'secondary' | 'destructive' | 'outline'> = {
  asset_committed: 'default',
  held_for_review: 'secondary',
  duplicate: 'secondary',
  statement_preview: 'outline',
  failed: 'destructive',
};

/** True when the file looks like a ledger statement (needs an account). */
function isStatementFile(file: File): boolean {
  const name = file.name.toLowerCase();
  return name.endsWith('.csv') || file.type === 'text/csv';
}

/** A single duplicate outcome's restore affordance (asset was soft-deleted). */
function RestoreOffer({ assetId, onRestored }: { assetId: string; onRestored?: () => void }) {
  const restore = useRestoreAsset();
  const [state, setState] = useState<'idle' | 'done' | 'error'>('idle');
  const [error, setError] = useState<string | null>(null);

  const handleRestore = (): void => {
    setState('idle');
    setError(null);
    restore.mutate({ assetId }, {
      onSuccess: () => {
        setState('done');
        setError(null);
        onRestored?.();
      },
      onError: (err) => {
        setState('error');
        setError(err.message ?? 'Restore failed');
      },
    });
  };

  return (
    <div className="flex items-center gap-2">
      {state === 'done' ? (
        <span role="status" className="text-sm text-emerald-600 dark:text-emerald-400">
          Restored
        </span>
      ) : state === 'error' ? (
        <span role="alert" className="text-sm text-destructive">
          {error}
        </span>
      ) : null}
      <Button
        variant="outline"
        size="sm"
        onClick={handleRestore}
        disabled={restore.isPending || state === 'done'}
      >
        {restore.isPending ? 'Restoring…' : 'Restore'}
      </Button>
    </div>
  );
}

/** The per-item outcome row (badge + navigation + restore/reason). */
function OutcomeRow({ outcome }: { outcome: AddItemOutcome }) {
  const label = OUTCOME_LABELS[outcome.kind];
  const variant = OUTCOME_VARIANTS[outcome.kind];

  let nav: { to: string; label: string } | null = null;
  if (outcome.kind === 'asset_committed' && outcome.asset_id) {
    nav = { to: `/assets/${outcome.asset_id}`, label: 'View asset' };
  } else if (outcome.kind === 'held_for_review') {
    nav = { to: '/ingest/reviews', label: 'View review queue' };
  } else if (outcome.kind === 'duplicate' && outcome.duplicate_asset_id) {
    nav = { to: `/assets/${outcome.duplicate_asset_id}`, label: 'View existing asset' };
  } else if (outcome.kind === 'statement_preview' && outcome.import_batch_id) {
    nav = { to: `/finance/import/${outcome.import_batch_id}`, label: 'Open import batch' };
  }

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border p-3">
      <Badge variant={variant}>{label}</Badge>
      {nav ? (
        <Link to={nav.to} className="text-sm font-medium text-primary underline-offset-4 hover:underline">
          {nav.label}
        </Link>
      ) : null}
      {outcome.kind === 'duplicate' && outcome.asset_deleted && outcome.duplicate_asset_id ? (
        <RestoreOffer assetId={outcome.duplicate_asset_id} />
      ) : null}
      {outcome.kind === 'failed' && outcome.reason ? (
        <span className="text-sm text-muted-foreground">{outcome.reason}</span>
      ) : null}
    </li>
  );
}

/**
 * The unified Add surface. State: queued files, pasted text, selected account,
 * and the client-side validation error. On a successful add the files and text
 * are cleared (the account selection is kept for the next add).
 */
export function AddPage() {
  const [files, setFiles] = useState<File[]>([]);
  const [text, setText] = useState('');
  const [accountId, setAccountId] = useState<string>('');
  const [validationError, setValidationError] = useState<string | null>(null);

  const { data: accounts } = useAccounts();
  const add = useAdd();

  const hasStatement = files.some(isStatementFile);
  const hasSomething = files.length > 0 || text.trim().length > 0;

  const removeFile = (index: number): void => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
    setValidationError(null);
  };

  const handleDrop = (dropped: File[]): void => {
    setFiles((prev) => [...prev, ...dropped]);
    setValidationError(null);
  };

  const handleSubmit = (): void => {
    setValidationError(null);

    // A statement file needs an account; the backend also enforces this (400),
    // but the client check is the observable UI behavior.
    if (hasStatement && accountId === '') {
      setValidationError('Select an account for the statement file(s)');
      return;
    }

    const body: Parameters<typeof add.mutate>[0] = {
      ...(files.length > 0 ? { files } : {}),
      ...(text.trim().length > 0 ? { text: text.trim() } : {}),
      ...(accountId !== '' ? { account_id: accountId } : {}),
    };

    void add.mutateAsync(body).then(() => {
      // Keep the account selection; clear the inputs.
      setFiles([]);
      setText('');
    });
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">Add</h1>

      <section
        aria-labelledby="add-surface-heading"
        className="space-y-5 rounded-lg border p-5"
      >
        <h2 id="add-surface-heading" className="text-base font-semibold">
          Add assets &amp; statements
        </h2>

        {/* a. dropzone */}
        <div className="space-y-2">
          <Upload
            onDrop={handleDrop}
            accept={ACCEPT}
            multiple
            label="file upload"
            hint="Drag & drop documents or images here, or click to select. CSV statements require an account."
          />
          {files.length > 0 ? (
            <ul className="space-y-1" aria-label="Queued files">
              {files.map((file, index) => (
                <li
                  key={`${file.name}-${index}`}
                  className="flex items-center justify-between gap-2 rounded-md border px-3 py-1.5 text-sm"
                >
                  <span className="truncate">
                    {file.name}
                    {isStatementFile(file) ? (
                      <span className="ml-2 text-xs text-muted-foreground">statement</span>
                    ) : null}
                  </span>
                  <Button
                    variant="ghost"
                    size="xs"
                    aria-label={`Remove ${file.name}`}
                    onClick={() => removeFile(index)}
                  >
                    Remove
                  </Button>
                </li>
              ))}
            </ul>
          ) : null}
        </div>

        {/* b. paste text */}
        <div className="space-y-2">
          <Label htmlFor="add-paste-text">Paste text</Label>
          <Textarea
            id="add-paste-text"
            value={text}
            onChange={(event) => {
              setText(event.target.value);
              setValidationError(null);
            }}
            placeholder="Or paste text describing the item (e.g. &quot;2020 Toyota Prius, serial ABC123&quot;)"
            rows={3}
          />
        </div>

        {/* c. account selector */}
        <div className="space-y-2">
          <Label htmlFor="add-account">
            Account
            <span className="ml-2 font-normal text-muted-foreground">
              (required for statement files)
            </span>
          </Label>
          <Select value={accountId} onValueChange={(value) => { setAccountId(value); setValidationError(null); }}>
            <SelectTrigger id="add-account" className="w-full sm:w-[280px]" aria-label="Account">
              <SelectValue placeholder="No account (documents only)" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">No account (documents only)</SelectItem>
              {(accounts ?? []).map((account) => (
                <SelectItem key={account.id} value={account.id}>
                  {account.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {validationError ? (
          <p role="alert" className="text-sm text-destructive">
            {validationError}
          </p>
        ) : null}

        {/* d. single CTA */}
        <Button onClick={handleSubmit} disabled={add.isPending || !hasSomething}>
          {add.isPending ? 'Adding…' : 'Add'}
        </Button>
      </section>

      {/* outcome summary */}
      {add.isSuccess && add.data && add.data.length > 0 ? (
        <section aria-labelledby="results-heading" className="space-y-3">
          <h2 id="results-heading" className="text-base font-semibold">
            Results
          </h2>
          <ul className="space-y-2">
            {add.data.map((outcome, index) => (
              <OutcomeRow key={index} outcome={outcome} />
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
