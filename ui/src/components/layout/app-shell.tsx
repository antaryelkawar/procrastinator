/**
 * App shell (design D9): the responsive frame around every screen.
 *
 * - Permanent sidebar at `md` and up.
 * - Top bar with a hamburger opening a Sheet below `md`.
 * - Always-visible active-user switcher (spec: the active tenant identifier
 *   is visible in the UI at all times) — bottom of the sidebar at md+,
 *   mobile top bar below md — backed by the task 2.3 context: switching
 *   users validates the id and clears the query cache (D4).
 * - `<main>` landmark hosting the active screen via `<Outlet/>`.
 *
 * The route table lives in `src/router.tsx`.
 */
import { useEffect, useRef, useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import {
  ArrowLeftRight,
  ClipboardList,
  FileUp,
  History,
  Landmark,
  Menu,
  Package,
  Search,
  Upload,
  Wallet,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useActiveUser } from '@/context/active-user';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import { QuickSearch } from '@/components/search/quick-search';

interface NavItem {
  readonly to: string;
  readonly label: string;
  readonly icon: LucideIcon;
  /** Match the link exactly (so `/finance/import` is not active on its child routes). */
  readonly end?: boolean;
}

/** Primary navigation — the eight screens of the route table. */
const NAV_ITEMS: readonly NavItem[] = [
  { to: '/upload', label: 'Upload', icon: Upload },
  { to: '/assets', label: 'Assets', icon: Package },
  { to: '/search', label: 'Search', icon: Search },
  { to: '/ingest/reviews', label: 'Review queue', icon: ClipboardList },
  { to: '/finance/accounts', label: 'Accounts', icon: Wallet },
  { to: '/finance/movements', label: 'Movements', icon: ArrowLeftRight },
  { to: '/finance/import', label: 'Import', icon: FileUp, end: true },
  { to: '/finance/import/history', label: 'Import history', icon: History },
];

function Brand() {
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
        <Landmark aria-hidden="true" className="size-4" />
      </span>
      <span className="truncate text-sm font-semibold">Procrastinator</span>
    </div>
  );
}

function AppNav({
  ariaLabel,
  large = false,
  onNavigate,
}: {
  ariaLabel: string;
  large?: boolean;
  onNavigate?: () => void;
}) {
  return (
    <nav aria-label={ariaLabel} className="flex flex-col gap-1">
      {NAV_ITEMS.map((item) => {
        const Icon = item.icon;
        return (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-lg px-3 transition-colors',
                large ? 'py-2.5' : 'py-2',
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-foreground/70 hover:bg-accent/70 hover:text-foreground',
              )
            }
          >
            <Icon aria-hidden="true" className="size-4 shrink-0" />
            <span className="truncate text-sm font-medium">{item.label}</span>
          </NavLink>
        );
      })}
    </nav>
  );
}

interface ActiveUserSwitcherProps {
  /** Unique input id (the shell renders one switcher per visible surface). */
  readonly id: string;
  /** Render the field label visibly (desktop) instead of sr-only (mobile). */
  readonly showLabel?: boolean;
  /** Use larger touch targets (44px). */
  readonly touch?: boolean;
  readonly className?: string;
}

/**
 * Active-user switcher. Drafts the id, validates + applies it through the
 * task 2.3 context (`setActiveUser`), which persists to localStorage and
 * clears the query cache so no data from the previous user survives (D4).
 */
function ActiveUserSwitcher({ id, showLabel = false, touch = false, className }: ActiveUserSwitcherProps) {
  const { activeUser, setActiveUser } = useActiveUser();
  const [draft, setDraft] = useState(activeUser ?? '');
  const [rejected, setRejected] = useState(false);

  // Keep the draft in sync with programmatic switches (e.g. the other
  // switcher surface, or a user switch clearing the cache).
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
      className={cn('flex items-center gap-2', className)}
      onSubmit={(event) => {
        event.preventDefault();
        apply();
      }}
    >
      <Label htmlFor={id} className={cn('shrink-0', !showLabel && 'sr-only')}>
        Active user
      </Label>
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
        className={cn('min-w-0 flex-1', touch ? 'h-11' : 'h-8')}
      />
      <Button
        type="submit"
        size="sm"
        className={cn(touch && 'h-11 px-4')}
        aria-label="Set active user"
        disabled={draft.trim() === ''}
      >
        Set
      </Button>
      {rejected ? (
        <p id={`${id}-hint`} role="alert" className="sr-only">
          Invalid user id. Use letters, numbers, underscores, or hyphens (1 to 64 characters).
        </p>
      ) : null}
    </form>
  );
}

export function AppShell() {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);

  return (
    <div className="min-h-dvh bg-background md:flex">
      {/* Sidebar — permanent at md and up */}
      <aside className="hidden w-64 shrink-0 flex-col border-r bg-sidebar p-4 md:flex">
        <Brand />
        <div className="mt-4">
          <QuickSearch />
        </div>
        <div className="mt-6 flex-1">
          <AppNav ariaLabel="Primary" />
        </div>
        <div className="mt-6 space-y-2 border-t border-sidebar-border pt-4">
          <ActiveUserSwitcher id="user-switcher-sidebar" showLabel />
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        {/* Top bar — below md only */}
        <header className="sticky top-0 z-40 border-b bg-background md:hidden">
          <div className="flex flex-wrap items-center gap-2 px-4 pt-3">
            <Button
              ref={menuButtonRef}
              variant="ghost"
              size="icon"
              className="size-11"
              aria-label="Open navigation menu"
              onClick={() => setMobileNavOpen(true)}
            >
              <Menu aria-hidden="true" />
            </Button>
            <Brand />
            <div className="w-full mt-2">
              <QuickSearch />
            </div>
            <div className="w-full mt-2">
              <ActiveUserSwitcher id="user-switcher-mobile" touch />
            </div>
          </div>
        </header>

        {/* Mobile navigation sheet — below md only */}
        <Sheet open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
          <SheetContent
            side="left"
            className="w-full gap-0 p-0 sm:max-w-xs"
            onCloseAutoFocus={(event) => {
              // Ensure focus returns to the hamburger trigger on close (F1).
              event.preventDefault();
              menuButtonRef.current?.focus();
            }}
          >
            <SheetHeader className="border-b">
              <SheetTitle>Navigation</SheetTitle>
              <SheetDescription>Choose where to go</SheetDescription>
            </SheetHeader>
            <div className="flex-1 overflow-y-auto p-4">
              <AppNav
                large
                ariaLabel="Mobile navigation"
                onNavigate={() => setMobileNavOpen(false)}
              />
            </div>
          </SheetContent>
        </Sheet>

        <main id="main-content" className="min-w-0 flex-1 overflow-x-hidden p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
