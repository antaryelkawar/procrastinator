import { useNavigate } from 'react-router-dom';
import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { useAssets } from '@/lib/api/hooks';
import { formatDate, isWarrantyExpired } from '@/lib/format/date';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/feedback/empty-state';
import { Loading } from '@/components/feedback/loading';
import { ErrorState } from '@/components/feedback/error-state';
import { DataTable, features } from '@/components/data-table';
import type { Asset } from '@/lib/api/schema';

const columnHelper = createColumnHelper<typeof features, Asset>();

/**
 * Asset list columns, rendered through the shared DataTable
 * (consolidated-primitives decision: one table component, no per-page
 * hand-rolled tables). For 8.4 name, brand, and model are distinct columns,
 * with purchase date and a warranty status marker (string date comparison —
 * no `new Date()` on API values).
 */
function buildColumns(): ColumnDef<TableFeatures, Asset, unknown>[] {
  return [
    columnHelper.accessor('name', {
      header: 'Name',
      cell: (context) => context.getValue() ?? '-',
    }),
    columnHelper.accessor('brand', {
      header: 'Brand',
      cell: (context) => context.getValue() ?? '-',
    }),
    columnHelper.accessor('model', {
      header: 'Model',
      cell: (context) => context.getValue() ?? '-',
    }),
    columnHelper.accessor('asset_category', {
      header: 'Category',
      cell: (context) => (
        <Badge variant="outline">{context.getValue() ?? 'other'}</Badge>
      ),
    }),
    columnHelper.accessor('purchase_date', {
      header: 'Purchase date',
      cell: (context) => {
        const value = context.getValue();
        return value ? formatDate(value) : '-';
      },
    }),
    columnHelper.accessor('warranty_end', {
      header: 'Warranty',
      cell: (context) => {
        const warrantyEnd = context.getValue();
        if (!warrantyEnd) {
          return <span>-</span>;
        }
        const expired = isWarrantyExpired(warrantyEnd);
        return (
          <span className="inline-flex items-center gap-2">
            <span className={expired ? 'text-red-600 font-medium' : ''}>
              {formatDate(warrantyEnd)}
            </span>
            {expired ? (
              // Solid destructive fill (bg-destructive text-white) for WCAG AA
              // contrast on the 12px label — the default destructive variant
              // (red-on-pale-red) measures 4.0:1, below the 4.5:1 floor.
              <Badge variant="destructive" className="bg-destructive text-white">
                Expired
              </Badge>
            ) : (
              <Badge variant="outline">Active</Badge>
            )}
          </span>
        );
      },
    }),
    columnHelper.accessor('serial_number', {
      header: 'Serial number',
      cell: (context) => context.getValue() ?? '-',
    }),
  ] as ColumnDef<TableFeatures, Asset, unknown>[];
}

export function AssetListPage() {
  const navigate = useNavigate();
  const { data: assets, isLoading, error } = useAssets();

  if (isLoading) return <Loading />;
  if (error) return <ErrorState title="Failed to load assets" message={error.message} />;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Assets</h1>

      {!assets || assets.length === 0 ? (
        <EmptyState
          title="No assets yet"
          description="Add your first asset or upload a document to get started."
        >
          <Button onClick={() => navigate('/add')}>Add something</Button>
        </EmptyState>
      ) : (
        <DataTable
          ariaLabel="Assets"
          data={assets}
          getRowId={(asset) => asset.id}
          columns={buildColumns()}
          onRowClick={(asset) => navigate(`/assets/${asset.id}`)}
        />
      )}
    </div>
  );
}
