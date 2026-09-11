/**
 * Theme context (task 6.1 — dark mode).
 *
 * Follows the `active-user.tsx` house style: a plain `createContext` provider
 * plus a no-op fallback in `useTheme` so bare tests that render a
 * `useTheme` consumer without a provider do not throw.
 *
 * The initial `<html>` class is applied pre-paint by the inline script in
 * `index.html` (see `src/lib/theme-bootstrap.ts`); this provider keeps the
 * class in sync from React state and follows the OS color scheme live while
 * the theme is `"system"`.
 */
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';

export type ThemeMode = 'light' | 'dark' | 'system';
export type ResolvedTheme = 'light' | 'dark';

interface ThemeContextType {
  theme: ThemeMode;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: ThemeMode) => void;
}

const THEME_STORAGE_KEY = 'procrastinator-theme';
const DARK_MEDIA_QUERY = '(prefers-color-scheme: dark)';

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

/**
 * Returns a defensive `matchMedia` — jsdom does not implement it, so tests
 * (or other non-browser environments) get a stub that reports a light scheme
 * with no-op listeners.
 */
const safeMatchMedia = (query: string): MediaQueryList => {
  try {
    const media = window.matchMedia(query);
    if (media) {
      return media;
    }
  } catch {
    // fall through to the stub
  }
  return {
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => false,
  } as MediaQueryList;
};

const readStoredTheme = (): string | null => {
  try {
    return localStorage.getItem(THEME_STORAGE_KEY);
  } catch {
    return null;
  }
};

const toThemeMode = (stored: string | null): ThemeMode =>
  stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system';

const resolve = (theme: ThemeMode): ResolvedTheme => {
  if (theme === 'light' || theme === 'dark') {
    return theme;
  }
  return safeMatchMedia(DARK_MEDIA_QUERY).matches ? 'dark' : 'light';
};

export const ThemeProvider = ({ children }: { children: ReactNode }) => {
  const [theme, setInternalTheme] = useState<ThemeMode>(() => toThemeMode(readStoredTheme()));
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(() => resolve(theme));

  // Track the resolved theme in a ref so the media listener stays current
  // across re-renders without re-subscribing (and without firing for a theme
  // that is no longer "system").
  const resolvedRef = useRef(resolvedTheme);
  resolvedRef.current = resolvedTheme;

  // Apply the class to <html> whenever the resolved theme changes. The
  // pre-paint script has already set it for the initial paint; this keeps it
  // in sync for state changes (setTheme, OS scheme changes under "system").
  useEffect(() => {
    const root = document.documentElement;
    root.classList.remove('light');
    root.classList.remove('dark');
    root.classList.add(resolvedTheme);
  }, [resolvedTheme]);

  // Follow the OS color scheme live while the theme is "system".
  useEffect(() => {
    if (theme !== 'system') {
      return;
    }
    const media = safeMatchMedia(DARK_MEDIA_QUERY);
    const handleChange = (): void => {
      setResolvedTheme(media.matches ? 'dark' : 'light');
    };
    media.addEventListener('change', handleChange);
    return () => {
      media.removeEventListener('change', handleChange);
    };
  }, [theme]);

  const setTheme = useCallback((next: ThemeMode): void => {
    try {
      localStorage.setItem(THEME_STORAGE_KEY, next);
    } catch (error) {
      console.error('Error setting theme in localStorage:', error);
    }
    const nextResolved = resolve(next);
    setInternalTheme(next);
    setResolvedTheme(nextResolved);
    resolvedRef.current = nextResolved;
  }, []);

  return (
    <ThemeContext.Provider value={{ theme, resolvedTheme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
};

/**
 * Returns the theme context. While no `ThemeProvider` is mounted (e.g. in bare
 * component tests) the hook degrades to a no-op instead of throwing: the theme
 * stays `"system"` resolving to `"light"` and `setTheme` does nothing. The app
 * root always provides the real context (see `main.tsx`).
 */
export const useTheme = (): ThemeContextType => {
  const context = useContext(ThemeContext);
  if (context !== undefined) {
    return context;
  }
  return {
    theme: 'system',
    resolvedTheme: 'light',
    setTheme: () => undefined,
  };
};
