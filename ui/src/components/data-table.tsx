import { useState, type ReactNode } from 'react';
import {
  columnVisibilityFeature,
  createPaginatedRowModel,
  createSortedRowModel,
  flexRender,
  rowPaginationFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_basic,
  sortFn_text,
  tableFeatures,
  useTable,
  type ColumnDef,
  type ColumnVisibilityState,
  type PaginationState,
  type SortingState,
  type TableFeatures,
} from '@tanstack/react-table';
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { cn } from '@/lib/utils';

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const;

/**
 * Native v9 feature set for the shared data table: column visibility,
 * multi-column sorting, and client-side pagination. The core row model is
 * automatic in v9 (no `getCoreRowModel` option).
 */
// Register the sort functions the auto-detection may resolve to. `alphanumeric`
// is required: without it, `column_getAutoSortFn` falls back to plain
// lexicographic `text` for mixed text+numeric values (e.g. "Item 10" < "Item 9"
// lexicographically but > "Item 2" naturally). Registering `alphanumeric` lets
// auto-detection pick natural ordering for such columns.
export const features = tableFeatures({
  columnVisibilityFeature,
  rowSortingFeature,
  rowPaginationFeature,
  paginatedRowModel: createPaginatedRowModel(),
  sortedRowModel: createSortedRowModel(),
  sortFns: {
    alphanumeric: sortFn_alphanumeric,
    text: sortFn_text,
    basic: sortFn_basic,
  },
});

export interface DataTableProps<TData extends Record<string, unknown>, TValue> {
  readonly data: TData[];
  readonly columns: ColumnDef<TableFeatures, TData, TValue>[];
  /** Accessible name for the table (axe: tables need a label). Default "Data table". */
  readonly ariaLabel?: string;
  /** Default page size when pagination is on. Default 10. */
  readonly pageSize?: number;
  /** Show the toolbar with the column-visibility ("View") dropdown. Default true. */
  readonly toolbar?: boolean;
  /** Enable multi-column sorting via clickable headers. Default true. */
  readonly sort?: boolean;
  /** Enable client-side pagination + footer. Default true. */
  readonly pagination?: boolean;
  /** Stable row id accessor (TanStack auto id otherwise). */
  readonly getRowId?: (row: TData, index: number) => string;
  /** Initial column visibility. Defaults to all columns visible. */
  readonly initialColumnVisibility?: ColumnVisibilityState;
  /** Row click handler; adds cursor-pointer to rows. */
  readonly onRowClick?: (row: TData) => void;
  readonly className?: string;
}

/**
 * Shared feature-rich data table on TanStack Table v9 (native `useTable`):
 * column visibility selection, multi-column sorting, and pagination.
 *
 * Framework-agnostic: renders through the shadcn table primitives and takes
 * plain data + `ColumnDef` config — no routing/query dependencies.
 */
