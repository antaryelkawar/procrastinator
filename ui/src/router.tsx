/**
 * Application route table (design D9 — route-surface cleanup).
 *
 * Seven reachable routes, every other path hits the `*` not-found:
 *
 *   /                        landing page
 *   /search                  search results
 *   /ingest/reviews          review queue
 *   /assets                  asset list
 *   /assets/:assetId         asset detail
 *   /documents               documents list
 *   /finance/accounts        finance accounts
 *
 * Five legacy routes redirect (via <Navigate replace>) to their closest
 * surviving view:
 *
 *   /add                        → /
 *   /finance/movements          → /finance/accounts
 *   /finance/import             → /finance/accounts
 *   /finance/import/history     → /finance/accounts
 *   /finance/import/:batchId    → /finance/accounts
 *
 * Every screen renders inside <AppShell /> — a single top bar (brand + ☰
 * hamburger trigger + theme toggle) and the shared right-side navigation
 * sheet. There is no sidebar and no permanent nav list; the hamburger sheet
 * is the only navigation mechanism on every route.
 */
import { Link, Navigate, Route, Routes } from 'react-router';
import { AppShell } from '@/components/layout/app-shell';
import { EmptyState } from '@/components/feedback/empty-state';
import { Button } from '@/components/ui/button';
import { AssetListPage } from './features/assets/asset-list-page';
import { AssetDetailPage } from './features/assets/asset-detail-page';
import { DocumentsPage } from './features/documents/documents-page';
import { AccountsPage } from './features/finance/accounts-page';
import { SearchResultsPage } from './features/search/search-results-page';
import { LandingPage } from './features/landing/landing-page';
import { ReviewQueuePage } from './features/reviews/review-queue-page';

function NotFoundPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">Page not found</h1>
      <EmptyState
        title="This page does not exist"
        description="The link may be broken, or the page may have been moved."
      >
        <Button asChild>
          <Link to="/assets">Go to assets</Link>
        </Button>
      </EmptyState>
    </div>
  );
}

/** The full route table, rendered inside the app shell. */
export function AppRoutes() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/" element={<LandingPage />} />
        <Route path="/add" element={<Navigate to="/" replace />} />
        <Route path="/assets" element={<AssetListPage />} />
        <Route path="/assets/:assetId" element={<AssetDetailPage />} />
        <Route path="/documents" element={<DocumentsPage />} />
        <Route path="/finance/accounts" element={<AccountsPage />} />
        <Route path="/finance/movements" element={<Navigate to="/finance/accounts" replace />} />
        <Route path="/finance/import" element={<Navigate to="/finance/accounts" replace />} />
        <Route path="/finance/import/history" element={<Navigate to="/finance/accounts" replace />} />
        <Route path="/finance/import/:batchId" element={<Navigate to="/finance/accounts" replace />} />
        <Route path="/search" element={<SearchResultsPage />} />
        <Route path="/ingest/reviews" element={<ReviewQueuePage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
