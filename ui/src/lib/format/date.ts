/**
 * Date rendering — string surgery only, never `new Date()` on an API value
 * (design D3).
 *
 * Wire contract: `occurred_on` is a plain ISO date (`"2026-08-20"`); date-only
 * fields (`purchase_date`, `warranty_end`) arrive as RFC3339 UTC midnight
 * (`"2026-08-20T00:00:00Z"`); all other timestamps are RFC3339Nano UTC.
 * Rendering any of these through `Date` would apply the viewer's time zone and
 * could shift a calendar day (UTC midnight is 8 pm the previous day at
 * UTC−4). Every function here operates on the string, so the output is
 * time-zone-stable by construction. The single exception is
 * {@link todayLocalISO}, which must be viewer-local by definition.
 */

/**
 * Render a calendar-date value as `YYYY-MM-DD` by taking its date prefix.
 *
 * `formatDate('2026-08-20')` → `'2026-08-20'` (occurred_on, verbatim);
 * `formatDate('2026-08-20T00:00:00Z')` → `'2026-08-20'` (date-only RFC3339).
 */
export function formatDate(value: string): string {
  return value.slice(0, 10);
}

/**
 * Render an RFC3339Nano UTC timestamp as `YYYY-MM-DD HH:mm UTC` by string
 * surgery: drop the fractional seconds and `Z`, replace `T` with a space.
 *
 * `formatTimestamp('2026-08-20T14:30:05.123Z')` → `'2026-08-20 14:30 UTC'`.
 * The `UTC` suffix is part of the format: the value is UTC by contract, and
 * presenting it as local time would misstate it.
 */
export function formatTimestamp(value: string): string {
  const secondsOnly = value.split('.')[0]; // drop fractional seconds, if any
  return `${secondsOnly.slice(0, 10)} ${secondsOnly.slice(11, 16)} UTC`;
}

/**
 * Whether a warranty has expired, compared as date strings (design D3):
 * expired exactly when the warranty-end date is strictly before the viewer's
 * local today. Equal-length ISO dates compare lexicographically in calendar
 * order, and `warrantyEnd` may be a plain ISO date or an RFC3339 value (both
 * reduce to their `YYYY-MM-DD` prefix) — no `Date`, no TZ drift.
 *
 * `today` is injectable for tests; it defaults to {@link todayLocalISO}.
 * A warranty that ends today is still in force (not expired).
 */
export function isWarrantyExpired(warrantyEnd: string, today: string = todayLocalISO()): boolean {
  return warrantyEnd.slice(0, 10) < today;
}

/**
 * Today's calendar date in the viewer's local time zone, as `YYYY-MM-DD`,
 * built from the local `Date` parts (not `toISOString()`, which is UTC —
 * design D3). The only place this module touches `Date`, and never on an API
 * value.
 */
export function todayLocalISO(): string {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
}