export function DataTable<TData extends Record<string, unknown>, TValue = unknown>(
  props: DataTableProps<TData, TValue>,
) {
  const {
    data,
    columns,
    ariaLabel,
    pageSize = 10,
    toolbar = true,
    sort = true,
    pagination = true,
    getRowId,
    onRowClick,
    className,
    initialColumnVisibility,
  } = props;

  const [sorting, setSorting] = useState<SortingState>([]);
  const [columnVisibility, setColumnVisibility] = useState<ColumnVisibilityState>(
    () => initialColumnVisibility ?? {},
  );

  const [paginationState, setPaginationState] = useState<PaginationState>(() => ({
    pageIndex: 0,
    pageSize,
  }));

  // `useTable`'s `columns` slot is invariant in TValue (it requires `unknown`),
  // so the caller's per-column TValue must be widened here. The render code
  // below reads cell values through `flexRender` (typed as `unknown | null`),
  // so no information is lost.
  const typedColumns = columns as unknown as ColumnDef<TableFeatures, TData, unknown>[];

  const table = useTable({
    features,
    data,
    columns: typedColumns,
    getRowId,
    state: {
      sorting,
      columnVisibility,
      pagination: paginationState,
    },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPaginationState,
    enableMultiSort: sort,
    // First click is always ascending; further clicks flip direction.
    sortDescFirst: false,
  });

  const rows = table.getRowModel().rows;
  const visibleColumns = table.getVisibleLeafColumns();
  const tableLabel = ariaLabel ?? 'Data table';
  const { pageSize: activePageSize, pageIndex } = table.state.pagination;
  const rowStart = rows.length === 0 ? 0 : pageIndex * activePageSize + 1;
  const rowEnd = rows.length === 0 ? 0 : rowStart + rows.length - 1;

  return (
    <div data-slot="data-table" className={cn('flex w-full flex-col gap-2', className)}>
      {toolbar ? (
        <div data-slot="data-table-toolbar" className="flex items-center justify-end gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm" aria-label="Toggle column visibility">
                View
                <ChevronDown aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-40">
              <DropdownMenuLabel>Toggle columns</DropdownMenuLabel>
              {table
                .getAllColumns()
                .flatMap((column) => {
                  const header = column.columnDef.header;
                  if (typeof header !== 'string') {
                    return [];
                  }
                  return [
                    <DropdownMenuCheckboxItem
                      key={column.id}
                      checked={column.getIsVisible()}
                      onCheckedChange={() => column.toggleVisibility(!column.getIsVisible())}
                    >
                      {header}
                    </DropdownMenuCheckboxItem>,
                  ];
                })}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ) : null}

      <Table aria-label={tableLabel}>
        <TableHeader>
          <TableRow>
            {table.getVisibleLeafColumns().map((column) => {
              // Read from the table instance (not the React state copy) so the
              // header always reflects the committed sort state on this render.
              const sortEntry = table
                .state.sorting.find((entry) => entry.id === column.id);
              const ariaSort: 'ascending' | 'descending' | 'none' = sortEntry
                ? sortEntry.desc
                  ? 'descending'
                  : 'ascending'
                : 'none';
              const headerValue = flexRender(
                column.columnDef.header,
                { column } as never,
              ) as ReactNode;
              const rawHeader = column.columnDef.header;
              const headerText =
                typeof headerValue === 'string'
                  ? headerValue
                  : typeof rawHeader === 'string'
                    ? rawHeader
                    : column.id;
              if (!sort || !column.getCanSort()) {
                return (
                  <TableHead key={column.id} aria-sort={ariaSort}>
                    {headerValue}
                  </TableHead>
                );
              }
              // Explicit direction: first click → ascending (v9 auto-detection
              // would otherwise start descending for non-string values); while
              // in a multi-sort the button adds/removes this column without
              // disturbing the other sort entries.
              const inMultiSort = sortEntry !== undefined && sorting.length > 1;
              const sortIndex = sortEntry ? table.state.sorting.indexOf(sortEntry) : -1;
              const directionLabel =
                sortEntry === undefined ? null : sortEntry.desc ? 'descending' : 'ascending';
              const ariaLabel =
                directionLabel === null
                  ? `Sort by ${headerText}`
                  : inMultiSort
                    ? `Sort by ${headerText}, sorted ${directionLabel}, position ${sortIndex + 1}`
                    : `Sort by ${headerText}, sorted ${directionLabel}`;
              const handleSortClick = () => {
                const current = table.state.sorting;
                const currentEntry = current.find((entry) => entry.id === column.id);
                if (currentEntry === undefined) {
                  // Not sorted yet → add ascending.
                  column.toggleSorting(false, true);
                  return;
                }
                const isPrimary = current.indexOf(currentEntry) === 0;
                if (current.length > 1 && isPrimary) {
                  // Primary column in a multi-sort → remove it, leaving the
                  // other sorted column(s) untouched.
                  column.clearSorting();
                  return;
                }
                // Sole sort, or a secondary column in a multi-sort → flip this
                // column's direction. Passing `multi=true` keeps the other
                // sorted column(s) in place (v9.2.4's `toggleSorting` with
                // `multi=false` would otherwise replace the whole sort with just
                // this column). The explicit `desc` value flips the direction.
                column.toggleSorting(!currentEntry.desc, true);
              };
              return (
                <TableHead key={column.id} aria-sort={ariaSort}>
                  <button
                    type="button"
                    className="inline-flex items-center gap-1 hover:underline"
                    aria-label={ariaLabel}
                    onClick={handleSortClick}
                  >
                    {headerValue}
                    {sortEntry?.desc ? (
                      <ArrowDown aria-hidden="true" />
                    ) : sortEntry ? (
                      <ArrowUp aria-hidden="true" />
                    ) : (
                      <ArrowUpDown aria-hidden="true" />
                    )}
                    {inMultiSort ? (
                      <span className="text-xs text-muted-foreground">{sortIndex + 1}</span>
                    ) : null}
                  </button>
                </TableHead>
              );
            })}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={visibleColumns.length}
                className="h-24 text-center text-muted-foreground"
              >
                No results.
              </TableCell>
            </TableRow>
          ) : (
            rows.map((row) => (
              <TableRow
                key={row.id}
                onClick={onRowClick ? () => onRowClick(row.original as TData) : undefined}
                className={cn(onRowClick && 'cursor-pointer')}
              >
                 {row.getVisibleCells().map((cell) => (
                   <TableCell key={cell.id}>
                     {flexRender(cell.column.columnDef.cell, cell.getContext()) as ReactNode}
                   </TableCell>
                 ))}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      {pagination ? (
        <div data-slot="data-table-footer" className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <label htmlFor="data-table-page-size" className="text-sm text-muted-foreground">
              Rows per page
            </label>
            <select
              id="data-table-page-size"
              className="h-8 rounded-lg border border-input bg-transparent px-2 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring"
              value={activePageSize}
              onChange={(event) => table.setPageSize(Number(event.target.value))}
            >
              {PAGE_SIZE_OPTIONS.map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
            <span className="text-sm text-muted-foreground" aria-live="polite">
              {rows.length === 0 ? '0 of 0' : `${rowStart}-${rowEnd} of ${data.length}`}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="icon"
              aria-label="First page"
              onClick={() => table.setPageIndex(0)}
              disabled={!table.getCanPreviousPage()}
            >
              <ChevronsLeft aria-hidden="true" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              aria-label="Previous page"
              onClick={() => table.previousPage()}
              disabled={!table.getCanPreviousPage()}
            >
              <ChevronLeft aria-hidden="true" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              aria-label="Next page"
              onClick={() => table.nextPage()}
              disabled={!table.getCanNextPage()}
            >
              <ChevronRight aria-hidden="true" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              aria-label="Last page"
              onClick={() => table.setPageIndex(table.getPageCount() - 1)}
              disabled={!table.getCanNextPage()}
            >
              <ChevronsRight aria-hidden="true" />
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
