import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { useQuickSearch } from '@/lib/api/hooks';
import { formatConfidence } from '@/lib/format/confidence';
import type { SearchHit } from '@/lib/api/schema';

const DEBOUNCE_MS = 250;

/**
 * Map a search hit to its destination route (design D8 navigation mapping).
 * - `asset` → `/assets/{id}`
 * - `import_batch` → `/finance/import/{id}`
 * - `account` → `/finance/accounts` (no detail page)
 * - `movement` → `/finance/movements` (no detail page)
 * - `document` → `/assets` (document's owning asset is not addressable)
 */
export function hitRoute(hit: SearchHit): string {
  switch (hit.type) {
    case 'asset':
      return `/assets/${hit.id}`;
    case 'import_batch':
      return `/finance/import/${hit.id}`;
    case 'account':
      return '/finance/accounts';
    case 'movement':
      return '/finance/movements';
    case 'document':
      return '/assets';
    default:
      return '/assets';
  }
}

export function QuickSearch() {
  const [rawValue, setRawValue] = useState('');
  const [debouncedValue, setDebouncedValue] = useState('');
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);
  const listboxId = 'quick-search-listbox';

  const { data } = useQuickSearch(debouncedValue);
  const hits = data?.results ?? [];

  // 250ms debounce
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedValue(rawValue);
    }, DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [rawValue]);

  // Reset active index when hits change
  useEffect(() => {
    setActiveIndex(-1);
  }, [hits]);

  // Blank query → close + no hits
  useEffect(() => {
    if (debouncedValue.trim() === '') {
      setIsOpen(false);
    } else if (hits.length > 0) {
      setIsOpen(true);
    } else {
      // No hits: keep the dropdown closed (no results state).
      setIsOpen(false);
    }
  }, [debouncedValue, hits.length]);

  const select = (hit: SearchHit) => {
    navigate(hitRoute(hit));
    setRawValue('');
    setDebouncedValue('');
    setIsOpen(false);
    setActiveIndex(-1);
    inputRef.current?.focus();
  };

  const dismiss = () => {
    setIsOpen(false);
    setActiveIndex(-1);
    inputRef.current?.focus();
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      dismiss();
      return;
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (!isOpen) return;
      setActiveIndex((i) => Math.min(i + 1, hits.length - 1));
      return;
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      if (!isOpen) return;
      setActiveIndex((i) => Math.max(i - 1, 0));
      return;
    }
    if (event.key === 'Enter') {
      if (isOpen && activeIndex >= 0 && activeIndex < hits.length) {
        event.preventDefault();
        select(hits[activeIndex]);
      }
    }
  };

  return (
    <div className="relative w-full max-w-md">
      <label htmlFor="quick-search-input" className="sr-only">
        Search
      </label>
      <div className="relative">
        <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          id="quick-search-input"
          ref={inputRef}
          type="search"
          role="combobox"
          aria-expanded={isOpen}
          aria-controls={listboxId}
          aria-activedescendant={activeIndex >= 0 ? `quick-search-option-${activeIndex}` : undefined}
          aria-autocomplete="list"
          autoComplete="off"
          placeholder="Search…"
          value={rawValue}
          onChange={(event) => {
            setRawValue(event.target.value);
          }}
          onFocus={() => {
            if (debouncedValue.trim() !== '' && hits.length > 0) {
              setIsOpen(true);
            }
          }}
          onKeyDown={handleKeyDown}
          className="pl-8"
        />
      </div>
      {isOpen ? (
        <ul
          id={listboxId}
          role="listbox"
          aria-label="Search results"
          className="absolute left-0 right-0 top-full z-50 mt-1 max-h-80 overflow-auto rounded-lg border bg-popover p-1 shadow-md"
        >
          {hits.map((hit, index) => {
            const confidence = formatConfidence(hit.confidence);
            return (
              <li
                key={`${hit.type}-${hit.id}`}
                id={`quick-search-option-${index}`}
                role="option"
                aria-selected={index === activeIndex}
                onClick={() => select(hit)}
                className={cn(
                  'cursor-pointer rounded-md px-3 py-2 text-sm',
                  index === activeIndex ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/70',
                )}
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">
                      <span className="mr-1.5 rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                        {hit.type.replace('_', ' ')}
                      </span>
                      {hit.title}
                    </div>
                    {hit.subtitle ? (
                      <div className="truncate text-xs text-muted-foreground">{hit.subtitle}</div>
                    ) : null}
                  </div>
                  {confidence ? (
                    <span className="shrink-0 text-xs text-muted-foreground">{confidence}</span>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}
