/**
 * Movement creation form (spec "Movement creation form"; design D7).
 *
 * Kinds `expense`/`income`/`transfer` with kind-specific account selection:
 * an expense picks a source account, an income a destination account, a
 * transfer two DISTINCT accounts of the SAME currency. Every rule is
 * enforced client-side by {@link buildMovementRequest} — a pure exported
 * function, so each rule is directly unit-testable (D7) — and no request is
 * sent while any rule fails:
 *   - non-positive / malformed amount (string validation via `isValidAmount`,
 *     design D2 — never `parseFloat`),
 *   - missing occurred date,
 *   - blank description,
 *   - missing account for the selected kind,
 *   - identical transfer source/destination,
 *   - cross-currency transfer.
 *
 * On success the form resets to its initial values; the `useCreateMovement`
 * hook (D5) invalidates `["movements", uid]` + `["accounts", uid]` so the
 * movements list and account balances refresh. Server rejections surface as
 * form-level error text WITHOUT clearing the values the user entered (D7).
 *
 * Native `<select>` elements are used deliberately: the Radix Select portal
 * does not position under jsdom (lesson from task 4.5).
 */
import { useState } from 'react';
import type { FormEvent } from 'react';
import { useAccounts, useCreateMovement } from '../../lib/api/hooks';
import type { Account, CreateMovementRequest, MovementKind } from '../../lib/api/types';
import { isValidAmount } from '../../lib/format/money';
import { todayLocalISO } from '../../lib/format/date';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loading } from '@/components/feedback/loading';

/** The form's own field values (before kind-specific wiring into a request). */
export interface MovementFormValues {
  readonly kind: MovementKind;
  /** User-entered amount string; validated string-only (D2). */
  readonly amount: string;
  /** `YYYY-MM-DD` from the native date input. */
  readonly occurredOn: string;
  readonly description: string;
  readonly sourceAccountId: string;
  readonly destinationAccountId: string;
}

/** Either a ready-to-send request (all client rules passed) or the blocking reason. */
export type MovementFormResult =
  | { readonly ok: true; readonly request: CreateMovementRequest }
  | { readonly ok: false; readonly error: string };

/**
 * Initial form state. The date defaults to the viewer's local today (the one
 * sanctioned use of `Date`, design D3); everything else starts blank.
 */
export function createInitialMovementFormValues(): MovementFormValues {
  return {
    kind: 'expense',
    amount: '',
    occurredOn: todayLocalISO(),
    description: '',
    sourceAccountId: '',
    destinationAccountId: '',
  };
}

/** The native date input emits `YYYY-MM-DD`; anything else is a missing date. */
const OCCURRED_DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

function accountById(accounts: readonly Account[], id: string): Account | undefined {
  return accounts.find((account) => account.id === id);
}

/**
 * Validate the form values and, when every rule passes, build the exact
 * `POST /api/finance/movements` body. Pure and exported so each validation
 * rule is unit-testable without rendering (design D7, incl. the
 * cross-currency transfer rejection).
 *
 * The amount check is string-only: `isValidAmount` enforces the backend wire
 * pattern `^[0-9]+(\.[0-9]+)?$` plus a string-level non-zero check — no float
 * interpretation, so there is nothing to drift (design D2).
 */
export function buildMovementRequest(
  values: MovementFormValues,
  accounts: readonly Account[],
): MovementFormResult {
  if (!isValidAmount(values.amount)) {
    return { ok: false, error: 'Amount must be a positive number.' };
  }
  if (!OCCURRED_DATE_PATTERN.test(values.occurredOn)) {
    return { ok: false, error: 'Pick the date the movement occurred.' };
  }
  if (values.description.trim() === '') {
    return { ok: false, error: 'Description cannot be empty.' };
  }

  if (values.kind === 'expense') {
    const source = accountById(accounts, values.sourceAccountId);
    if (source === undefined) {
      return { ok: false, error: 'Pick the account the expense leaves.' };
    }
    return {
      ok: true,
      request: {
        kind: 'expense',
        amount: values.amount,
        currency: source.currency,
        occurred_on: values.occurredOn,
        description: values.description.trim(),
        source_account_id: source.id,
      },
    };
  }

  if (values.kind === 'income') {
    const destination = accountById(accounts, values.destinationAccountId);
    if (destination === undefined) {
      return { ok: false, error: 'Pick the account the income lands in.' };
    }
    return {
      ok: true,
      request: {
        kind: 'income',
        amount: values.amount,
        currency: destination.currency,
        occurred_on: values.occurredOn,
        description: values.description.trim(),
        destination_account_id: destination.id,
      },
    };
  }

  // transfer: two distinct accounts of the same currency
  const source = accountById(accounts, values.sourceAccountId);
  if (source === undefined) {
    return { ok: false, error: 'Pick the account the transfer leaves.' };
  }
  const destination = accountById(accounts, values.destinationAccountId);
  if (destination === undefined) {
    return { ok: false, error: 'Pick the account the transfer lands in.' };
  }
  if (source.id === destination.id) {
    return { ok: false, error: 'A transfer needs two different accounts.' };
  }
  if (source.currency !== destination.currency) {
    return { ok: false, error: 'A transfer needs both accounts in the same currency.' };
  }
  return {
    ok: true,
    request: {
      kind: 'transfer',
      amount: values.amount,
      currency: source.currency,
      occurred_on: values.occurredOn,
      description: values.description.trim(),
      source_account_id: source.id,
      destination_account_id: destination.id,
    },
  };
}

