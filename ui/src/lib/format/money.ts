/**
 * Money formatting — string-only, never parsed to a binary float (design D2).
 *
 * The backend transmits money as exact-decimal strings
 * (`"39999.99"`, pattern `^[0-9]+(\.[0-9]+)?$`, unsigned). Display formatting
 * operates on that string: the integer part is grouped with a regex, the
 * fraction is kept verbatim, and the currency resolves to a symbol from a
 * static map (`INR ₹`, `USD $`, `EUR €`, `GBP £`) with fallback to the ISO
 * code. `Intl.NumberFormat` is deliberately not used — it requires a float,
 * and a float can never hold an exact decimal, so `39999.99` could drift to
 * `39999.989999…`.
 *
 * Sign is derived, never stored: `expense` / import `out` → `−` (U+2212),
 * `income` / import `in` → `+`, `transfer` → no sign (accounts shown instead).
 */
import type { ImportDirection, MovementKind } from '../api/types';

/** Static currency symbol map (design D2); unlisted ISO codes fall back to the code itself. */
const CURRENCY_SYMBOLS = new Map<string, string>([
  ['INR', '₹'],
  ['USD', '$'],
  ['EUR', '€'],
  ['GBP', '£'],
]);

/** True minus sign (U+2212 MINUS SIGN) — not a hyphen-minus (design D2). */
export const MINUS_SIGN = '\u2212';

/** Display sign derived from data, never stored (design D2). */
export type MoneySign = 'negative' | 'positive' | 'none';

/**
 * Group the integer part of an exact-decimal string in thousands.
 * Pure string surgery: `39999` → `39,999`. The fraction is not touched here;
 * the caller keeps it verbatim.
 */
function groupIntegerPart(integerPart: string): string {
  return integerPart.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

/**
 * Format an exact-decimal amount string with its currency (design D2).
 *
 * `formatMoney('39999.99', 'INR')` → `'₹39,999.99'`. Known currencies use the
 * mapped symbol as a prefix; unknown ISO codes are appended after the amount
 * (ISO 4217 placement, e.g. `'39,999.99 JPY'`); an empty currency renders the
 * bare grouped amount.
 */
export function formatMoney(amount: string, currency: string): string {
  const dot = amount.indexOf('.');
  const integerPart = dot === -1 ? amount : amount.slice(0, dot);
  const fraction = dot === -1 ? '' : amount.slice(dot); // includes the dot
  const grouped = groupIntegerPart(integerPart) + fraction;

  if (currency === '') {
    return grouped;
  }
  const symbol = CURRENCY_SYMBOLS.get(currency);
  return symbol === undefined ? `${grouped} ${currency}` : symbol + grouped;
}

/**
 * Format an amount with its derived display sign (design D2):
 * `formatSignedMoney('100', 'INR', 'negative')` → `'−₹100'`,
 * `formatSignedMoney('100', 'INR', 'positive')` → `'+₹100'`,
 * `formatSignedMoney('100', 'INR', 'none')` → `'₹100'`.
 */
export function formatSignedMoney(amount: string, currency: string, sign: MoneySign): string {
  const money = formatMoney(amount, currency);
  if (sign === 'negative') {
    return MINUS_SIGN + money;
  }
  if (sign === 'positive') {
    return `+${money}`;
  }
  return money;
}

/** Derive the display sign for a movement from its kind (design D2). */
export function movementDisplaySign(kind: MovementKind): MoneySign {
  switch (kind) {
    case 'expense':
      return 'negative';
    case 'income':
      return 'positive';
    case 'transfer':
      return 'none';
  }
}

/** Derive the display sign for an import line from its direction (design D2). */
export function importLineDisplaySign(direction: ImportDirection): MoneySign {
  switch (direction) {
    case 'out':
      return 'negative';
    case 'in':
      return 'positive';
  }
}

/** The backend wire pattern for an exact-decimal amount (see `types.ts`). */
const AMOUNT_PATTERN = /^[0-9]+(\.[0-9]+)?$/;

/**
 * Validate a user-entered amount string exactly the way the backend will
 * (design D2): the exact-decimal pattern plus a string-level non-zero check.
 * Never parses to a number, so there is no float interpretation to drift.
 *
 * `isValidAmount('1250.50')` → `true`; `isValidAmount('0.00')` → `false`.
 */
export function isValidAmount(amount: string): boolean {
  if (!AMOUNT_PATTERN.test(amount)) {
    return false;
  }
  // The pattern guarantees digits and at most one dot; "non-zero" is then
  // simply "some digit is not 0".
  return /[1-9]/.test(amount);
}
