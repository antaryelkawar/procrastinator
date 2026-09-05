/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider } from '@/context/active-user';
import { ReviewQueuePage } from './review-queue-page';
import * as hooks from '@/lib/api/hooks';
import * as sonner from 'sonner';
import type { IngestReview } from '@/lib/api/schema';

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
}));

vi.mock('@/lib/api/hooks', async () => {
  const actual = await vi.importActual('@/lib/api/hooks');
  return {
    ...actual,
    useReviews: vi.fn(),
    useApproveReview: vi.fn(),
    useRejectReview: vi.fn(),
  };
});

const mockUseReviews = vi.mocked(hooks.useReviews);
const mockUseApproveReview = vi.mocked(hooks.useApproveReview);
const mockUseRejectReview = vi.mocked(hooks.useRejectReview);

const mockReviews: IngestReview[] = [
  {
    id: 'r1',
    source_filename: 'invoice.pdf',
    doc_type: 'invoice',
    confidence: 0.45,
    candidate_fields: { type: 'invoice', merchant: 'Coffee Shop', amount: 12.50 },
    raw_extraction: { type: 'invoice' },
    state: 'pending',
    source_uploaded_at: '2024-01-15T10:00:00Z',
    best_matched_asset_id: 'a1',
    best_matched_asset_title: 'Coffee Machine',
    created_at: '2024-01-15T10:00:00Z',
    decided_at: null,
  },
  {
    id: 'r2',
    source_filename: 'receipt.pdf',
    doc_type: 'receipt',
    confidence: null,
    candidate_fields: { type: 'receipt', merchant: 'Grocery Store', amount: 45.00 },
    raw_extraction: { type: 'receipt' },
    state: 'pending',
    source_uploaded_at: '2024-01-14T10:00:00Z',
    best_matched_asset_id: null,
    best_matched_asset_title: null,
    created_at: '2024-01-14T10:00:00Z',
    decided_at: null,
  },
];

function renderWithProviders() {
  const queryClient = new QueryClient();
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <ActiveUserProvider queryClient={queryClient}>
          <ReviewQueuePage />
        </ActiveUserProvider>
      </QueryClientProvider>
    </MemoryRouter>
  );
}

