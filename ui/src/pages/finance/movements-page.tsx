import { useSearchParams } from 'react-router-dom';
import { createColumnHelper, type ColumnDef, type TableFeatures } from '@tanstack/react-table';
import { useMovements, useAccounts } from '../../lib/api/hooks';
import { cn } from '@/lib/utils';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Loading } from '@/components/feedback/loading';
import { DataTable, features } from '@/components/data-table';
import { formatDate } from '../../lib/format/date';
import { renderAccounts, MovementAmount, MovementActionCell } from './movements-row';
import { MovementCreateForm } from './movement-create-form';
import type { Movement, Account } from '../../lib/api/schema';

const columnHelper = createColumnHelper<typeof features, Movement>();

function buildColumns(accounts: Account[] | undefined): ColumnDef<TableFeatures, Movement, unknown>[] {
  return [
    columnHelper.accessor('kind', {
      header: 'Kind',
    }),
    columnHelper.accessor('amount', {
      header: 'Amount',
      cell: (context) => <MovementAmount movement={context.row.original} />,
    }),
    columnHelper.accessor('occurred_on', {
      header: 'Date',
      cell: (context) => formatDate(context.getValue()),
    }),
    columnHelper.accessor('description', {
      header: 'Description',
    }),
    columnHelper.accessor('origin', {
      header: 'Origin',
      cell: (context) => (
        <Badge variant={context.getValue() === 'import' ? 'outline' : 'default'}>
          {context.getValue()}
        </Badge>
      ),
    }),
    columnHelper.accessor('source_account_id', {
      header: 'Account',
      enableSorting: false,
      cell: (context) => renderAccounts(context.row.original, accounts),
    }),
    {
      id: 'actions',
      header: 'Actions',
      enableSorting: false,
      cell: (context) => <MovementActionCell movement={context.row.original} />,
    },
  ] as ColumnDef<TableFeatures, Movement, unknown>[];
}

export function MovementsPage() {
  const [searchParams, setSearchParams] = useSearchParams();

  const accountId = searchParams.get('account_id') ?? undefined;
  const from = searchParams.get('from') ?? undefined;
  const to = searchParams.get('to') ?? undefined;

  const { data: movements, isLoading } = useMovements({ accountId, from, to });
  const { data: accounts } = useAccounts();

  const handleFilterChange = (key: string, value: string | undefined) => {
    const newParams = new URLSearchParams(searchParams);
    if (value) {
      newParams.set(key, value);
    } else {
      newParams.delete(key);
    }
    setSearchParams(newParams);
  };

  const handleAccountChange = (value: string) => {
    handleFilterChange('account_id', value === 'all' ? undefined : value);
  };

  return (
    <div className={cn('container mx-auto min-w-0 py-8 space-y-6')} data-testid="movements-page">
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
        <h1 className="text-2xl font-bold">Money movements</h1>
        <MovementCreateForm />
      </div>

      <div className="flex flex-col sm:flex-row gap-4">
        <div className="w-full sm:w-auto">
          <Label htmlFor="movements-account-select">Account</Label>
          <Select
            value={accountId ?? 'all'}
            onValueChange={handleAccountChange}
          >
            <SelectTrigger className="w-full sm:w-[200px]" aria-label="Account">
              <SelectValue placeholder="All accounts" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All accounts</SelectItem>
              {accounts?.map((acc) => (
                <SelectItem key={acc.id} value={acc.id}>{acc.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="w-full sm:w-auto">
          <Label htmlFor="movements-date-from">From</Label>
          <Input
            id="movements-date-from"
            type="date"
            value={from ?? ''}
            onChange={(e) => handleFilterChange('from', e.target.value)}
            className="w-full sm:w-[150px]"
          />
        </div>

        <div className="w-full sm:w-auto">
          <Label htmlFor="movements-date-to">To</Label>
          <Input
            id="movements-date-to"
            type="date"
            value={to ?? ''}
            onChange={(e) => handleFilterChange('to', e.target.value)}
            className="w-full sm:w-[150px]"
          />
        </div>
      </div>

      {isLoading ? (
        <Loading />
      ) : (
        <DataTable
          ariaLabel="Money movements"
          data={movements ?? []}
          getRowId={(movement) => movement.id}
          columns={buildColumns(accounts)}
        />
      )}
    </div>
  );
}
