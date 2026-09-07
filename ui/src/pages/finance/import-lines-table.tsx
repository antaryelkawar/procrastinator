import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { ImportBatch, ImportLine } from '../../lib/api/schema';
import { Badge } from '../../components/ui/badge';
import { DataTable, features } from '../../components/data-table';

interface Props {
  batch: ImportBatch;
}

const columnHelper = createColumnHelper<typeof features, ImportLine>();

function buildColumns(): ColumnDef<TableFeatures, ImportLine, unknown>[] {
  return [
    columnHelper.accessor('occurred_on', {
      header: 'Date',
      cell: (context) => context.getValue() || '-',
    }),
    columnHelper.accessor('amount', {
      header: 'Amount',
      cell: (context) =>
        context.getValue()
          ? `${context.row.original.direction === 'out' ? '\u2212' : context.row.original.direction === 'in' ? '+' : ''}${context.getValue()}`
          : '-',
    }),
    columnHelper.accessor('description', {
      header: 'Description',
      cell: (context) => context.getValue() || '-',
    }),
    columnHelper.accessor('status', {
      header: 'Status',
      cell: (context) => (
        <>
          <Badge variant={context.getValue() === 'valid' ? 'default' : 'destructive'}>
            {context.getValue()}
          </Badge>
          {context.row.original.error_reason && (
            <p className="text-xs text-destructive mt-1">{context.row.original.error_reason}</p>
          )}
        </>
      ),
    }),
  ] as ColumnDef<TableFeatures, ImportLine, unknown>[];
}

export function ImportLinesTable({ batch }: Props) {
  if (!batch.lines) return null;

  return (
    <DataTable
      ariaLabel="Import lines"
      data={batch.lines}
      getRowId={(line) => String(line.line_ref)}
      columns={buildColumns()}
    />
  );
}
