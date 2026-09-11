import { useMemo, useState } from 'react';
import { Check, ChevronDown, Pencil } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { formatConfidence } from '@/lib/format/confidence';
import { cn } from '@/lib/utils';

export interface ReviewChipField {
  /** Stable id, e.g. 'brand' | 'model' | 'serial_number' | 'price'. */
  readonly key: string;
  /** Display label, e.g. 'Brand'. */
  readonly label: string;
  /** The extracted value. */
  readonly value: string;
  /** 0..1, optional; absent/null → no indicator. */
  readonly confidence?: number | null;
}

export interface ReviewChipsProps {
  /** Extracted (found) fields — rendered as chips. */
  readonly fields: readonly ReviewChipField[];
  /** Optional fields the extractor did NOT find — collapsed behind "Add details". */
  readonly optionalFields?: readonly ReviewChipField[];
  /** Fired when the user saves (edited OR unedited) with the final values keyed by field.key. */
  readonly onConfirm: (values: Readonly<Record<string, string>>) => void;
  /** Fired when the user cancels without saving (render Cancel only when provided). */
  readonly onCancel?: () => void;
  /** Disable Save while the create mutation is in flight. */
  readonly busy?: boolean;
  /** Heading text. Defaults to 'Review before you save'. */
  readonly title?: string;
}

const CHIP = 'rounded-full border border-input bg-muted/50 px-3 py-1';

/**
 * Normalize a raw backend `candidate_fields` map into `ReviewChipField[]`.
 * Stringifies scalar values, skips null/undefined/empty, and prettifies keys
 * (snake_case → Title Case label). Kept simple and strongly typed.
 */
export function toReviewChips(
  raw: Record<string, unknown>,
  confidence?: number | null,
): ReviewChipField[] {
  return Object.entries(raw)
    .map(([key, value]) => {
      const scalar = value as string | number | boolean | null | undefined;
      if (scalar === null || scalar === undefined) return null;
      if (typeof scalar === 'string' && scalar.trim() === '') return null;
      const trimmed = String(scalar).trim();
      if (trimmed === '') return null;
      const field: ReviewChipField = {
        key,
        label: prettyKey(key),
        value: trimmed,
        ...(confidence !== undefined && confidence !== null
          ? { confidence }
          : {}),
      };
      return field;
    })
    .filter((f): f is ReviewChipField => f !== null);
}

/** Prettify a snake_case / kebab-case key into Title Case. */
function prettyKey(key: string): string {
  return key
    .replace(/[-_]+/g, ' ')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim();
}

/**
 * ReviewChips (task 14.3 / design D11): the progressive-disclosure review
 * surface shown after extraction. Found fields render as confirmable chips
 * (tap to reveal a corrected value); unextracted optional fields stay
 * collapsed behind a single "Add details" affordance; Save commits the
 * final values (edited or unedited). Presentational and provider-free — the
 * parent flow owns the container.
 */
export function ReviewChips({
  fields,
  optionalFields = [],
  onConfirm,
  onCancel,
  busy = false,
  title = 'Review before you save',
}: ReviewChipsProps): React.JSX.Element {
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(fields.map((f) => [f.key, f.value])),
  );
  const [optionalValues, setOptionalValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(optionalFields.map((f) => [f.key, f.value ?? ''])),
  );
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const [showOptional, setShowOptional] = useState(false);

  const optionalEditable = useMemo(
    () => optionalFields.map((f) => ({ ...f, value: optionalValues[f.key] ?? '' })),
    [optionalFields, optionalValues],
  );

  const handleSave = (): void => {
    const out: Record<string, string> = { ...values };
    for (const f of optionalFields) {
      const v = optionalEditable.find((o) => o.key === f.key)?.value ?? '';
      if (v !== '') out[f.key] = v;
    }
    onConfirm(out);
  };

  return (
    <div className="w-full space-y-4">
      <h3 className="text-lg font-semibold">{title}</h3>

      {/* Found-field chips (progressive disclosure per chip). */}
      <div className="flex flex-wrap gap-2">
        {fields.map((f) => {
          const isActive = expandedKey === f.key;
          const current = values[f.key] ?? f.value;
          const indicator = f.confidence !== undefined && f.confidence !== null
            ? formatConfidence(f.confidence)
            : null;
          return (
            <div key={f.key} className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="default"
                aria-expanded={isActive}
                aria-controls={`review-editor-${f.key}`}
                onClick={() => setExpandedKey(isActive ? null : f.key)}
                className={cn(
                  CHIP,
                  // min-h-11 → ≥44px tap target for the primary tap-to-reveal control
                  'min-h-11 flex items-center gap-2 px-3 py-1 text-sm font-normal',
                )}
              >
                {/* sr-only action word: keeps the accessible name ("Edit <Label>: <value>")
                    consistent with the visible content (axe 4.12 label-content-name-mismatch). */}
                <span className="sr-only">Edit </span>
                <span className="text-muted-foreground">{f.label}:</span>
                <span className="max-w-[12rem] truncate" title={current}>
                  {current}
                </span>
                {indicator ? <span className="text-xs text-muted-foreground">{indicator}</span> : null}
                <Pencil className="size-3.5 text-muted-foreground" aria-hidden="true" />
              </Button>

              {isActive ? (
                <div
                  id={`review-editor-${f.key}`}
                  role="group"
                  aria-label={`${f.label} editor`}
                  className="flex items-center gap-2"
                >
                  <Input
                    id={`review-input-${f.key}`}
                    aria-label={f.label}
                    value={current}
                    onChange={(e) =>
                      setValues((prev) => ({ ...prev, [f.key]: e.target.value }))
                    }
                    className="min-w-[8rem]"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="icon-sm"
                    aria-label="Done"
                    onClick={() => setExpandedKey(null)}
                    className={cn(CHIP, 'px-2 py-1')}
                  >
                    <Check className="size-4" aria-hidden="true" />
                  </Button>
                </div>
              ) : null}
            </div>
          );
        })}
      </div>

      {/* "Add details" disclosure for optional fields. Collapsed by default:
          NO optional-field inputs are rendered until expanded. */}
      {optionalFields.length > 0 ? (
        <div className="space-y-2">
          <Button
            type="button"
            variant="ghost"
            size="default"
            aria-expanded={showOptional}
            aria-controls="review-optional-region"
            onClick={() => setShowOptional((s) => !s)}
            className="flex items-center gap-1 text-sm font-normal"
          >
            Add details
            <ChevronDown
              className={cn('size-4 transition-transform', showOptional && 'rotate-180')}
              aria-hidden="true"
            />
          </Button>

          {showOptional ? (
            <div id="review-optional-region" className="space-y-2">
              {optionalEditable.map((f) => (
                <div key={f.key} className="flex flex-col gap-1">
                  <label htmlFor={`optional-input-${f.key}`} className="text-sm font-medium">
                    {f.label}
                  </label>
                  <Input
                    id={`optional-input-${f.key}`}
                    value={f.value}
                    placeholder={`Add ${f.label.toLowerCase()}`}
                    onChange={(e) =>
                      setOptionalValues((prev) => ({ ...prev, [f.key]: e.target.value }))
                    }
                  />
                </div>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      {/* Footer: Cancel (when provided) + Save. */}
      <div className="flex items-center justify-end gap-2">
        {onCancel ? (
          <Button type="button" variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
        ) : null}
        <Button
          type="button"
          variant="default"
          onClick={handleSave}
          disabled={busy}
          aria-busy={busy}
          className="min-h-11"
        >
          {busy ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  );
}
