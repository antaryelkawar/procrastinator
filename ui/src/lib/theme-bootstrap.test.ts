import { describe, expect, it, vi, beforeEach } from 'vitest';
import { applyThemeClass, readStoredTheme, THEME_STORAGE_KEY } from './theme-bootstrap';

describe('applyThemeClass', () => {
  beforeEach(() => {
    document.documentElement.classList.remove('light');
    document.documentElement.classList.remove('dark');
  });

  it('applies dark when the stored theme is dark', () => {
    applyThemeClass('dark', false);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
  });

  it('applies light when the stored theme is light, even if the OS is dark', () => {
    applyThemeClass('light', true);
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('follows a dark media query for the system theme', () => {
    applyThemeClass('system', true);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
  });

  it('follows a light media query for the system theme', () => {
    applyThemeClass('system', false);
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('treats a missing preference as system (follows the media query)', () => {
    applyThemeClass(null, true);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
  });

  it('treats an unrecognized value as system (follows the media query)', () => {
    applyThemeClass('garbage', false);
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('replaces the previously applied class', () => {
    applyThemeClass('dark', false);
    applyThemeClass('light', false);
    expect(document.documentElement.classList.contains('light')).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });
});

describe('readStoredTheme', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('returns the stored theme preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    expect(readStoredTheme()).toBe('dark');
  });

  it('returns null when nothing is stored', () => {
    expect(readStoredTheme()).toBeNull();
  });

  it('returns null when localStorage throws (storage disabled)', () => {
    const original = window.localStorage;
    const throwing: Storage = {
      getItem: () => {
        throw new Error('access denied');
      },
      setItem: () => {
        throw new Error('access denied');
      },
      removeItem: () => {
        throw new Error('access denied');
      },
      clear: () => {
        throw new Error('access denied');
      },
      key: () => null,
      length: 0,
    };
    try {
      Object.defineProperty(window, 'localStorage', { value: throwing, configurable: true });
      expect(readStoredTheme()).toBeNull();
    } finally {
      Object.defineProperty(window, 'localStorage', { value: original, configurable: true });
    }
  });

  it('returns null when getItem throws', () => {
    const spy = vi
      .spyOn(window.localStorage, 'getItem')
      .mockImplementation(() => {
        throw new Error('security error');
      });
    try {
      expect(readStoredTheme()).toBeNull();
    } finally {
      spy.mockRestore();
    }
  });
});
