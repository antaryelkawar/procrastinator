/**
 * Shared top-right [+] chrome control (task 13.4; app-chrome spec "Context [+]
 * on every non-landing view", design D10). A section page drops this in to
 * offer "add the thing local to this section" (assets/documents/finance/
 * reviews in 14.x). Pure presentational: no data hooks, no state, no router
 * usage — the caller supplies the section label and the `onAdd` callback that
 * opens the section's composer flow.
 *
 * Mirrors the fixed top-left ☰ chrome in
 * `ui/src/components/layout/app-shell.tsx:30-39`, flipped to top-right.
 * Lives in `features/docs` because `docs` is the only allowed shared
 * cross-feature target (`ui/eslint.config.js`).
 */
import { Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export interface AddButtonProps {
  /**
   * The concrete per-section action label, e.g. `"Add asset"`. Becomes the
   * button's accessible name (aria-label).
   */
  readonly label: string;
  /** Opens the section's composer flow (supplied by the consuming feature). */
  readonly onAdd: () => void;
  /** Optional extra classes, merged via `cn`. */
  readonly className?: string;
}

/**
 * Fixed top-right [+] context control. `z-40` sits above page content and the
 * landing chat pill (z-30) and below the nav-sheet scrim/panel (z-50).
 * `size-12` = 48px ≥ 44px touch-target minimum (the `size="icon"` base is only
 * 32px; `cn`/tailwind-merge keeps `size-12` over the base `size-8`).
 */
export function AddButton({ label, onAdd, className }: AddButtonProps) {
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      aria-label={label}
      onClick={onAdd}
      // The core chrome (fixed top-right, size-12 = 48px ≥ 44px, rounded, z-40,
      // floating surface) is AUTHORITATIVE: it is merged AFTER any consumer
      // `className`, so tailwind-merge keeps it even if a 14.x consumer passes
      // a conflicting utility (e.g. a smaller `size-*`). Non-conflicting
      // additions (margins, data-*, hover states) still merge in.
      className={cn(className, 'fixed right-4 top-4 z-40 size-12 rounded-full bg-background shadow-md')}
    >
      <Plus aria-hidden="true" className="size-5" />
    </Button>
  );
}
