/**
 * Task 14.3 — ReviewChips (design D11 / `specs/intuitive-input`): the
 * progressive-disclosure review surface shown after extraction. Found fields
 * render as confirmable chips; unextracted optional fields stay collapsed
 * behind a single "Add details" affordance; a Save button commits the
 * values (edited or unedited). The component is presentational and
 * provider-free so it renders with no providers.
 */
/** @vitest-environment jsdom */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import axe from 'axe-core';
import { ReviewChips, toReviewChips } from './review-chips';
import type { ReviewChipsProps, ReviewChipField } from './review-chips';

const FIELDS: ReviewChipField[] = [
  { key: 'brand', label: 'Brand', value: 'Apple', confidence: 0.95 },
  { key: 'model', label: 'Model', value: 'MacBook Air' },
  { key: 'serial_number', label: 'Serial number', value: 'C02XYZ1234' },
  { key: 'price', label: 'Price', value: '39999.99', confidence: 0.82 },
];

const OPTIONAL: ReviewChipField[] = [
  { key: 'warranty_end', label: 'Warranty end', value: '' },
  { key: 'purchase_date', label: 'Purchase date', value: '' },
];

function renderChips(props: Partial<ReviewChipsProps> = {}) {
  return render(
    <ReviewChips
      fields={FIELDS}
      optionalFields={OPTIONAL}
      onConfirm={vi.fn()}
      onCancel={vi.fn()}
      {...props}
    />,
  );
}

describe('ReviewChips — found-field chips', () => {
  it('renders each found field as a chip showing its label and value', () => {
    renderChips();
    expect(screen.getByText(/brand/i)).toBeInTheDocument();
    expect(screen.getByText('Apple')).toBeInTheDocument();
    expect(screen.getByText('MacBook Air')).toBeInTheDocument();
    expect(screen.getByText(/price/i)).toBeInTheDocument();
    expect(screen.getByText('39999.99')).toBeInTheDocument();
  });

  it('shows a confidence percentage when confidence is present, and never 0%', () => {
    renderChips();
    expect(screen.getByText('95%')).toBeInTheDocument();
    expect(screen.getByText('82%')).toBeInTheDocument();
  });

  it('shows no confidence indicator when confidence is null/absent', () => {
    renderChips({ fields: [{ key: 'model', label: 'Model', value: 'MacBook Air' }] });
    expect(screen.getByText('MacBook Air')).toBeInTheDocument();
    // No percentage text anywhere.
    expect(screen.queryByText(/%$/)).toBeNull();
  });
});

describe('ReviewChips — tap-to-reveal + correct', () => {
  it('clicking a chip reveals an editable input pre-filled with the value', async () => {
    renderChips();
    const chip = screen.getByRole('button', { name: /edit brand/i });
    await fireEvent.click(chip);
    const input = screen.getByLabelText('Brand') as HTMLInputElement;
    expect(input).toBeInTheDocument();
    expect(input.value).toBe('Apple');
  });

  it('editing the revealed input and confirming changes the chip value AND the value passed to onConfirm', async () => {
    const onConfirm = vi.fn();
    renderChips({ onConfirm });

    const chip = screen.getByRole('button', { name: /edit brand/i });
    await fireEvent.click(chip);
    const input = screen.getByLabelText('Brand') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'Apple Vision Pro' } });
    const done = screen.getByRole('button', { name: 'Done' });
    await fireEvent.click(done);

    // Chip now shows the corrected value.
    expect(screen.getByText('Apple Vision Pro')).toBeInTheDocument();
    expect(screen.queryByText('Apple')).not.toBeInTheDocument();

    // Save passes the corrected value.
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onConfirm).toHaveBeenCalledWith(
      expect.objectContaining({ brand: 'Apple Vision Pro' }),
    );
  });

  it('uses aria-expanded / aria-controls on the chip and editor region', async () => {
    renderChips();
    const chip = screen.getByRole('button', { name: /brand/i });
    expect(chip).toHaveAttribute('aria-expanded', 'false');
    const controlsId = chip.getAttribute('aria-controls');
    await fireEvent.click(chip);
    expect(chip).toHaveAttribute('aria-expanded', 'true');
    const editor = document.getElementById(controlsId!);
    expect(editor).toBeInTheDocument();
  });
});

describe('ReviewChips — save (unedited) path', () => {
  it('Save with no edits calls onConfirm with exactly the extracted values', () => {
    const onConfirm = vi.fn();
    renderChips({ onConfirm });

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onConfirm).toHaveBeenCalledWith({
      brand: 'Apple',
      model: 'MacBook Air',
      serial_number: 'C02XYZ1234',
      price: '39999.99',
    });
  });

  it('Save with no optionalFields provided still commits all found fields', () => {
    const onConfirm = vi.fn();
    render(
      <ReviewChips
        fields={[
          { key: 'brand', label: 'Brand', value: 'Sony' },
          { key: 'price', label: 'Price', value: '199.99' },
        ]}
        onConfirm={onConfirm}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onConfirm).toHaveBeenCalledWith({ brand: 'Sony', price: '199.99' });
  });
});

