import { useState } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import { Card, CardContent } from '../../components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { Badge } from '../../components/ui/badge';
import { EmptyState } from '../../components/feedback/empty-state';
import { ErrorState } from '../../components/feedback/error-state';
import { ConfirmDialog } from '../../components/confirm-dialog';
import { useBatches, useBatch, useCommitBatch, useDiscardBatch } from '../../lib/api/hooks';
import { ImportLinesTable } from './import-lines-table';
import { Button } from '../../components/ui/button';

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
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead id="filename-col">Filename</TableHead>
                <TableHead id="account-col">Account</TableHead>
                <TableHead id="status-col">Status</TableHead>
                <TableHead id="counts-col">Counts</TableHead>
                <TableHead id="created-col">Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {batches.map((batch) => (
                <TableRow 
                  key={batch.id} 
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => navigate(`/finance/import/${batch.id}`)}
                  role="button"
                >
                  <TableCell>{batch.filename}</TableCell>
                  <TableCell>{batch.account_id}</TableCell>
                  <TableCell>
                    <Badge variant={batch.state === 'preview' ? 'secondary' : 'default'}>
                      {batch.state}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    V:{batch.line_count_valid} E:{batch.line_count_error} D:{batch.line_count_duplicate}
                  </TableCell>
                  <TableCell>{new Date(batch.created_at).toLocaleString()}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
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
