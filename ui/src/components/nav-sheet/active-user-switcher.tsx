/**
 * Active-user switcher (extracted from the app shell, task 7.1).
 *
 * Drafts the id, validates + applies it through the task 2.3 context
 * (`setActiveUser`), which persists to localStorage and clears the query
 * cache so no data from the previous user survives (D4). The nav-sheet
 * profile row is the single visible surface for this control.
 */
import { useEffect, useState } from 'react';
import { useActiveUser } from '@/context/active-user';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';

interface ActiveUserSwitcherProps {
  /** Unique input id. */
  readonly id: string;
  /** Render the field label visibly instead of sr-only. */
  readonly showLabel?: boolean;
  readonly className?: string;
}

export function ActiveUserSwitcher({ id, showLabel = true, className }: ActiveUserSwitcherProps) {
  const { activeUser, setActiveUser } = useActiveUser();
  const [draft, setDraft] = useState(activeUser ?? '');
  const [rejected, setRejected] = useState(false);

  // Keep the draft in sync with programmatic switches (e.g. the context
  // being reset by another surface, or a user switch clearing the cache).
  useEffect(() => {
    setDraft(activeUser ?? '');
    setRejected(false);
  }, [activeUser]);

  const apply = (): void => {
    const next = draft.trim();
    if (next === '') {
      return;
    }
    setRejected(!setActiveUser(next));
  };

  return (
    <form
      className={cn('flex flex-col gap-2', className)}
      onSubmit={(event) => {
        event.preventDefault();
        apply();
      }}
    >
      <Label htmlFor={id} className={cn(!showLabel && 'sr-only')}>
        Active user
      </Label>
      <div className="flex items-center gap-2">
        <Input
          id={id}
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value);
            setRejected(false);
          }}
          placeholder="user"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={rejected || undefined}
          aria-describedby={rejected ? `${id}-hint` : undefined}
          className="min-w-0 flex-1 h-11"
        />
        <Button
          type="submit"
          size="sm"
          className="h-11 px-4 shrink-0"
          aria-label="Set active user"
          disabled={draft.trim() === ''}
        >
          Set
        </Button>
      </div>
      {rejected ? (
        <p id={`${id}-hint`} role="alert" className="sr-only">
          Invalid user id. Use letters, numbers, underscores, or hyphens (1 to 64 characters).
        </p>
      ) : null}
    </form>
  );
}
