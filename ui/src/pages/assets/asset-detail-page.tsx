import { useParams } from 'react-router-dom';
import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { useAsset, useAssetDocuments } from '@/lib/api/hooks';
import { formatDate } from '@/lib/format/date';
import { formatMoney } from '@/lib/format/money';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loading } from '@/components/feedback/loading';
import { ErrorState } from '@/components/feedback/error-state';
import { DataTable, features } from '@/components/data-table';
import type { Document } from '@/lib/api/schema';

const columnHelper = createColumnHelper<typeof features, Document>();

/**
 * Documents table columns, rendered through the shared DataTable
 * (consolidated-primitives decision: one table component).
 */
function buildDocumentColumns(): ColumnDef<TableFeatures, Document, unknown>[] {
  return [
    columnHelper.accessor('source_filename', {
      header: 'Filename',
    }),
    columnHelper.accessor('doc_type', {
      header: 'Type',
    }),
    columnHelper.accessor('source_uploaded_at', {
      header: 'Uploaded',
      cell: (context) => formatDate(context.getValue()),
    }),
  ] as ColumnDef<TableFeatures, Document, unknown>[];
}

export function AssetDetailPage() {
  const { assetId } = useParams<{ assetId: string }>();
  const { data: asset, isLoading: assetLoading, error: assetError } = useAsset(assetId ?? '');
  const { data: docs, isLoading: docsLoading, error: docsError } = useAssetDocuments(assetId ?? '');

  if (assetLoading || docsLoading) return <Loading />;

  if (assetError || docsError) {
    if ((assetError as any)?.status === 404) {
      return (
        <div className="space-y-6">
          <ErrorState
            title="Asset not found"
            message="The requested asset does not exist."
          />
        </div>
      );
    }
    return <ErrorState title="Failed to load asset" message={(assetError || docsError)?.message ?? 'Unknown error'} />;
  }

  if (!asset) return null;

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-semibold">
          {asset.brand ?? 'Asset'} {asset.model ?? ''}
        </h1>
        <Badge variant="outline">{asset.asset_category ?? 'other'}</Badge>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Asset Details</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <p className="text-sm text-muted-foreground">Serial Number</p>
            <p className="font-medium">{asset.serial_number ?? '-'}</p>
          </div>
          <div>
            <p className="text-sm text-muted-foreground">Purchase Date</p>
            <p className="font-medium">{asset.purchase_date ? formatDate(asset.purchase_date) : '-'}</p>
          </div>
          <div>
            <p className="text-sm text-muted-foreground">Warranty Ends</p>
            <p className="font-medium">{asset.warranty_end ? formatDate(asset.warranty_end) : '-'}</p>
          </div>
          <div>
            <p className="text-sm text-muted-foreground">Price</p>
            <p className="font-medium">
              {asset.price && asset.currency
                ? formatMoney(asset.price, asset.currency)
                : '-'}
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Documents</CardTitle>
        </CardHeader>
        <CardContent>
          {!docs || docs.length === 0 ? (
            <p className="text-muted-foreground">No documents attached.</p>
          ) : (
            <DataTable
              ariaLabel="Documents"
              data={docs}
              getRowId={(doc) => doc.id}
              columns={buildDocumentColumns()}
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
