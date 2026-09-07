import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { axe } from 'vitest-axe';

import { DataTable, features } from './data-table';

type Row = {
  id: string;
  name: string;
  category: string;
  purchased: string;
  /** Kept as a string so every column shares one value type (TValue inference). */
  amount: string;
};

function makeRows(count: number): Row[] {
  return Array.from({ length: count }, (_, i) => ({
    id: `r${i}`,
    name: `Item ${i}`,
    category: i % 2 === 0 ? 'appliance' : 'furniture',
    purchased: `2026-${String((i % 9) + 1).padStart(2, '0')}-15`,
    amount: String(i * 10),
  }));
}

const columnHelper = createColumnHelper<typeof features, Row>();

function buildColumns(): ColumnDef<TableFeatures, Row, string>[] {
  // `alphanumeric` gives natural ordering for the mixed text+numeric test data
  // ("Item 24" > "Item 9"), which plain lexicographic `text` would not. The
  // helper is typed against the concrete `features` (which registers the
  // `alphanumeric` sort fn) so the string literal is valid; the result is
  // widened to the generic `TableFeatures` the DataTable `columns` prop expects.
  return [
    columnHelper.accessor('name', { header: 'Name', sortFn: 'alphanumeric' }),
    columnHelper.accessor('category', { header: 'Category' }),
    columnHelper.accessor('purchased', { header: 'Purchased' }),
    columnHelper.accessor('amount', { header: 'Amount', sortFn: 'alphanumeric' }),
  ] as ColumnDef<TableFeatures, Row, string>[];
}

/** Cell text of the first body row, by column header name. */
function firstRowCell(headerName: string): string {
  const headers = screen.getAllByRole('columnheader');
  const index = headers.findIndex((header) => header.textContent?.includes(headerName));
  const rows = screen.getAllByRole('row');
  return rows[1]?.children.item(index)?.textContent ?? '';
}

/**
 * Flush pending macrotasks so Radix's open-menu `setTimeout` callbacks (focus
 * management, pointer-grace timers) fire and the event loop drains. Without
 * this, a subsequent `await` while the menu's timers are still queued makes
 * vitest wait for a macrotask chain that never settles → test timeout.
 */
function flushMacrotasks(ms = 500): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

