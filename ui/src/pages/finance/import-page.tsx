import React, { useState } from 'react';
import { useAccounts, useUploadStatement, useCommitBatch, useDiscardBatch } from '../../lib/api/hooks';
import { ImportLinesTable } from './import-lines-table';
import { Button } from '../../components/ui/button';
import { ConfirmDialog } from '../../components/confirm-dialog';
import { Progress } from '../../components/ui/progress';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/card';
import { AlertCircle } from 'lucide-react';

export function ImportPage() {
  const { data: accounts, isLoading: accountsLoading } = useAccounts();
  const [selectedAccountId, setSelectedAccountId] = useState<string>('');
  const [file, setFile] = useState<File | null>(null);
  const [progress, setProgress] = useState(0);
  const [uploadError, setUploadError] = useState<string | null>(null);
  
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [discardDialogOpen, setDiscardDialogOpen] = useState(false);

  const uploadMutation = useUploadStatement();
  const commitMutation = useCommitBatch();
  const discardMutation = useDiscardBatch();

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      setFile(e.target.files[0]);
      setUploadError(null);
    }
  };

  const handleUpload = async () => {
    if (!selectedAccountId || !file) return;
    
    setUploadError(null);
    setProgress(0);
    try {
      await uploadMutation.mutateAsync({
        accountId: selectedAccountId,
        file,
        onProgress: (ev) => setProgress(Math.round((ev.loaded / ev.total) * 100)),
      });
    } catch (err: any) {
      setUploadError(err.detail || err.message || 'Upload failed');
    }
  };

  if (accountsLoading) return <div>Loading...</div>;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Import Statement</h1>
      
      <div className="space-y-4">
        <div>
          <label htmlFor="account-select" className="block text-sm font-medium">Account</label>
          <select
            id="account-select"
            className="w-full p-2 border rounded"
            value={selectedAccountId}
            onChange={(e) => setSelectedAccountId(e.target.value)}
          >
            <option value="">Select an account</option>
            {accounts?.map((acc) => (
              <option key={acc.id} value={acc.id}>{acc.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label htmlFor="file-upload" className="block text-sm font-medium">CSV/PDF File</label>
          <input
            id="file-upload"
            type="file"
            accept=".csv,.pdf"
            onChange={handleFileChange}
            className="w-full p-2 border rounded"
          />
        </div>

        <Button onClick={handleUpload} disabled={!selectedAccountId || !file || uploadMutation.isPending}>
          {uploadMutation.isPending ? 'Uploading...' : 'Upload Statement'}
        </Button>

        {uploadMutation.isPending && (
          <Progress value={progress} className="w-full" />
        )}
      </div>

      {uploadError && (
        <div className="p-4 bg-destructive/10 text-destructive rounded flex items-center gap-2">
          <AlertCircle size={16} />
          {uploadError}
        </div>
      )}

      {uploadMutation.data && (
        <Card>
          <CardHeader>
            <CardTitle>Preview: {uploadMutation.data.filename}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex gap-4 mb-4">
              <span className="text-sm">Valid: {uploadMutation.data.line_count_valid}</span>
              <span className="text-sm">Duplicates: {uploadMutation.data.line_count_duplicate}</span>
              <span className="text-sm">Errors: {uploadMutation.data.line_count_error}</span>
            </div>
            
            <ImportLinesTable batch={uploadMutation.data} />
            
            <div className="flex gap-2 mt-4">
              <Button onClick={() => setCommitDialogOpen(true)}>Commit</Button>
              <Button variant="destructive" onClick={() => setDiscardDialogOpen(true)}>Discard</Button>
            </div>

            <ConfirmDialog
              open={commitDialogOpen}
              onOpenChange={setCommitDialogOpen}
              title="Commit Import"
              description="This will create the movements. Continue?"
              busy={commitMutation.isPending}
              onConfirm={() => commitMutation.mutate({ batchId: uploadMutation.data!.id })}
            />

            <ConfirmDialog
              open={discardDialogOpen}
              onOpenChange={setDiscardDialogOpen}
              title="Discard Import"
              description="This will delete the previewed import. Continue?"
              destructive
              busy={discardMutation.isPending}
              onConfirm={() => discardMutation.mutate({ batchId: uploadMutation.data!.id })}
            />
          </CardContent>
        </Card>
      )}

      {commitMutation.isSuccess && (
        <div className="p-4 bg-green-100 text-green-800 rounded">
          Import committed! Created: {commitMutation.data.created}, Skipped: {commitMutation.data.skipped}.
        </div>
      )}
      
      {discardMutation.isSuccess && (
        <div className="p-4 bg-yellow-100 text-yellow-800 rounded">
          Import discarded.
        </div>
      )}
    </div>
  );
}