describe('ReviewQueuePage', () => {
  let mockApprove: ReturnType<typeof vi.fn>;
  let mockReject: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.setItem('activeUser', 'test-user');
    
    mockApprove = vi.fn();
    mockReject = vi.fn();
    
    mockUseApproveReview.mockReturnValue({
      mutate: mockApprove,
      mutateAsync: mockApprove,
    } as any);
    
    mockUseRejectReview.mockReturnValue({
      mutate: mockReject,
      mutateAsync: mockReject,
    } as any);
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('shows loading state initially (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.getByText(/loading reviews/i)).toBeInTheDocument();
  });

  it('shows error state when query fails (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    expect(screen.getByText(/failed to load reviews/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument();
  });

  it('shows "No pending reviews" when status filter is empty (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('No pending reviews')).toBeInTheDocument();
  });

  it('shows reviews list when data is available (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: mockReviews,
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    
    expect(screen.getByText('invoice.pdf')).toBeInTheDocument();
    expect(screen.getByText('receipt.pdf')).toBeInTheDocument();
    expect(screen.getByText('Coffee Machine')).toBeInTheDocument();
  });

  it('shows em dash when confidence is null (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[1]], // receipt with null confidence
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('shows confidence percentage when present (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]], // invoice with 0.45 confidence
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('45%')).toBeInTheDocument();
  });

  it('shows "no match" when best_matched_asset_title is null (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[1]], // receipt with no match
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText(/no match/i)).toBeInTheDocument();
  });

  it('shows best matched asset title when present (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]], // invoice with Coffee Machine match
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('Coffee Machine')).toBeInTheDocument();
  });

  it('shows status badges (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: mockReviews,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    expect(screen.getAllByText('pending').length).toBeGreaterThan(0);
  });

  it('shows approve button for pending reviews (F-12)', () => {
    mockUseReviews.mockReturnValue({
      data: mockReviews,
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    const approveButtons = screen.getAllByRole('button', { name: /approve/i });
    expect(approveButtons.length).toBe(2);
  });

  it('shows reject button for pending reviews (F-12)', () => {
    mockUseReviews.mockReturnValue({
      data: mockReviews,
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    const rejectButtons = screen.getAllByRole('button', { name: /reject/i });
    expect(rejectButtons.length).toBe(2);
  });

  it('does not show approve/reject buttons for non-pending reviews (F-12)', () => {
    const approvedReview: IngestReview = {
      ...mockReviews[0],
      state: 'approved',
      decided_at: '2024-01-16T10:00:00Z',
    };
    
    mockUseReviews.mockReturnValue({
      data: [approvedReview],
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.queryByRole('button', { name: /approve/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /reject/i })).not.toBeInTheDocument();
  });

  it('opens confirmation dialog when approve is clicked (F-12)', async () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const approveButton = screen.getAllByRole('button', { name: /approve/i })[0];
    fireEvent.click(approveButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });
  });

  it('calls approve mutation when confirm is clicked in dialog (F-12)', async () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const approveButton = screen.getAllByRole('button', { name: /approve/i })[0];
    fireEvent.click(approveButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    const confirmButton = screen.getByRole('button', { name: /^approve$/i });
    fireEvent.click(confirmButton);

    expect(mockApprove).toHaveBeenCalledWith({ reviewId: 'r1' });
  });

  it('opens confirmation dialog when reject is clicked (F-12)', async () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const rejectButton = screen.getAllByRole('button', { name: /reject/i })[0];
    fireEvent.click(rejectButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });
  });

  it('calls reject mutation when confirm is clicked in dialog (F-12)', async () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const rejectButton = screen.getAllByRole('button', { name: /reject/i })[0];
    fireEvent.click(rejectButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    const confirmButton = screen.getByRole('button', { name: /^reject$/i });
    fireEvent.click(confirmButton);

    expect(mockReject).toHaveBeenCalledWith({ reviewId: 'r1' });
  });

  it('does not call mutation when cancel is clicked in dialog (F-12)', async () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const approveButton = screen.getAllByRole('button', { name: /approve/i })[0];
    fireEvent.click(approveButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    });

    expect(mockApprove).not.toHaveBeenCalled();
  });

  it('handles 409 conflict error on approve (F-12)', async () => {
    const error409 = new Error('Review already processed') as any;
    error409.status = 409;
    mockApprove.mockRejectedValue(error409);

    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const approveButton = screen.getAllByRole('button', { name: /approve/i })[0];
    fireEvent.click(approveButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    const confirmButton = screen.getByRole('button', { name: /^approve$/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(sonner.toast.error).toHaveBeenCalledWith('Review already processed', expect.any(Object));
    });

    // Dialog should close after error
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    });
  });

  it('handles 404 not found error on reject (F-12)', async () => {
    const error404 = new Error('Review not found') as any;
    error404.status = 404;
    mockReject.mockRejectedValue(error404);

    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const rejectButton = screen.getAllByRole('button', { name: /reject/i })[0];
    fireEvent.click(rejectButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    const confirmButton = screen.getByRole('button', { name: /^reject$/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(sonner.toast.error).toHaveBeenCalledWith('Review not found', expect.any(Object));
    });

    // Dialog should close after error
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    });
  });

  it('refetches reviews after successful approve (F-12)', async () => {
    const mockRefetch = vi.fn();
    mockApprove.mockResolvedValue({
      asset: { id: 'a1', brand: 'Coffee Machine' },
      review: { id: 'r1', state: 'approved' },
    });

    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as any);

    renderWithProviders();
    const approveButton = screen.getAllByRole('button', { name: /approve/i })[0];
    fireEvent.click(approveButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    const confirmButton = screen.getByRole('button', { name: /^approve$/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(mockRefetch).toHaveBeenCalled();
    });
  });

  it('refetches reviews after successful reject (F-12)', async () => {
    const mockRefetch = vi.fn();
    mockReject.mockResolvedValue({
      id: 'r1',
      state: 'rejected',
    });

    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    } as any);

    renderWithProviders();
    const rejectButton = screen.getAllByRole('button', { name: /reject/i })[0];
    fireEvent.click(rejectButton);

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    const confirmButton = screen.getByRole('button', { name: /^reject$/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(mockRefetch).toHaveBeenCalled();
    });
  });

  it('has correct aria-label on approve/reject buttons (F-12)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    const approveButton = screen.getByRole('button', { name: /approve invoice.pdf/i });
    const rejectButton = screen.getByRole('button', { name: /reject invoice.pdf/i });
    
    expect(approveButton).toBeInTheDocument();
    expect(rejectButton).toBeInTheDocument();
  });

  it('confirmation dialog has correct role="alertdialog" (F-12)', async () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    } as any);

    renderWithProviders();
    const approveButton = screen.getAllByRole('button', { name: /approve/i })[0];
    fireEvent.click(approveButton);

    await waitFor(() => {
      const dialog = screen.getByRole('alertdialog');
      expect(dialog).toBeInTheDocument();
    });
  });

  it('shows source filename (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('invoice.pdf')).toBeInTheDocument();
  });

  it('shows doc_type (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    expect(screen.getByText('invoice')).toBeInTheDocument();
  });

  it('shows created_at timestamp (F-11)', () => {
    mockUseReviews.mockReturnValue({
      data: [mockReviews[0]],
      isLoading: false,
      isError: false,
    } as any);

    renderWithProviders();
    // The date formatter shows YYYY-MM-DD HH:mm UTC format
    expect(screen.getByText(/2024-01-15/)).toBeInTheDocument();
  });
});
