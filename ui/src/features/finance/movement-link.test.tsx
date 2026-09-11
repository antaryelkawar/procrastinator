import { render, screen, fireEvent } from '@testing-library/react';
import { MovementLink } from './movement-link';
import * as hooks from '@/features/finance/hooks';
import * as docsHooks from '@/features/docs/hooks';
import { vi, test, expect, describe, beforeEach } from 'vitest';

vi.mock('@/features/finance/hooks', async () => {
  const actual = await vi.importActual<typeof hooks>('@/features/finance/hooks');
  return {
    ...actual,
    useLinkMovement: vi.fn(),
  };
});

vi.mock('@/features/docs/hooks', () => ({
  useAssets: vi.fn(),
  useAssetDocuments: vi.fn(),
}));

describe('MovementLink', () => {
  const mockAssets = [{ id: 'a1', brand: 'Brand', model: 'Model' }];
  const mockDocuments = [{ id: 'd1', source_filename: 'file.pdf', doc_type: 'invoice' }];
  const mockLinkDocument = vi.fn();

  beforeEach(() => {
    (docsHooks.useAssets as any).mockReturnValue({ data: mockAssets });
    (docsHooks.useAssetDocuments as any).mockReturnValue({ data: mockDocuments });
    (hooks.useLinkMovement as any).mockReturnValue({
      mutate: mockLinkDocument,
      isPending: false,
      error: null,
    });
  });

  test('opens dialog and links document', async () => {
    render(<MovementLink movementId="m1" />);
    fireEvent.click(screen.getByText('Link Document'));

    // Asset — shared Radix Select: open the trigger (click), pick option by text.
    fireEvent.click(screen.getByLabelText('Asset'));
    fireEvent.click(await screen.findByRole('option', { name: 'Brand Model' }));

    // Document appears once an asset is selected; open and pick it.
    const docTrigger = await screen.findByLabelText('Document');
    fireEvent.click(docTrigger);
    fireEvent.click(await screen.findByRole('option', { name: 'file.pdf (invoice)' }));

    fireEvent.click(screen.getByText('Link'));

    expect(mockLinkDocument).toHaveBeenCalledWith(
      { movementId: 'm1', documentId: 'd1' },
      expect.anything()
    );
  });
});
