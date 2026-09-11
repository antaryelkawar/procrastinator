/**
 * App shell (task 7.1, chrome rewritten in task 13.2): the responsive frame
 * around every screen.
 *
 * No top bar. The sole chrome element is a fixed top-left floating ☰
 * hamburger (≥44px) on every view, including the landing page. It opens the
 * shared navigation sheet (right side) — the ONLY navigation mechanism in the
 * app. There is no sidebar and no permanent nav list anywhere.
 *
 * Profile + theme live inside the nav sheet (task 13.3), not in the shell.
 * `<main id="main-content">` hosts the active screen via `<Outlet/>`; its
 * top padding reserves a strip so page content never hides under the ☰.
 */
import { useRef, useState } from 'react';
import { Outlet } from 'react-router';
import { Menu } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NavSheet } from '@/components/nav-sheet/nav-sheet';
import { Toaster } from '@/components/ui/sonner';

export function AppShell() {
  const [navSheetOpen, setNavSheetOpen] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);

  return (
    <div className="flex min-h-dvh flex-col bg-background">
      {/* Sole chrome (task 13.2): fixed top-left floating ☰. z-40 sits above
          page content and the landing chat pill (z-30), below the nav sheet
          scrim/panel (z-50). Opaque bg + shadow so it reads as floating. */}
      <Button
        ref={menuButtonRef}
        variant="outline"
        size="icon"
        className="fixed left-4 top-4 z-40 size-12 rounded-full bg-background shadow-md"
        aria-label="Open navigation"
        onClick={() => setNavSheetOpen(true)}
      >
        <Menu aria-hidden="true" />
      </Button>

      <NavSheet open={navSheetOpen} onOpenChange={setNavSheetOpen} triggerRef={menuButtonRef} />

      <main id="main-content" className="min-w-0 flex-1 overflow-x-hidden px-4 pb-4 pt-20 md:px-6 md:pb-6">
        <Outlet />
      </main>

      {/* Ingest / duplicate-choice / timeout toasts (task 7.3). `top-center`
          avoids the fixed chat pill (bottom) and the top-left ☰. */}
      <Toaster richColors position="top-center" />
    </div>
  );
}
