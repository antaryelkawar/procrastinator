import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

export interface LoadingProps {
  /** Accessible announcement read by screen readers while loading. */
  readonly label?: string;
  /** Number of skeleton rows to render. */
  readonly rows?: number;
  readonly className?: string;
}

/**
 * Shared loading state — part of the four-state screen contract (spec:
 * "Consistent error and loading feedback"). A screen whose data is in flight
 * renders <Loading /> instead of a blank area.
 */
export function Loading({ label = 'Loading', rows = 4, className }: LoadingProps) {
  return (
    <div role="status" className={cn('space-y-3', className)}>
      <span className="sr-only">{label}…</span>
      {Array.from({ length: rows }, (_, index) => (
        <Skeleton key={index} className="h-10 w-full" />
      ))}
    </div>
  );
}
