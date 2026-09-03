import { TableRow, TableCell } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { formatSignedMoney, movementDisplaySign, type MovementKind } from '../../lib/format/money';
import { formatDate } from '../../lib/format/date';
import type { Movement, Account } from '../../lib/api/schema';
import { MovementActions } from './movement-actions';
import { Link } from 'react-router-dom';

interface MovementRowProps {
  readonly movement: Movement;
  readonly accounts: Account[] | undefined;
}

export function MovementRow({ movement, accounts }: MovementRowProps) {
  const isImported = movement.origin === 'import';
  
  const getAccountName = (id: string | undefined) => {
    if (!id) return null;
    const acc = accounts?.find(a => a.id === id);
    return acc ? (
      <Link to="/finance/accounts" className="text-blue-600 hover:underline">{acc.name}</Link>
    ) : id;
  };

  const renderAccounts = () => {
    if (movement.kind === 'transfer') {
      return (
        <>
          {getAccountName(movement.source_account_id ?? undefined)}
          {' → '}
          {getAccountName(movement.destination_account_id ?? undefined)}
        </>
      );
    }
    return getAccountName(movement.source_account_id ?? movement.destination_account_id ?? undefined);
  };

  return (
    <TableRow>
      <TableCell>{movement.kind}</TableCell>
      <TableCell className="font-mono">
        {formatSignedMoney(movement.amount, movement.currency, movementDisplaySign(movement.kind as MovementKind))}
      </TableCell>
      <TableCell>{formatDate(movement.occurred_on)}</TableCell>
      <TableCell>{movement.description}</TableCell>
      <TableCell>
        <Badge variant={isImported ? 'outline' : 'default'}>
          {movement.origin}
        </Badge>
      </TableCell>
      <TableCell>{renderAccounts()}</TableCell>
      <TableCell>
        <MovementActions movement={movement} />
      </TableCell>
    </TableRow>
  );
}
