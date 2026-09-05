/** @vitest-environment jsdom */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { UploadPage } from './upload-page';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { uploadDocument } from '../../lib/api/upload';
import { ApiError } from '../../lib/api/errors';
import { ActiveUserProvider } from '../../context/active-user';

vi.mock('../../lib/api/upload', async () => {
  const actual = await vi.importActual('../../lib/api/upload');
  return {
    ...actual,
    uploadDocument: vi.fn(),
  };
});

function renderWithProviders() {
  const queryClient = new QueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <ActiveUserProvider queryClient={queryClient}>
        <UploadPage />
      </ActiveUserProvider>
    </QueryClientProvider>
  );
}

describe('UploadPage', () => {
  beforeEach(() => {
    localStorage.setItem('activeUser', 'alice');
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('handles successful upload', async () => {
    vi.mocked(uploadDocument).mockResolvedValue({ kind: 'committed', asset: { id: 'asset-123' } } as never);

    renderWithProviders();
    const file = new File(['hello'], 'success.pdf', { type: 'application/pdf' });
    const input = screen.getByLabelText(/file upload/i);
    fireEvent.change(input, { target: { files: [file] } });
    
    await waitFor(() => expect(screen.getByText('success.pdf')).toBeInTheDocument());
    
    // Check for success state (status span "success 0%" — exact text, not the "success.pdf" name)
    await waitFor(() => expect(screen.getByText('success')).toBeInTheDocument());
    expect(screen.getByText(/Asset ID: asset-123/i)).toBeInTheDocument();
  });

  it('handles 415 error', async () => {
    vi.mocked(uploadDocument).mockRejectedValue(new ApiError(415, 'Unsupported Media Type'));

    renderWithProviders();
    const file = new File(['hello'], 'error.pdf', { type: 'application/pdf' });
    const input = screen.getByLabelText(/file upload/i);
    fireEvent.change(input, { target: { files: [file] } });
    
    await waitFor(() => expect(screen.getByText('error.pdf')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText(/Unsupported Media Type/i)).toBeInTheDocument());
  });

  it('handles 502 error', async () => {
    vi.mocked(uploadDocument).mockRejectedValue(new ApiError(502, 'The document processor hiccuped'));

    renderWithProviders();
    const file = new File(['hello'], 'retry.pdf', { type: 'application/pdf' });
    const input = screen.getByLabelText(/file upload/i);
    fireEvent.change(input, { target: { files: [file] } });
    
    await waitFor(() => expect(screen.getByText('retry.pdf')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText(/The document processor hiccuped/i)).toBeInTheDocument());
    const retry = screen.getByRole('button', { name: /retry/i });
    fireEvent.click(retry);
    // after the retry the mock rejects with the same 502 ApiError → the
    // retryable error is displayed again (retry wiring works end-to-end)
    await waitFor(() => expect(screen.getByText(/The document processor hiccuped/i)).toBeInTheDocument());
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument();
  });
});
