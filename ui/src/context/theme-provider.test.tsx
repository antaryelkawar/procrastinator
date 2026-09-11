/**
 * Task 6.1 — theme provider: default is system, setTheme persists across
 * "reload" (fresh provider mount), and "system" follows the OS color scheme
 * live via the matchMedia change listener.
 */
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider, useTheme } from './theme-provider';

const THEME_KEY = 'procrastinator-theme';

/** Probe: renders the hook's state and a button per action. */
function Probe() {
  const { theme, resolvedTheme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="resolved-theme">{resolvedTheme}</span>
      <button onClick={() => setTheme('dark')}>Set dark</button>
      <button onClick={() => setTheme('light')}>Set light</button>
      <button onClick={() => setTheme('system')}>Set system</button>
    </div>
  );
}

const renderProvider = () => render(<ThemeProvider><Probe /></ThemeProvider>);

/**
 * A controllable matchMedia mock: reports `matches`, captures the change
 * listeners, and lets the test flip the scheme and fire "change".
 */
function installMatchMediaMock(initialMatches: boolean) {
  const state = { matches: initialMatches };
  const listeners = new Set<() => void>();
  window.matchMedia = ((query: string) => ({
    get matches() {
      return state.matches;
    },
    media: query,
    onchange: null,
    addEventListener: (_type: string, listener: () => void) => {
      listeners.add(listener);
    },
    removeEventListener: (_type: string, listener: () => void) => {
      listeners.delete(listener);
    },
    addListener: (listener: () => void) => {
      listeners.add(listener);
    },
    removeListener: (listener: () => void) => {
      listeners.delete(listener);
    },
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
  return {
    fireChange(matches: boolean) {
      act(() => {
        state.matches = matches;
        listeners.forEach((listener) => listener());
      });
    },
  };
}

describe('ThemeProvider', () => {
  const originalMatchMedia = window.matchMedia;

  beforeEach(() => {
    localStorage.clear();
    document.documentElement.classList.remove('light');
    document.documentElement.classList.remove('dark');
  });

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    document.documentElement.classList.remove('light');
    document.documentElement.classList.remove('dark');
  });

  it('defaults to system and resolves against the OS scheme (setup polyfill reports light)', () => {
    renderProvider();
    expect(screen.getByTestId('theme')).toHaveTextContent('system');
    expect(screen.getByTestId('resolved-theme')).toHaveTextContent('light');
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('setTheme("dark") persists, switches the html class, and resolves dark', () => {
    renderProvider();
    fireEvent.click(screen.getByText('Set dark'));
    expect(localStorage.getItem(THEME_KEY)).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
    expect(screen.getByTestId('theme')).toHaveTextContent('dark');
    expect(screen.getByTestId('resolved-theme')).toHaveTextContent('dark');
  });

  it('setTheme("light") persists, switches the html class, and resolves light', () => {
    localStorage.setItem(THEME_KEY, 'dark');
    renderProvider();
    fireEvent.click(screen.getByText('Set light'));
    expect(localStorage.getItem(THEME_KEY)).toBe('light');
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    expect(screen.getByTestId('theme')).toHaveTextContent('light');
    expect(screen.getByTestId('resolved-theme')).toHaveTextContent('light');
  });

  it('picks up a persisted preference on a fresh mount (reload)', () => {
    localStorage.setItem(THEME_KEY, 'dark');
    renderProvider();
    expect(screen.getByTestId('theme')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
  });

  it('treats an unrecognized stored value as system', () => {
    localStorage.setItem(THEME_KEY, 'garbage');
    renderProvider();
    expect(screen.getByTestId('theme')).toHaveTextContent('system');
  });

  it('resolves dark on a fresh mount when the OS is dark and no preference is stored', () => {
    installMatchMediaMock(true);
    renderProvider();
    expect(screen.getByTestId('theme')).toHaveTextContent('system');
    expect(screen.getByTestId('resolved-theme')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('system theme follows an OS scheme change live without user action', () => {
    const media = installMatchMediaMock(false);
    renderProvider();
    expect(screen.getByTestId('resolved-theme')).toHaveTextContent('light');

    media.fireChange(true);

    expect(screen.getByTestId('resolved-theme')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
    // The user preference is untouched by OS changes.
    expect(localStorage.getItem(THEME_KEY)).toBeNull();
  });

  it('does not follow OS scheme changes when a fixed theme is selected', () => {
    const media = installMatchMediaMock(false);
    renderProvider();
    fireEvent.click(screen.getByText('Set light'));

    media.fireChange(true);

    expect(screen.getByTestId('resolved-theme')).toHaveTextContent('light');
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('unsubscribes the change listener on unmount', () => {
    const media = installMatchMediaMock(false);
    const { unmount } = renderProvider();
    unmount();

    // After unmount the listener set is empty, so a late OS scheme change must
    // not throw (no state update on an unmounted tree) and must not re-apply
    // the dark class.
    expect(() => media.fireChange(true)).not.toThrow();
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });
});
