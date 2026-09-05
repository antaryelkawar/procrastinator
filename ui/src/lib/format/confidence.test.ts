import { describe, expect, it } from 'vitest';
import { formatConfidence } from './confidence';

describe('formatConfidence', () => {
  it('formats 0.82 as "82%"', () => {
    expect(formatConfidence(0.82)).toBe('82%');
  });

  it('formats 1.0 as "100%"', () => {
    expect(formatConfidence(1.0)).toBe('100%');
  });

  it('formats 0.5 as "50%"', () => {
    expect(formatConfidence(0.5)).toBe('50%');
  });

  it('formats 0.01 as "1%"', () => {
    expect(formatConfidence(0.01)).toBe('1%');
  });

  it('returns null for absent (undefined)', () => {
    expect(formatConfidence(undefined)).toBeNull();
  });

  it('returns null for null', () => {
    expect(formatConfidence(null)).toBeNull();
  });

  it('returns null for 0 (treat zero as absent, never render "0%")', () => {
    expect(formatConfidence(0)).toBeNull();
  });

  it('clamps values above 1 to 100%', () => {
    expect(formatConfidence(1.5)).toBe('100%');
  });

  it('clamps values below 0 to 0 (then null since it is 0)', () => {
    // -0.1 clamps to 0, which is treated as absent → null
    expect(formatConfidence(-0.1)).toBeNull();
  });
});
