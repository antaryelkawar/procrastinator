/**
 * Confidence display formatter (design D8).
 *
 * - Absent/null confidence renders no indicator (never `0` or `0%`).
 * - Present confidence is rendered as a percentage (e.g., `0.82` → `82%`).
 */

/**
 * Format a confidence value for display. Returns `null` when the value is
 * absent (null or undefined) or `0`, so the caller can omit the indicator.
 * Otherwise returns a formatted percentage string.
 */
export function formatConfidence(value: number | null | undefined): string | null {
  if (value === null || value === undefined) {
    return null;
  }
  // Clamp to [0, 1] and format as percentage.
  const clamped = Math.max(0, Math.min(1, value));
  if (clamped === 0) {
    // Treat 0 as absent (never render `0%`).
    return null;
  }
  const percent = Math.round(clamped * 100);
  return `${percent}%`;
}
