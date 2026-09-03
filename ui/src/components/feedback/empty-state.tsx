import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

export interface EmptyStateProps {
  readonly title: string;
  /** Explanatory copy under the title. */
  readonly description?: ReactNode;
  /** Optional call-to-action (e.g. an upload button or link). */
  readonly children?: ReactNode;
  readonly className?: string;
}

/**
 * Shared empty state (spec: "Empty registry shows guidance" — an explicit
 * empty state with a call to action, not a bare blank table).
 */
export function EmptyState({ title, description, children, className }: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center gap-3 rounded-lg border border-dashed p-8 text-center sm:p-12',
        className,
      )}
    >
      <h2 className="text-base font-semibold">{title}</h2>
      {description !== undefined ? (
        <div className="max-w-md text-sm text-muted-foreground">{description}</div>
      ) : null}
      {children !== undefined ? (
        <div className="mt-2 flex flex-wrap items-center justify-center gap-2">{children}</div>
      ) : null}
    </div>
  );
}
