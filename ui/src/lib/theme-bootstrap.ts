/**
 * Theme bootstrap — shared logic for the pre-paint script in `index.html`.
 *
 * The inline `<script>` in `index.html` must run before the module bundle
 * (it cannot import this file), but it stays behaviorally identical to these
 * two functions, which are exercised by `theme-bootstrap.test.ts`.
 */

/** localStorage key the theme preference is persisted under. */
export const THEME_STORAGE_KEY = 'procrastinator-theme';

/**
 * Applies the theme class to `<html>` for the given stored preference.
 *
 * `"light"`/`"dark"` are applied as-is; `"system"` (or anything unrecognized,
 * including `null`) follows the OS color scheme (`mediaDark`).
 */
export const applyThemeClass = (theme: string | null, mediaDark: boolean): void => {
  const resolved = theme === 'light' || theme === 'dark' ? theme : mediaDark ? 'dark' : 'light';
  const root = document.documentElement;
  root.classList.remove('light');
  root.classList.remove('dark');
  root.classList.add(resolved);
};

/**
 * Reads the stored theme preference, returning `null` when nothing is stored
 * or localStorage is unavailable (e.g. storage disabled in private mode).
 */
export const readStoredTheme = (): string | null => {
  try {
    return localStorage.getItem(THEME_STORAGE_KEY);
  } catch {
    return null;
  }
};
