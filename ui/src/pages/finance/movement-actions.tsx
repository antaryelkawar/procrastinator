import { useState } from 'react';
import { Edit2, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { usePatchDescription, useDeleteMovement } from '../../lib/api/hooks';
import type { Movement } from '../../lib/api/schema';

interface MovementActionsProps {
  readonly movement: Movement;
}

export function MovementActions({ movement }: MovementActionsProps) {
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
  const [description, setDescription] = useState(movement.description);
  const [error, setError] = useState<string | null>(null);

  const patchMutation = usePatchDescription();
  const deleteMutation = useDeleteMovement();

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = description.trim();
    if (!trimmed) {
      setError('Description cannot be blank');
      return;
    }
    setError(null);
    try {
      await patchMutation.mutateAsync({
        movementId: movement.id,
        description: trimmed,
      });
      setIsEditDialogOpen(false);
    } catch (err) {
      // Error handling is usually global or handled by the hook/toast
    }
  };

  const handleDeleteConfirm = async () => {
    try {
      await deleteMutation.mutateAsync({ movementId: movement.id });
      setIsDeleteDialogOpen(false);
    } catch (err) {
      // Handle error
    }
  };

  const isManual = movement.origin === 'manual';

  return (
    <div className="flex items-center gap-2">
      <Dialog open={isEditDialogOpen} onOpenChange={(open) => {
        setIsEditDialogOpen(open);
        if (open) {
          setDescription(movement.description);
          setError(null);
        }
      }}>
        <DialogTrigger asChild>
          <Button variant="ghost" size="icon-sm" title="Edit description">
            <Edit2 className="h-4 w-4" />
            <span className="sr-only">Edit</span>
          </Button>
        </DialogTrigger>
        <DialogContent>
          <form onSubmit={handleEditSubmit}>
            <DialogHeader>
              <DialogTitle>Edit description</DialogTitle>
            </DialogHeader>
            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor={`edit-description-${movement.id}`}>Description field</Label>
                <Input
                  id={`edit-description-${movement.id}`}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  className={error ? 'border-destructive' : ''}
                  autoFocus
                />
                {error && <p className="text-xs text-destructive">{error}</p>}
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setIsEditDialogOpen(false)}
                disabled={patchMutation.isPending}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={patchMutation.isPending}>
                {patchMutation.isPending ? 'Saving...' : 'Save'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {isManual && (
        <>
          <Button
            variant="ghost"
            size="icon-sm"
            className="text-destructive hover:text-destructive"
            onClick={() => setIsDeleteDialogOpen(true)}
            title="Delete movement"
          >
            <Trash2 className="h-4 w-4" />
            <span className="sr-only">Delete</span>
          </Button>
          <ConfirmDialog
            open={isDeleteDialogOpen}
            onOpenChange={setIsDeleteDialogOpen}
            title="Delete movement"
            description="Are you sure you want to delete this movement? This action cannot be undone."
            confirmLabel="Delete"
            destructive
            busy={deleteMutation.isPending}
            onConfirm={handleDeleteConfirm}
          />
        </>
      )}
    </div>
  );
}