/** Native-select styling matching the movements page's filter selects. */
const NATIVE_SELECT_CLASS =
  'h-9 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm shadow-xs outline-none ' +
  'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30';

/**
 * The movement creation form. Renders the four-state loading contract via
 * `<Loading />` while the account list is in flight, then the controlled form.
 */
export function MovementCreateForm() {
  const { data: accounts, isLoading: accountsLoading } = useAccounts();
  const createMovement = useCreateMovement();
  const [values, setValues] = useState<MovementFormValues>(createInitialMovementFormValues);
  const [error, setError] = useState<string | null>(null);

  function setValue<K extends keyof MovementFormValues>(key: K, value: MovementFormValues[K]): void {
    setValues((previous) => ({ ...previous, [key]: value }));
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    const result = buildMovementRequest(values, accounts ?? []);
    if (!result.ok) {
      setError(result.error);
      return;
    }
    setError(null);
    createMovement.mutate(result.request, {
      onSuccess: () => {
        setValues(createInitialMovementFormValues());
        setError(null);
      },
      // Server rejection: show the mapped reason, keep the entered values (D7).
      onError: (failure) => {
        setError(failure.message);
      },
    });
  }

  if (accountsLoading) {
    return <Loading label="Loading accounts" />;
  }

  const accountOptions = accounts ?? [];
  const showsSource = values.kind === 'expense' || values.kind === 'transfer';
  const showsDestination = values.kind === 'income' || values.kind === 'transfer';

  return (
    <form
      aria-label="Create movement"
      data-testid="movement-create-form"
      onSubmit={handleSubmit}
      className="space-y-4"
    >
      {error !== null && (
        <p data-testid="movement-form-error" role="alert" className="text-sm font-medium text-destructive">
          {error}
        </p>
      )}

      <div className="space-y-1.5">
        <Label htmlFor="movement-kind">Kind</Label>
        <select
          id="movement-kind"
          name="kind"
          value={values.kind}
          onChange={(event) => setValue('kind', event.target.value as MovementKind)}
          className={NATIVE_SELECT_CLASS}
        >
          <option value="expense">Expense</option>
          <option value="income">Income</option>
          <option value="transfer">Transfer</option>
        </select>
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="movement-amount">Amount</Label>
        <Input
          id="movement-amount"
          name="amount"
          inputMode="decimal"
          placeholder="0.00"
          value={values.amount}
          onChange={(event) => setValue('amount', event.target.value)}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="movement-occurred-on">Occurred on</Label>
        <Input
          id="movement-occurred-on"
          name="occurred_on"
          type="date"
          value={values.occurredOn}
          onChange={(event) => setValue('occurredOn', event.target.value)}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="movement-description">Description</Label>
        <Input
          id="movement-description"
          name="description"
          value={values.description}
          onChange={(event) => setValue('description', event.target.value)}
        />
      </div>

      {showsSource && (
        <div className="space-y-1.5">
          <Label htmlFor="movement-source-account">Source account</Label>
          <select
            id="movement-source-account"
            name="source_account_id"
            value={values.sourceAccountId}
            onChange={(event) => setValue('sourceAccountId', event.target.value)}
            className={NATIVE_SELECT_CLASS}
          >
            <option value="">Select an account</option>
            {accountOptions.map((account) => (
              <option key={account.id} value={account.id}>
                {account.name} ({account.currency})
              </option>
            ))}
          </select>
        </div>
      )}

      {showsDestination && (
        <div className="space-y-1.5">
          <Label htmlFor="movement-destination-account">Destination account</Label>
          <select
            id="movement-destination-account"
            name="destination_account_id"
            value={values.destinationAccountId}
            onChange={(event) => setValue('destinationAccountId', event.target.value)}
            className={NATIVE_SELECT_CLASS}
          >
            <option value="">Select an account</option>
            {accountOptions.map((account) => (
              <option key={account.id} value={account.id}>
                {account.name} ({account.currency})
              </option>
            ))}
          </select>
        </div>
      )}

      <Button type="submit" disabled={createMovement.isPending}>
        {createMovement.isPending ? 'Creating…' : 'Create movement'}
      </Button>
    </form>
  );
}
