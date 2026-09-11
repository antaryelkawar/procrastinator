import { useState } from 'react';
import { useNavigate, useParams, Link } from 'react-router';
import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { ConfirmDialog } from '@/features/docs/confirm-dialog';
import { DataTable, features } from '@/features/docs/data-table';
import { useBatches, useBatch, useCommitBatch, useDiscardBatch } from '@/features/finance/hooks';
import { ImportLinesTable } from './import-lines-table';
import { Button } from '@/components/ui/button';
import type { ImportBatch } from '@/lib/api/schema';

const columnHelper = createColumnHelper<typeof features, ImportBatch>();

function buildHistoryColumns(): ColumnDef<TableFeatures, ImportBatch, unknown>[] {
  return [
    columnHelper.accessor('filename', {
      header: 'Filename',
    }),
    columnHelper.accessor('account_id', {
      header: 'Account',
    }),
    columnHelper.accessor('state', {
      header: 'Status',
      cell: (context) => (
        <Badge variant={context.getValue() === 'preview' ? 'secondary' : 'default'}>
          {context.getValue()}
        </Badge>
      ),
    }),
    columnHelper.accessor('line_count_valid', {
      header: 'Counts',
      cell: (context) =>
        `V:${context.getValue()} E:${context.row.original.line_count_error} D:${context.row.original.line_count_duplicate}`,
    }),
    columnHelper.accessor('created_at', {
      header: 'Created',
      cell: (context) => new Date(context.getValue()).toLocaleString(),
    }),
  ] as ColumnDef<TableFeatures, ImportBatch, unknown>[];
}

export function ImportHistoryPage() {
  const { batchId } = useParams<{ batchId: string }>();
  const navigate = useNavigate();
  const [showCommit, setShowCommit] = useState(false);
  const [showDiscard, setShowDiscard] = useState(false);
  
  const { data: batches, isLoading, isError: isBatchesError } = useBatches();
  const { data: selectedBatch, isError: isBatchError, refetch: refetchBatch } = useBatch(batchId || '');
  const commitBatch = useCommitBatch();
  const discardBatch = useDiscardBatch();

  if (isLoading) return <div role="status">Loading...</div>;
  if (isBatchesError || !batches) return <ErrorState title="Error" message="Failed to load import history" />;

  if (!batchId) {
    if (batches.length === 0) {
      return (
        <div className="p-8 space-y-4">
          <h1 className="text-2xl font-bold">Import History</h1>
          <EmptyState title="No import history" description="You have not imported any statements yet." />
        </div>
      );
    }

    return (
      <div className="p-8 space-y-4">
        <h1 className="text-2xl font-bold">Import History</h1>
        <Card>
          <DataTable
            ariaLabel="Import history"
            data={batches}
            getRowId={(batch) => batch.id}
            columns={buildHistoryColumns()}
            onRowClick={(batch) => navigate(`/finance/import/${batch.id}`)}
          />
        </Card>
      </div>
    );
  }

  if (isBatchError) {
    return (
      <div className="p-8 space-y-4">
        <ErrorState 
          title="Error loading batch" 
          message="The requested batch could not be found or failed to load." 
          onRetry={refetchBatch}
        />
        <Button variant="outline" asChild>
          <Link to="/finance/import/history">← Back to history</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="p-8 space-y-4">
      <h1 className="text-2xl font-bold">Batch detail</h1>
      <Button variant="outline" asChild>
        <Link to="/finance/import/history">← Back to history</Link>
      </Button>
      {selectedBatch ? (
        <Card>
          <CardContent className="pt-6 space-y-4">
            <div className="flex justify-between items-center">
              <div>
                <h2 className="text-xl font-semibold">Batch: {selectedBatch.filename}</h2>
                <div className="text-sm text-muted-foreground">Account: {selectedBatch.account_id}</div>
              </div>
              <Badge variant={selectedBatch.state === 'preview' ? 'secondary' : 'default'}>
                {selectedBatch.state}
              </Badge>
            </div>
            
            {selectedBatch.state === 'preview' && (
              <div className="flex space-x-2">
                <Button variant="default" onClick={() => setShowCommit(true)}>Commit</Button>
                <Button variant="destructive" onClick={() => setShowDiscard(true)}>Discard</Button>
              </div>
            )}
            
            <ImportLinesTable batch={selectedBatch} />
            
            <ConfirmDialog 
                open={showCommit}
                onOpenChange={setShowCommit}
                title="Commit batch?"
                description="This will import all valid lines into the system."
                confirmLabel="Commit"
                onConfirm={() => commitBatch.mutate({ batchId: selectedBatch.id })}
                busy={commitBatch.isPending}
            />
            <ConfirmDialog 
                open={showDiscard}
                onOpenChange={setShowDiscard}
                title="Discard batch?"
                description="This will permanently discard this batch."
                confirmLabel="Discard"
                destructive
                onConfirm={() => discardBatch.mutate({ batchId: selectedBatch.id })}
                busy={discardBatch.isPending}
            />
          </CardContent>
        </Card>
      ) : <div role="status">Loading batch...</div>}
    </div>
  );
}
