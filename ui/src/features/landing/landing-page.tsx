/**
 * Landing page (task 7.2 / design D7): the app's home at `/`, rendered
 * directly (no redirect). Mobile-first at 360px; AppShell provides the top
 * bar (brand + ☰ hamburger + theme toggle) and page padding, so this page
 * only adds its own horizontal centering (AppShell's `<main>` has none).
 *
 * Top-to-bottom (per the landing-page spec):
 *   1. Centered PROCRASTINATOR hero wordmark — a text (not image) brand mark
 *      that links back to `/`. Letter-spaced and responsive so it fits a
 *      360px viewport without overflowing.
 *   2. Large search bar — a plain form that navigates to `/search?q=<query>`
 *      on submit (the search page reads `q`).
 *   3. Centered [+] button — the primary ingest entry point. It reveals the
 *      unified "Add" composer (a describe-first textarea + attach strip +
 *      optional directive note; a 409 duplicate opens the reprocess/keep
 *      dialog). The button is an accessible, idempotent toggle.
 *   4. Labeled "Insights" placeholder section.
 *   5. Fixed, disabled chat-bar placeholder pill at the bottom of the
 *      viewport (z-30: above content, below the z-40 header and z-50 nav
 *      sheet). The page carries bottom padding so content is never hidden
 *      behind it.
 */
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { Lightbulb, MessageCircle, Plus, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { AddComposer } from '@/features/docs/composer/add-composer';

export function LandingPage() {
  const [query, setQuery] = useState('');
  const [ingestOpen, setIngestOpen] = useState(false);
  const navigate = useNavigate();

  const submitSearch = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = query.trim();
    navigate(trimmed === '' ? '/search' : `/search?q=${encodeURIComponent(trimmed)}`);
  };

  return (
    <div className="mx-auto max-w-3xl space-y-10 pb-28">
      {/* 1 — PROCRASTINATOR hero wordmark (tap → /). Text brand mark, not an
          image; large, bold, uppercase, tracked. Responsive scale + tighter
          tracking on small screens so 14 chars fit a 360px viewport, and a
          ≥44px tap target. */}
      <div className="flex justify-center">
        <Link
          to="/"
          className="inline-flex min-h-11 items-center justify-center rounded px-2 py-2 text-2xl font-bold uppercase tracking-[0.18em] whitespace-nowrap sm:text-3xl sm:tracking-[0.25em] md:text-5xl md:tracking-[0.3em]"
        >
          PROCRASTINATOR
        </Link>
      </div>

      {/* 2 — Large search bar */}
      <form role="search" onSubmit={submitSearch} className="space-y-1.5">
        <label htmlFor="landing-search" className="sr-only">
          Search your stuff
        </label>
        <div className="relative">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute left-3 top-1/2 size-5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            id="landing-search"
            type="search"
            placeholder="Search your stuff…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="h-12 pl-10 text-base md:text-sm"
          />
        </div>
      </form>

      {/* 3 — Centered [+] ingest entry point */}
      <div className="flex justify-center">
        <Button
          size="icon"
          className="size-14 rounded-full text-xl"
          aria-label="Add something"
          aria-expanded={ingestOpen}
          onClick={() => setIngestOpen((open) => !open)}
        >
          <Plus aria-hidden="true" className="size-6" />
        </Button>
      </div>

      {/* Unified "Add" composer (task 14.1): controlled dialog/sheet,
          rendered unconditionally so it self-gates on `open`. */}
      <AddComposer open={ingestOpen} onOpenChange={setIngestOpen} />

      {/* 4 — Labeled Insights placeholder */}
      <section aria-labelledby="landing-insights-heading" className="space-y-4">
        <div className="flex items-center gap-2 text-muted-foreground">
          <Lightbulb aria-hidden="true" className="size-4" />
          <h2 id="landing-insights-heading" className="text-lg font-semibold">
            Insights
          </h2>
        </div>
        <Card>
          <CardContent className="text-sm text-muted-foreground">
            Insights widgets will land here later.
          </CardContent>
        </Card>
      </section>

      {/* 5 — Fixed, disabled chat-bar placeholder pill. z-30 stays below the
          z-40 header and z-50 nav sheet; the disabled button is not
          focusable/interactive, so it traps no focus. */}
      <div className="fixed inset-x-4 bottom-4 z-30 mx-auto max-w-3xl">
        <Button disabled className="w-full justify-start gap-2">
          <MessageCircle aria-hidden="true" className="size-4 shrink-0" />
          <span className="truncate">Ask about your stuff…</span>
        </Button>
      </div>
    </div>
  );
}
