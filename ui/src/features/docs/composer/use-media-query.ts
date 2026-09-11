import { useEffect, useState } from 'react';

/**
 * Subscribe to a CSS media query (SSR/jsdom-safe). Used to pick the composer's
 * responsive variant (bottom sheet at narrow, dialog >= sm). The test setup
 * stubs `window.matchMedia` to return `matches:false`, so the narrow branch is
 * the default in tests.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState<boolean>(() =>
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia(query).matches
      : false,
  );

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return;
    }
    const mql = window.matchMedia(query);
    const onChange = () => setMatches(mql.matches);
    setMatches(mql.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, [query]);

  return matches;
}
