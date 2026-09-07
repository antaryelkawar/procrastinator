/** @vitest-environment jsdom */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import axe from 'axe-core';
import { Upload } from './upload';

describe('Upload (shared dropzone)', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the dropzone with its accessible label', () => {
    render(<Upload onDrop={() => undefined} />);

    // the focusable dropzone root is labelled
    expect(screen.getByLabelText('file upload', { selector: 'div' })).toBeInTheDocument();
    // hidden file input is still in the DOM, labelled for a11y
    expect(screen.getByLabelText('file upload', { selector: 'input' })).toBeInTheDocument();
  });

  it('invokes onDrop with picked files', async () => {
    const onDrop = vi.fn();
    render(<Upload onDrop={onDrop} label="documents" />);

    const file = new File(['x'], 'a.pdf', { type: 'application/pdf' });
    const input = screen.getByLabelText('documents', { selector: 'input' });
    fireEvent.change(input, { target: { files: [file] } });

    await waitFor(() => expect(onDrop).toHaveBeenCalledWith([file]));
  });

  it('opens the file dialog on Enter and Space', () => {
    const clickSpy = vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(() => {});
    render(<Upload onDrop={() => undefined} label="documents" />);

    const root = screen.getByLabelText('documents', { selector: 'div' });
    fireEvent.keyDown(root, { key: 'Enter' });
    expect(clickSpy.mock.calls.length).toBeGreaterThanOrEqual(1);

    clickSpy.mockClear();
    fireEvent.keyDown(root, { key: ' ' });
    expect(clickSpy.mock.calls.length).toBeGreaterThanOrEqual(1);

    // other keys do not open the dialog
    clickSpy.mockClear();
    fireEvent.keyDown(root, { key: 'a' });
    expect(clickSpy).not.toHaveBeenCalled();
  });

  it('is axe-clean', async () => {
    const { container } = render(
      <Upload onDrop={() => undefined} hint="Drag & drop files here" />,
    );

    const results = await axe.run(container);
    expect(results.violations).toEqual([]);
  });
});
