import { useNavigate } from 'react-router-dom';
import { useAssets } from '@/lib/api/hooks';
import { formatDate, isWarrantyExpired } from '@/lib/format/date';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/feedback/empty-state';
import { Loading } from '@/components/feedback/loading';
import { ErrorState } from '@/components/feedback/error-state';

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
          description="Upload your first document to get started."
        >
          <Button onClick={() => navigate('/upload')}>Upload document</Button>
        </EmptyState>
      ) : (
        <>
          {/* Desktop/Tablet Table */}
          <div className="hidden md:block">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Brand</TableHead>
                  <TableHead>Model</TableHead>
                  <TableHead>Serial Number</TableHead>
                  <TableHead>Warranty End</TableHead>
                  <TableHead>Type</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {assets.map((asset) => (
                  <TableRow 
                    key={asset.id} 
                    onClick={() => navigate(`/assets/${asset.id}`)}
                    className="cursor-pointer"
                  >
                    <TableCell>{asset.brand ?? '-'}</TableCell>
                    <TableCell>{asset.model ?? '-'}</TableCell>
                    <TableCell>{asset.serial_number ?? '-'}</TableCell>
                    <TableCell className={asset.warranty_end && isWarrantyExpired(asset.warranty_end) ? 'text-red-600 font-medium' : ''}>
                      {asset.warranty_end ? formatDate(asset.warranty_end) : '-'}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{asset.doc_type}</Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {/* Mobile Card List */}
          <div className="md:hidden space-y-4">
            {assets.map((asset) => (
              <Card 
                key={asset.id} 
                className="p-4 cursor-pointer hover:bg-muted/50"
                onClick={() => navigate(`/assets/${asset.id}`)}
              >
                <div className="flex justify-between items-start">
                  <div className="space-y-1">
                    <p className="font-semibold">{asset.brand ?? 'No Brand'} {asset.model}</p>
                    <p className="text-sm text-muted-foreground">SN: {asset.serial_number ?? '-'}</p>
                  </div>
                  <Badge variant="outline">{asset.doc_type}</Badge>
                </div>
                <div className="mt-4 text-sm">
                  <span className="text-muted-foreground">Warranty: </span>
                  <span className={asset.warranty_end && isWarrantyExpired(asset.warranty_end) ? 'text-red-600 font-medium' : ''}>
                    {asset.warranty_end ? formatDate(asset.warranty_end) : 'N/A'}
                  </span>
                </div>
              </Card>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