describe('DataTable', () => {
  let rows: Row[];

  beforeEach(() => {
    rows = makeRows(25);
  });

  it('renders all columns, default page of 10 rows, and is axe-clean', async () => {
    const { container } = render(
      <DataTable ariaLabel="Assets" data={rows} columns={buildColumns()} />,
    );

    expect(screen.getByRole('table', { name: 'Assets' })).toBeInTheDocument();
    // 1 header row + 10 body rows (default pageSize 10)
    expect(screen.getAllByRole('row')).toHaveLength(11);
    expect(screen.getByRole('columnheader', { name: /name/i })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: /category/i })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: /purchased/i })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: /amount/i })).toBeInTheDocument();
    expect(firstRowCell('Name')).toBe('Item 0');

    expect(await axe(container)).toHaveNoViolations();
  });

  it('paginates: next/last/previous and rows-per-page change', async () => {
    render(<DataTable ariaLabel="Assets" data={rows} columns={buildColumns()} />);

    expect(screen.getByText('1-10 of 25')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'First page' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: 'Next page' }));
    expect(screen.getByText('11-20 of 25')).toBeInTheDocument();
    expect(firstRowCell('Name')).toBe('Item 10');
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeEnabled();

    fireEvent.click(screen.getByRole('button', { name: 'Last page' }));
    expect(screen.getByText('21-25 of 25')).toBeInTheDocument();
    expect(screen.getAllByRole('row')).toHaveLength(6); // 1 header + 5 rows
    expect(firstRowCell('Name')).toBe('Item 20');
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: 'Previous page' }));
    expect(screen.getByText('11-20 of 25')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'First page' }));
    expect(screen.getByText('1-10 of 25')).toBeInTheDocument();

    // Rows per page → 50 shows every row on a single page
    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: '50' } });
    await waitFor(() => {
      expect(screen.getByText('1-25 of 25')).toBeInTheDocument();
    });
    expect(screen.getAllByRole('row')).toHaveLength(26); // 1 header + 25 rows
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
  });

  it('supports multi-column sorting with aria-sort and position badge', async () => {
    render(<DataTable ariaLabel="Assets" data={rows} columns={buildColumns()} />);

    const nameHeader = screen.getByRole('columnheader', { name: /name/i });
    const amountHeader = screen.getByRole('columnheader', { name: /amount/i });
    const sortName = screen.getByRole('button', { name: 'Sort by Name' });
    const sortAmount = screen.getByRole('button', { name: 'Sort by Amount' });

    // No initial sort
    expect(nameHeader).toHaveAttribute('aria-sort', 'none');

    // Name ascending (single-sorted columns carry the direction in the label)
    fireEvent.click(sortName);
    expect(nameHeader).toHaveAttribute('aria-sort', 'ascending');
    expect(firstRowCell('Name')).toBe('Item 0');
    expect(screen.getByRole('button', { name: /Sort by Name/ })).toBeInTheDocument();

    // Name descending (rows update through React re-render; wait for it)
    fireEvent.click(screen.getByRole('button', { name: /Sort by Name/ }));
    await waitFor(
      () => {
        expect(nameHeader).toHaveAttribute('aria-sort', 'descending');
        expect(firstRowCell('Name')).toBe('Item 24');
      },
      { timeout: 4000 },
    );

    // Multi-column: add Amount ascending → Name (primary, desc) then Amount
    fireEvent.click(sortAmount);
    expect(nameHeader).toHaveAttribute('aria-sort', 'descending');
    expect(amountHeader).toHaveAttribute('aria-sort', 'ascending');
    expect(firstRowCell('Name')).toBe('Item 24');
    expect(screen.getByRole('button', { name: /Sort by Amount, sorted ascending, position 2/ }))
      .toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument(); // position badge on 2nd-sorted column

    // Toggle Amount → descending; order still primary-sorted by Name desc
    fireEvent.click(screen.getByRole('button', { name: /Sort by Amount, sorted ascending, position 2/ }));
    expect(amountHeader).toHaveAttribute('aria-sort', 'descending');
    expect(firstRowCell('Name')).toBe('Item 24');

    // Clear Name sort → Amount desc becomes the sole sort
    fireEvent.click(screen.getByRole('button', { name: /Sort by Name, sorted descending/ }));
    expect(nameHeader).toHaveAttribute('aria-sort', 'none');
    expect(amountHeader).toHaveAttribute('aria-sort', 'descending');
    expect(firstRowCell('Amount')).toBe('240'); // amounts are numeric strings, so lexicographic = numeric here
    expect(firstRowCell('Name')).toBe('Item 24');
  });

  // The Radix DropdownMenu open/close state machine does not fully reset in
  // jsdom (a second open in the same test hangs on `waitFor`), so the hide and
  // re-show cases are exercised in separate `it` blocks, each with a fresh
  // render. The trigger is driven with `fireEvent.pointerDown` (Radix opens on
  // `onPointerDown`; the PointerEvent/pointer-capture polyfills in
  // `src/test/setup.ts` make it work under jsdom), and `flushMacrotasks` lets
  // Radix's focus-management `setTimeout` fire so the event loop drains.
  it('hides a column from the View dropdown', async () => {
    render(<DataTable ariaLabel="Assets" data={rows} columns={buildColumns()} />);
    const viewButton = screen.getByRole('button', { name: 'Toggle column visibility' });

    fireEvent.pointerDown(viewButton);
    await waitFor(() => {
      expect(screen.getByRole('menuitemcheckbox', { name: 'Category' })).toBeInTheDocument();
    });

    // Uncheck Category → header + cells disappear (clicking the checkbox also
    // closes the Radix menu).
    fireEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Category' }));
    await flushMacrotasks();
    await waitFor(() => {
      expect(screen.queryByRole('columnheader', { name: /category/i })).not.toBeInTheDocument();
    });
    expect(screen.getByRole('columnheader', { name: /name/i })).toBeInTheDocument();
    expect(screen.getAllByRole('row')[1].children).toHaveLength(3); // Name, Purchased, Amount
  }, 20000);

  // Start with Category already hidden via `initialColumnVisibility`, then do a
  // SINGLE open + re-check cycle. This exercises the re-show path without the
  // flaky two-open jsdom pattern (a second open in the same test hangs because
  // Radix's DropdownMenu open/close state machine does not fully reset).
  it('re-shows a hidden column from the View dropdown', async () => {
    render(
      <DataTable
        ariaLabel="Assets"
        data={rows}
        columns={buildColumns()}
        initialColumnVisibility={{ category: false }}
      />,
    );
    const viewButton = screen.getByRole('button', { name: 'Toggle column visibility' });

    // Category starts hidden.
    expect(screen.queryByRole('columnheader', { name: /category/i })).not.toBeInTheDocument();
    expect(screen.getAllByRole('row')[1].children).toHaveLength(3); // Name, Purchased, Amount

    // Open the View dropdown and re-check Category → header + cells reappear.
    fireEvent.pointerDown(viewButton);
    await waitFor(() => {
      expect(screen.getByRole('menuitemcheckbox', { name: 'Category' })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Category' }));
    await flushMacrotasks();
    await waitFor(() => {
      expect(screen.getByRole('columnheader', { name: /category/i })).toBeInTheDocument();
    });
    expect(screen.getAllByRole('row')[1].children).toHaveLength(4);
  }, 20000);

  it('shows an empty state row when there is no data and stays axe-clean', async () => {
    const { container } = render(
      <DataTable ariaLabel="Assets" data={[]} columns={buildColumns()} />,
    );

    expect(screen.getByText('No results.')).toBeInTheDocument();
    expect(screen.getByText('0 of 0')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();

    expect(await axe(container)).toHaveNoViolations();
  });
});

// The `vitest-axe` package registers `toHaveNoViolations` at runtime via
// `expect.extend` in the test setup, but its bundled type augmentation targets a
// `Vi` namespace that vitest v3 does not use. Declare the matcher against the
// real `@vitest/expect` module so `tsc -b` can resolve it.
declare module '@vitest/expect' {
  interface Matchers<T = any> {
    toHaveNoViolations(): {
      actual: import('axe-core').Result[];
      pass: boolean;
      message(): string;
    };
  }
}
