/**
 * Shared hamburger navigation sheet (task 7.1).
 *
 * The ONLY navigation mechanism in the app, on every route. Slides in from
 * the right and holds:
 *
 * 1. SheetHeader with a ✕ close button (SheetContent default).
 * 2. A profile row (circular avatar + active-user id) — tapping it opens a
 *    nested panel containing the ActiveUserSwitcher (the app's
 *    profile/active-user context).
 * 3. Nav links: Home (first), then Review Queue, Assets, Documents, Accounts.
 *    Each navigates and closes the sheet on tap.
 * 4. A theme toggle row (the app's `ThemeToggle` select) on its own grouped
 *    line between the nav links and the TBD rows.
 * 5. Three visibly-disabled TBD placeholder rows (not focusable, not links).
 *
 * Controlled by the parent shell (`open` / `onOpenChange`). Focus returns to
 * the ☰ trigger on close.
 */
import { useState } from 'react';
import { NavLink } from 'react-router';
import {
  ClipboardList,
  Inbox,
  Package,
  User,
  Wallet,
} from 'lucide-react';
import { Home as HomeIcon } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useActiveUser } from '@/context/active-user';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import { ThemeToggle } from '@/components/ui/theme-toggle';
import { ActiveUserSwitcher } from './active-user-switcher';

export interface NavSheetProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** Called by SheetContent onCloseAutoFocus to refocus the trigger. */
  readonly triggerRef: React.RefObject<HTMLButtonElement | null>;
}

interface NavItem {
  readonly to: string;
  readonly label: string;
  readonly icon: LucideIcon;
  /** Exact-match only (e.g. Home `/` must not highlight on every route). */
  readonly end?: boolean;
}

/** The primary nav links: Home first, then the four section links (task 13.3). */
const NAV_ITEMS: readonly NavItem[] = [
  { to: '/', label: 'Home', icon: HomeIcon, end: true },
  { to: '/ingest/reviews', label: 'Review Queue', icon: ClipboardList },
  { to: '/assets', label: 'Assets', icon: Package },
  { to: '/documents', label: 'Documents', icon: Inbox },
  { to: '/finance/accounts', label: 'Accounts', icon: Wallet },
];

/** Visually present but inert TBD placeholders — not links, not focusable. */
const TBD_ITEMS: ReadonlyArray<{ label: string; icon: LucideIcon }> = [
  { label: 'Subscriptions', icon: Package },
  { label: 'Relationships', icon: User },
  { label: 'Inventory', icon: Inbox },
];

function ProfileRow({
  onOpenProfile,
}: {
  readonly onOpenProfile: () => void;
}) {
  const { activeUser } = useActiveUser();
  const name = activeUser ?? 'Not signed in';

  return (
    <button
      type="button"
      onClick={onOpenProfile}
      className="flex w-full items-center gap-3 rounded-lg px-3 py-3 min-h-11 text-left transition-colors hover:bg-accent"
    >
      {activeUser ? (
        <span
          aria-hidden="true"
          className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground text-sm font-semibold uppercase"
        >
          {activeUser.slice(0, 2)}
        </span>
      ) : (
        <span
          aria-hidden="true"
          className="flex size-10 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
        >
          <User className="size-5" />
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">
          {activeUser ? name : 'Choose a user'}
        </span>
        <span className="block text-xs text-muted-foreground">
          {activeUser ? 'Active user' : 'Tap to sign in'}
        </span>
      </span>
    </button>
  );
}

function ProfilePanel() {
  return (
    <div
      role="group"
      aria-label="Active user"
      className="space-y-2 border-t border-border bg-background/95 p-4"
    >
      <p className="text-xs font-medium text-muted-foreground">
        Switch active user
      </p>
      <ActiveUserSwitcher id="user-switcher-profile" />
    </div>
  );
}

export function NavSheet({ open, onOpenChange, triggerRef }: NavSheetProps) {
  const [profilePanelOpen, setProfilePanelOpen] = useState(false);

  const handleNavigate = (): void => {
    setProfilePanelOpen(false);
    onOpenChange(false);
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="left"
        className="w-full sm:max-w-sm gap-0 p-0 flex flex-col"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus();
        }}
      >
        <SheetHeader className="border-b border-border">
          <SheetTitle>Menu</SheetTitle>
          <SheetDescription>Choose where to go</SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {/* Profile row */}
          <section aria-label="Profile">
            <ProfileRow onOpenProfile={() => setProfilePanelOpen((v) => !v)} />
            {profilePanelOpen ? <ProfilePanel /> : null}
          </section>

          {/* Nav links */}
          <nav aria-label="Primary navigation" className="flex flex-col gap-1">
            {NAV_ITEMS.map((item) => {
              const Icon = item.icon;
              return (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  onClick={handleNavigate}
                  className={({ isActive }) =>
                    cn(
                      'flex min-h-11 items-center gap-3 rounded-lg px-3 py-3 text-sm font-medium transition-colors',
                      isActive
                        ? 'bg-accent text-accent-foreground'
                        : 'text-foreground/70 hover:bg-accent/70 hover:text-foreground',
                    )
                  }
                >
                  <Icon aria-hidden="true" className="size-4 shrink-0" />
                  <span className="truncate">{item.label}</span>
                </NavLink>
              );
            })}
          </nav>

          {/* Theme toggle */}
          <section
            aria-label="Appearance"
            className="flex flex-col gap-1 border-t border-border pt-3"
          >
            <p className="px-1 text-xs font-medium text-muted-foreground">
              Appearance
            </p>
            <div className="flex min-h-11 items-center">
              <ThemeToggle />
            </div>
          </section>

          {/* Disabled TBD placeholders */}
          <div aria-label="Coming soon" className="flex flex-col gap-1 border-t border-border pt-3">
            {TBD_ITEMS.map((item) => {
              const Icon = item.icon;
              return (
                <div
                  key={item.label}
                  aria-disabled="true"
                  className="flex min-h-11 items-center gap-3 rounded-lg px-3 py-3 opacity-50"
                >
                  <Icon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
                  <span className="flex-1 truncate text-sm font-medium text-muted-foreground">
                    {item.label}
                  </span>
                  <span className="text-xs uppercase tracking-wider text-muted-foreground">TBD</span>
                </div>
              );
            })}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
