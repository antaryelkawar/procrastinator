import { TableRow, TableCell } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { formatSignedMoney, movementDisplaySign } from '../../lib/format/money';
import { formatDate } from '../../lib/format/date';
import type { Movement } from '../../lib/api/types';
import { MovementActions } from './movement-actions';

interface MovementRowProps {
  readonly movement: Movement;
}

export function MovementRow({ movement }: MovementRowProps) {
  const isImported = movement.origin === 'import';

  return (
    <TableRow>
      <TableCell>{movement.kind}</TableCell>
      <TableCell className="font-mono">
        {formatSignedMoney(movement.amount, movement.currency, movementDisplaySign(movement.kind))}
      </TableCell>
      <TableCell>{formatDate(movement.occurred_on)}</TableCell>
      <TableCell>{movement.description}</TableCell>
      <TableCell>
        <Badge variant={isImported ? 'outline' : 'default'}>
          {movement.origin}
        </Badge>
      </TableCell>
      <TableCell>{movement.source_account_id || movement.destination_account_id || '-'}</TableCell>
      <TableCell>
        <MovementActions movement={movement} />
      </TableCell>
    </TableRow>
  );
}
