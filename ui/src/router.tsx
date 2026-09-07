/**
 * Application route table (design D9 / module layout).
 *
 *   /                         → redirects to /assets
 *   /add                       unified add (file/text/statement) (task 8.2)
 *   /assets                    asset list                 (task 4.2)
 *   /assets/:assetId           asset detail               (task 4.3)
 *   /finance/accounts          finance accounts           (task 4.4)
 *   /finance/movements         money movements            (task 4.5)
 *   /finance/import            statement import           (task 4.9)
 *   /finance/import/history    import history             (task 4.10)
 *   /finance/import/:batchId   batch detail               (task 4.10)
 *
 * Every screen renders inside <AppShell /> (sidebar / mobile nav sheet /
 * active-user switcher). Until the 4.x screens land, each route renders a
 * lightweight placeholder (see `pages/placeholders.tsx`) so the shell and
 * navigation are fully testable now. Static segments rank above dynamic
 * ones, so `/finance/import/history` wins over `/finance/import/:batchId`.
 */
import { Link, Navigate, Route, Routes } from 'react-router-dom';
import { AppShell } from '@/components/layout/app-shell';
import { EmptyState } from '@/components/feedback/empty-state';
import { Button } from '@/components/ui/button';
import { AddPage } from './pages/add/add-page';
import { AssetListPage } from './pages/assets/asset-list-page';
import { AssetDetailPage } from './pages/assets/asset-detail-page';
import { AccountsPage } from './pages/finance/accounts-page';
import { MovementsPage } from './pages/finance/movements-page';
import { ImportPage } from './pages/finance/import-page';
import { ImportHistoryPage } from './pages/finance/import-history-page';
import { SearchResultsPage } from './pages/search/search-results-page';
import { ReviewQueuePage } from './pages/reviews/review-queue-page';

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
        <Route path="/" element={<Navigate to="/assets" replace />} />
        <Route path="/add" element={<AddPage />} />
        <Route path="/assets" element={<AssetListPage />} />
        <Route path="/assets/:assetId" element={<AssetDetailPage />} />
        <Route path="/finance/accounts" element={<AccountsPage />} />
        <Route path="/finance/movements" element={<MovementsPage />} />
        <Route path="/finance/import" element={<ImportPage />} />
        <Route path="/finance/import/history" element={<ImportHistoryPage />} />
        <Route path="/finance/import/:batchId" element={<ImportHistoryPage />} />
        <Route path="/search" element={<SearchResultsPage />} />
        <Route path="/ingest/reviews" element={<ReviewQueuePage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
