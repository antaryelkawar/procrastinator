import { useParams } from 'react-router-dom';
import { useAsset, useAssetDocuments } from '@/lib/api/hooks';
import { formatDate } from '@/lib/format/date';
import { formatMoney } from '@/lib/format/money';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loading } from '@/components/feedback/loading';
import { ErrorState } from '@/components/feedback/error-state';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';

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
        <Badge variant="outline">{asset.doc_type}</Badge>
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
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Filename</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Uploaded</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {docs.map((doc) => (
                  <TableRow key={doc.id}>
                    <TableCell>{doc.source_filename}</TableCell>
                    <TableCell>{doc.doc_type}</TableCell>
                    <TableCell>{formatDate(doc.source_uploaded_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
