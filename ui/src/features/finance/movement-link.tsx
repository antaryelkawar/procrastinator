import { useState } from 'react';
import { useLinkMovement } from '@/features/finance/hooks';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
} from '@/components/ui/dialog';
import { useAssets, useAssetDocuments } from '@/features/docs/hooks';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

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
            <Select
              value={selectedAssetId}
              onValueChange={(value) => {
                setSelectedAssetId(value);
                setSelectedDocumentId('');
              }}
            >
              <SelectTrigger className="w-full" aria-label="Asset">
                <SelectValue placeholder="Select an asset" />
              </SelectTrigger>
              <SelectContent>
                {assets?.map((asset) => (
                  <SelectItem key={asset.id} value={asset.id}>
                    {asset.brand} {asset.model}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {selectedAssetId && (
            <div>
              <Label htmlFor="document-select">Document</Label>
              <Select
                value={selectedDocumentId}
                onValueChange={setSelectedDocumentId}
              >
                <SelectTrigger id="document-select" className="w-full" aria-label="Document">
                  <SelectValue placeholder="Select a document" />
                </SelectTrigger>
                <SelectContent>
                  {documents?.map((doc) => (
                    <SelectItem key={doc.id} value={doc.id}>
                      {doc.source_filename} ({doc.doc_type})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
