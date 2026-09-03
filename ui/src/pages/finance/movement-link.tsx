import { useState } from 'react';
import { useLinkMovement } from '../../lib/api/hooks'; 
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
} from '@/components/ui/dialog';
import { useAssets, useAssetDocuments } from '../../lib/api/hooks';
import { Label } from '@/components/ui/label';

interface MovementLinkProps {
  movementId: string;
}

export function MovementLink({ movementId }: MovementLinkProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [selectedAssetId, setSelectedAssetId] = useState<string>('');
  const [selectedDocumentId, setSelectedDocumentId] = useState<string>('');
  const { data: assets } = useAssets();
  const { data: documents } = useAssetDocuments(selectedAssetId);
  const { mutate: linkDocument, isPending, error } = useLinkMovement(); 

  const handleLink = () => {
    if (selectedDocumentId) {
      linkDocument(
        { movementId, documentId: selectedDocumentId },
        {
          onSuccess: () => {
            setIsOpen(false);
          },
        }
      );
    }
  };

  const errorMessage = error instanceof Error && error.message.includes('409') 
    ? 'Document already linked' 
    : error?.message;

  return (
    <Dialog open={isOpen} onOpenChange={setIsOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">Link Document</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Link Document</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <Label htmlFor="asset-select">Asset</Label>
            <select
              id="asset-select"
              value={selectedAssetId}
              onChange={(e) => {
                setSelectedAssetId(e.target.value);
                setSelectedDocumentId('');
              }}
              className="w-full border rounded p-2"
            >
              <option value="">Select an asset</option>
              {assets?.map((asset) => (
                <option key={asset.id} value={asset.id}>
                  {asset.brand} {asset.model}
                </option>
              ))}
            </select>
          </div>
          {selectedAssetId && (
            <div>
              <Label htmlFor="document-select">Document</Label>
              <select
                id="document-select"
                value={selectedDocumentId}
                onChange={(e) => setSelectedDocumentId(e.target.value)}
                className="w-full border rounded p-2"
              >
                <option value="">Select a document</option>
                {documents?.map((doc) => (
                  <option key={doc.id} value={doc.id}>
                    {doc.source_filename} ({doc.doc_type})
                  </option>
                ))}
              </select>
            </div>
          )}
          {errorMessage && <p className="text-red-500">{errorMessage}</p>}
        </div>
        <DialogFooter>
          <Button onClick={handleLink} disabled={!selectedDocumentId || isPending}>
            {isPending ? 'Linking...' : 'Link'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