describe('ReviewChips — add details disclosure', () => {
  it('collapsed: renders exactly one "Add details" affordance and NO optional inputs', () => {
    renderChips();
    const addDetails = screen.getByRole('button', { name: /add details/i });
    expect(addDetails).toBeInTheDocument();
    // Collapse affordance must be collapsed by default.
    expect(addDetails).toHaveAttribute('aria-expanded', 'false');
    // No optional-field inputs rendered (no empty field grid).
    expect(screen.queryByLabelText(/warranty/i)).toBeNull();
    expect(screen.queryByLabelText(/purchase date/i)).toBeNull();
  });

  it('expanded: clicking "Add details" reveals the optional-field inputs', async () => {
    renderChips();
    const addDetails = screen.getByRole('button', { name: /add details/i });
    await fireEvent.click(addDetails);
    expect(screen.getByLabelText(/warranty end/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/purchase date/i)).toBeInTheDocument();
  });

  it('typing into an optional field and saving includes that value in onConfirm', async () => {
    const onConfirm = vi.fn();
    renderChips({ onConfirm });

    await fireEvent.click(screen.getByRole('button', { name: /add details/i }));
    const warranty = screen.getByLabelText(/warranty end/i) as HTMLInputElement;
    await fireEvent.change(warranty, { target: { value: '2027-12-31' } });

    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onConfirm).toHaveBeenCalledWith(
      expect.objectContaining({ warranty_end: '2027-12-31' }),
    );
  });

  it('empty optional field is omitted from the onConfirm map', async () => {
    const onConfirm = vi.fn();
    renderChips({ onConfirm });

    // Expand but leave all optional fields empty.
    await fireEvent.click(screen.getByRole('button', { name: /add details/i }));

    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onConfirm).not.toHaveBeenCalledWith(expect.objectContaining({ warranty_end: undefined }));
    // warranty_end key must be absent from the map entirely.
    expect(onConfirm).toHaveBeenCalledTimes(1);
    const map = onConfirm.mock.calls[0][0] as Record<string, string>;
    expect(map).not.toHaveProperty('warranty_end');
    expect(map).toHaveProperty('brand', 'Apple');
  });

  it('collapses again when clicking "Add details" a second time', async () => {
    renderChips();
    const addDetails = screen.getByRole('button', { name: /add details/i });
    await fireEvent.click(addDetails);
    expect(screen.getByLabelText(/warranty end/i)).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: /add details/i }));
    expect(screen.queryByLabelText(/warranty end/i)).toBeNull();
  });
});

describe('ReviewChips — cancel', () => {
  it('Cancel is rendered only when onCancel is provided, and calls it', () => {
    const onCancel = vi.fn();
    renderChips({ onCancel });
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('Cancel is not rendered when onCancel is not provided', () => {
    renderChips({ onCancel: undefined });
    expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull();
  });
});

describe('ReviewChips — busy', () => {
  it('disables Save and sets aria-busy when busy', () => {
    renderChips({ busy: true });
    const save = screen.getByRole('button', { name: /saving/i });
    expect(save).toBeDisabled();
    expect(save).toHaveAttribute('aria-busy', 'true');
  });
});

describe('ReviewChips — title', () => {
  it('defaults to "Review before you save" and honors a custom title', () => {
    renderChips();
    expect(screen.getByText('Review before you save')).toBeInTheDocument();

    render(
      <ReviewChips fields={FIELDS} onConfirm={vi.fn()} title="Check your details" />,
    );
    expect(screen.getByText('Check your details')).toBeInTheDocument();
  });
});

describe('ReviewChips — toReviewChips helper', () => {
  it('stringifies scalar values, skips null/undefined/empty, and prettifies keys', () => {
    const result = toReviewChips({
      brand: 'Apple',
      model: 42,
      serial_number: '   ',
      empty: '',
      missing: null,
      absent: undefined,
      price: 39999.99,
    });
    const byKey = Object.fromEntries(result.map((f) => [f.key, f.value]));
    expect(byKey).toEqual({
      brand: 'Apple',
      model: '42',
      price: '39999.99',
    });
    expect(result.find((f) => f.key === 'serial_number')).toBeUndefined();
    expect(result.find((f) => f.key === 'missing')).toBeUndefined();
    expect(result.find((f) => f.key === 'absent')).toBeUndefined();
  });

  it('applies a passed-through confidence to every field', () => {
    const result = toReviewChips({ brand: 'Apple' }, 0.88);
    expect(result).toEqual([{ key: 'brand', label: 'Brand', value: 'Apple', confidence: 0.88 }]);
  });
});

describe('ReviewChips — accessibility', () => {
  it('passes axe (no violations) in the collapsed state', async () => {
    const { container } = renderChips();
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });

  it('passes axe (no violations) in the expanded state', async () => {
    const { container } = renderChips();
    await fireEvent.click(screen.getByRole('button', { name: /add details/i }));
    const results = await axe.run(container);
    expect(results).toHaveNoViolations();
  });
});
