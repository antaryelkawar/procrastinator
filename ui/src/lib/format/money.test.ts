import { describe, it, expect } from 'vitest';
import {
  MINUS_SIGN,
  formatMoney,
  formatSignedMoney,
  importLineDisplaySign,
  isValidAmount,
  movementDisplaySign,
} from './money';

describe('formatMoney (design D2)', () => {
  it('should render 39999.99 INR exactly as ₹39,999.99 when the amount has a fraction', () => {
    // spec: "Price displays exactly"
    expect(formatMoney('39999.99', 'INR')).toBe('₹39,999.99');
  });

  it('should render 19999.99 INR exactly as ₹19,999.99 without rounding or binary artifacts', () => {
    // spec: "Amount renders without drift"
    expect(formatMoney('19999.99', 'INR')).toBe('₹19,999.99');
    expect(formatMoney('39999.99', 'INR')).not.toContain('989999');
  });

  it('should keep the fraction verbatim when it is not exactly two digits', () => {
    expect(formatMoney('42.5', 'GBP')).toBe('£42.5');
    expect(formatMoney('123.456', 'INR')).toBe('₹123.456');
  });

  it('should render whole amounts without padding a fraction', () => {
    expect(formatMoney('250', 'USD')).toBe('$250');
    expect(formatMoney('150', 'EUR')).toBe('€150');
    expect(formatMoney('0', 'USD')).toBe('$0');
  });

  it('should group the integer part in thousands when the amount is large', () => {
    expect(formatMoney('1000', 'USD')).toBe('$1,000');
    expect(formatMoney('1234567', 'INR')).toBe('₹1,234,567');
    expect(formatMoney('1000000.005', 'USD')).toBe('$1,000,000.005');
  });

  it('should render zero-with-fraction exactly when the integer part is zero', () => {
    expect(formatMoney('0.5', 'USD')).toBe('$0.5');
  });

  it('should fall back to the ISO code as a suffix when the currency is not in the symbol map', () => {
    expect(formatMoney('1000', 'JPY')).toBe('1,000 JPY');
    expect(formatMoney('39999.99', 'JPY')).toBe('39,999.99 JPY');
  });

  it('should render the bare grouped amount without trailing whitespace when the currency is empty', () => {
    expect(formatMoney('100', '')).toBe('100');
    expect(formatMoney('100', '')).not.toMatch(/\s$/);
  });
});

describe('formatSignedMoney and derived signs (design D2)', () => {
  it('should prefix a true minus sign (U+2212, not a hyphen) when the sign is negative', () => {
    const rendered = formatSignedMoney('100', 'INR', 'negative');

    expect(rendered).toBe('\u2212₹100');
    expect(rendered.charCodeAt(0)).toBe(0x2212);
    expect(rendered).not.toContain('-');
  });

  it('should prefix a plus sign when the sign is positive', () => {
    expect(formatSignedMoney('1250.50', 'INR', 'positive')).toBe('+₹1,250.50');
  });

  it('should render no sign when the sign is none (transfer: accounts shown instead)', () => {
    expect(formatSignedMoney('500', 'USD', 'none')).toBe('$500');
  });

  it('should keep the grouping under the sign when the amount is large', () => {
    expect(formatSignedMoney('39999.99', 'INR', 'negative')).toBe('−₹39,999.99');
  });

  it('should derive negative for expense, positive for income, none for transfer from the movement kind', () => {
    expect(movementDisplaySign('expense')).toBe('negative');
    expect(movementDisplaySign('income')).toBe('positive');
    expect(movementDisplaySign('transfer')).toBe('none');
  });

  it('should derive negative for out and positive for in from the import line direction', () => {
    expect(importLineDisplaySign('out')).toBe('negative');
    expect(importLineDisplaySign('in')).toBe('positive');
  });

  it('should render an expense movement as signed, grouped, and exact', () => {
    const rendered = formatSignedMoney('19999.99', 'INR', movementDisplaySign('expense'));

    expect(rendered).toBe(MINUS_SIGN + '₹19,999.99');
  });
});

describe('isValidAmount (design D2 — string-level, never parseFloat)', () => {
  it('should accept positive decimals matching the backend pattern', () => {
    expect(isValidAmount('1250.50')).toBe(true);
    expect(isValidAmount('250')).toBe(true);
    expect(isValidAmount('0.01')).toBe(true);
    expect(isValidAmount('1000000')).toBe(true);
  });

  it('should reject zero amounts at the string level', () => {
    expect(isValidAmount('0')).toBe(false);
    expect(isValidAmount('0.0')).toBe(false);
    expect(isValidAmount('0.000')).toBe(false);
  });

  it('should reject strings that violate the exact-decimal pattern', () => {
    expect(isValidAmount('')).toBe(false);
    expect(isValidAmount('-5')).toBe(false);
    expect(isValidAmount('+5')).toBe(false);
    expect(isValidAmount('1.2.3')).toBe(false);
    expect(isValidAmount('12.')).toBe(false);
    expect(isValidAmount('.5')).toBe(false);
    expect(isValidAmount('1e3')).toBe(false);
    expect(isValidAmount('abc')).toBe(false);
    expect(isValidAmount(' 12')).toBe(false);
  });

  it('should treat a tiny positive amount as non-zero without any float interpretation', () => {
    expect(isValidAmount('0.0000001')).toBe(true);
  });
});
