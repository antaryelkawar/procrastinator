import { ImportBatch } from '../../lib/api/types';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { Badge } from '../../components/ui/badge';

interface Props {
  batch: ImportBatch;
}

export function ImportLinesTable({ batch }: Props) {
  if (!batch.lines) return null;

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Date</TableHead>
          <TableHead>Amount</TableHead>
          <TableHead>Description</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {batch.lines.map((line) => (
          <TableRow key={line.line_ref}>
            <TableCell>{line.occurred_on || '-'}</TableCell>
            <TableCell>
              {line.amount
                ? `${line.direction === 'out' ? '\u2212' : line.direction === 'in' ? '+' : ''}${line.amount}`
                : '-'}
            </TableCell>
            <TableCell>{line.description || '-'}</TableCell>
            <TableCell>
              <Badge variant={line.status === 'valid' ? 'default' : 'destructive'}>
                {line.status}
              </Badge>
              {line.error_reason && <p className="text-xs text-destructive mt-1">{line.error_reason}</p>}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
