import { useState } from 'react';
import { Card, CardContent } from '../../components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { Badge } from '../../components/ui/badge';
import { EmptyState } from '../../components/feedback/empty-state';
import { ConfirmDialog } from '../../components/confirm-dialog';
import { useBatches, useBatch, useCommitBatch, useDiscardBatch } from '../../lib/api/hooks';
import { ImportLinesTable } from './import-lines-table';
import { Button } from '../../components/ui/button';

export function ImportHistoryPage() {
  const [selectedBatchId, setSelectedBatchId] = useState<string | null>(null);
  const [showCommit, setShowCommit] = useState(false);
  const [showDiscard, setShowDiscard] = useState(false);
  
  const { data: batches, isLoading, isError } = useBatches();
  const { data: selectedBatch } = useBatch(selectedBatchId || '');
  const commitBatch = useCommitBatch();
  const discardBatch = useDiscardBatch();

  if (isLoading) return <div role="status">Loading...</div>;
  if (isError || !batches) return <div>Error loading history</div>;

  if (batches.length === 0) {
    return (
      <div className="p-8">
        <EmptyState title="No import history" description="You have not imported any statements yet." />
      </div>
    );
  }

  return (
    <div className="p-8 space-y-4">
      <h1 className="text-2xl font-bold">Import History</h1>
      
      {!selectedBatchId ? (
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
                  onClick={() => setSelectedBatchId(batch.id)}
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
      ) : (
        <div className="space-y-4">
          <Button variant="outline" onClick={() => setSelectedBatchId(null)}>← Back to list</Button>
          {selectedBatch ? (
            <Card>
              <CardContent className="pt-6">
                <div className="flex justify-between items-center mb-4">
                  <h2 className="text-xl font-semibold">Batch: {selectedBatch.filename}</h2>
                  {selectedBatch.state === 'preview' && (
                    <div className="space-x-2">
                      <Button variant="default" onClick={() => setShowCommit(true)}>Commit</Button>
                      <Button variant="destructive" onClick={() => setShowDiscard(true)}>Discard</Button>
                    </div>
                  )}
                </div>
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
      )}
    </div>
  );
}
