import { formatSignedMoney, movementDisplaySign, type MovementKind } from '../../lib/format/money';
import { Link } from 'react-router-dom';
import type { Movement, Account } from '../../lib/api/schema';
import { MovementActions } from './movement-actions';

export function getAccountName(
  id: string | undefined,
  accounts: Account[] | undefined,
): string | React.ReactNode {
  if (!id) return null;
  const acc = accounts?.find((a) => a.id === id);
  return acc ? (
    <Link to="/finance/accounts" className="text-blue-600 hover:underline">
      {acc.name}
    </Link>
  ) : id;
}

export function renderAccounts(movement: Movement, accounts: Account[] | undefined): React.ReactNode {
  if (movement.kind === 'transfer') {
    return (
      <>
        {getAccountName(movement.source_account_id ?? undefined, accounts)}
        {' \u2192 '}
        {getAccountName(movement.destination_account_id ?? undefined, accounts)}
      </>
    );
  }
  return getAccountName(movement.source_account_id ?? movement.destination_account_id ?? undefined, accounts);
}

export function MovementAmount({ movement }: { movement: Movement }) {
  return (
    <span className="font-mono">
      {formatSignedMoney(movement.amount, movement.currency, movementDisplaySign(movement.kind as MovementKind))}
    </span>
  );
}

export function MovementActionCell({ movement }: { movement: Movement }) {
  return <MovementActions movement={movement} />;
}
