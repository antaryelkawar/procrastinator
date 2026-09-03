import { afterEach, describe, expect, it, vi } from 'vitest';
import { formatDate, formatTimestamp, isWarrantyExpired, todayLocalISO } from './date';

const ORIGINAL_TZ: string | undefined = process.env.TZ;

function withTz(tz: string): void {
  process.env.TZ = tz;
}

afterEach(() => {
  if (ORIGINAL_TZ === undefined) {
    delete process.env.TZ;
  } else {
    process.env.TZ = ORIGINAL_TZ;
  }
  vi.useRealTimers();
});

describe('formatDate (design D3 — string surgery, no Date on API values)', () => {
  it('should render a plain ISO occurred_on date verbatim', () => {
    expect(formatDate('2026-08-20')).toBe('2026-08-20');
  });

  it('should render an RFC3339 date-only field as its YYYY-MM-DD prefix', () => {
    expect(formatDate('2026-08-20T00:00:00Z')).toBe('2026-08-20');
    expect(formatDate('2025-01-05T00:00:00Z')).toBe('2025-01-05');
  });

  it.each(['America/New_York', 'America/Santiago', 'Pacific/Marquesas'])(
    'should keep an occurred_on date on its own calendar day when the viewer is in %s',
    (tz) => {
      // spec: "Occurred date is time-zone stable"
      withTz(tz);

      // Sanity: the shift is in effect — a naive Date WOULD show the adjacent day.
      expect(new Date('2026-08-20T00:00:00Z').toLocaleDateString('en-CA')).toBe('2026-08-19');

      expect(formatDate('2026-08-20')).toBe('2026-08-20');
      expect(formatDate('2026-08-20T00:00:00Z')).toBe('2026-08-20');
    },
  );
});

describe('formatTimestamp (design D3 — YYYY-MM-DD HH:mm UTC)', () => {
  it('should render an RFC3339 timestamp as YYYY-MM-DD HH:mm UTC', () => {
    expect(formatTimestamp('2026-08-20T14:30:05Z')).toBe('2026-08-20 14:30 UTC');
  });

  it('should strip fractional seconds when present (milliseconds or nanoseconds)', () => {
    expect(formatTimestamp('2026-08-20T14:30:05.123Z')).toBe('2026-08-20 14:30 UTC');
    expect(formatTimestamp('2026-08-20T14:30:05.123456789Z')).toBe('2026-08-20 14:30 UTC');
  });

  it('should render midnight and year-end timestamps without shifting the day', () => {
    expect(formatTimestamp('2026-01-01T00:00:00Z')).toBe('2026-01-01 00:00 UTC');
    expect(formatTimestamp('2026-12-31T23:59:59Z')).toBe('2026-12-31 23:59 UTC');
  });

  it('should keep a UTC-midnight timestamp on its own day when the viewer is in a shifted time zone', () => {
    withTz('America/New_York');

    // Sanity: a naive Date would render the previous day (20:00 local).
    expect(new Date('2026-08-20T00:00:00Z').toLocaleDateString('en-CA')).toBe('2026-08-19');

    expect(formatTimestamp('2026-08-20T00:00:00Z')).toBe('2026-08-20 00:00 UTC');
  });
});

describe('isWarrantyExpired (design D3 — lexicographic date-string comparison)', () => {
  it('should report expired when the warranty end is strictly before today', () => {
    expect(isWarrantyExpired('2026-08-20', '2026-08-21')).toBe(true);
  });

  it('should report not expired when the warranty ends today', () => {
    expect(isWarrantyExpired('2026-08-20', '2026-08-20')).toBe(false);
  });

  it('should report not expired when the warranty end is after today', () => {
    expect(isWarrantyExpired('2026-08-20', '2026-08-19')).toBe(false);
  });

  it('should reduce an RFC3339 warranty_end to its date prefix before comparing', () => {
    expect(isWarrantyExpired('2026-08-20T00:00:00Z', '2026-08-21')).toBe(true);
    expect(isWarrantyExpired('2026-08-20T00:00:00Z', '2026-08-20')).toBe(false);
  });

  it('should compare in calendar order across month and year boundaries', () => {
    expect(isWarrantyExpired('2026-07-31', '2026-08-01')).toBe(true);
    expect(isWarrantyExpired('2025-12-31', '2026-01-01')).toBe(true);
    // Single-digit vs double-digit month: lexicographic order equals calendar order.
    expect(isWarrantyExpired('2026-09-01', '2026-10-01')).toBe(true);
    expect(isWarrantyExpired('2026-10-01', '2026-09-01')).toBe(false);
  });
});

describe('todayLocalISO (design D3 — viewer-local, not UTC toISOString)', () => {
  it('should render today as a YYYY-MM-DD local calendar date', () => {
    expect(todayLocalISO()).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  it('should advance past UTC midnight when the viewer is ahead of UTC', () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.UTC(2026, 7, 20, 22, 0, 0)); // 2026-08-20T22:00:00Z
    withTz('Pacific/Kiritimati'); // UTC+14 → local 2026-08-21T12:00

    expect(todayLocalISO()).toBe('2026-08-21');
  });

  it('should stay on the UTC calendar day when the viewer is in UTC', () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.UTC(2026, 7, 20, 22, 0, 0)); // 2026-08-20T22:00:00Z
    withTz('UTC');

    expect(todayLocalISO()).toBe('2026-08-20');
  });

  it('should step back before UTC midnight when the viewer is behind UTC', () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.UTC(2026, 7, 20, 1, 0, 0)); // 2026-08-20T01:00:00Z
    withTz('America/Santiago'); // UTC-4 in August → local 2026-08-19T21:00

    expect(todayLocalISO()).toBe('2026-08-19');
  });

  it('should feed the warranty-expired default with the viewer-local today', () => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.UTC(2026, 7, 20, 22, 0, 0)); // 2026-08-20T22:00:00Z
    withTz('Pacific/Kiritimati'); // viewer-local today = 2026-08-21

    expect(isWarrantyExpired('2026-08-20')).toBe(true);
    expect(isWarrantyExpired('2026-08-21')).toBe(false);
  });
});
