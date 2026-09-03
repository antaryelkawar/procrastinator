import { CircleAlert } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export interface ErrorStateProps {
  /** Human-readable error copy (mapped from the API status by `lib/api/errors`). */
  readonly message: string;
  /** Retry action; when omitted no retry button is rendered. */
  readonly onRetry?: () => void;
  readonly title?: string;
  readonly className?: string;
}

/**
 * Shared error state (spec: "Failed load shows an error, not a blank screen"
 * and "Network failure offers retry"). Renders the mapped message plus a
 * retry action that re-issues the failed request.
 */
export function ErrorState({
  message,
  onRetry,
  title = 'Something went wrong',
  className,
}: ErrorStateProps) {
  return (
    <div
      role="alert"
      className={cn(
        'flex flex-col items-center gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-6 text-center',
        className,
      )}
    >
      <CircleAlert aria-hidden="true" className="size-6 text-destructive" />
      <div className="space-y-1">
        <h2 className="text-base font-semibold">{title}</h2>
        <p className="text-sm text-muted-foreground">{message}</p>
      </div>
      {onRetry !== undefined ? (
        <Button variant="outline" size="sm" onClick={onRetry}>
          Retry
        </Button>
      ) : null}
    </div>
  );
}
