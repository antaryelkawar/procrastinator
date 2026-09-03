import { useSearchParams } from 'react-router-dom';
import { useMovements, useAccounts } from '../../lib/api/hooks';
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { cn } from '@/lib/utils';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loading } from '@/components/feedback/loading';
import { MovementRow } from './movements-row';

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
    <div className={cn('container mx-auto py-8 space-y-6')} data-testid="movements-page">
      <h1 className="text-2xl font-bold">Money movements</h1>
      
      <div className="flex gap-4 items-end">
        <div>
          <Label htmlFor="movements-account-select">Account</Label>
          <select
            id="movements-account-select"
            name="account_id"
            value={accountId ?? 'all'}
            onChange={(e) => handleAccountChange(e.target.value)}
            className="w-[200px] h-8 rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
          >
            <option value="all">All accounts</option>
            {accounts?.map((acc) => (
              <option key={acc.id} value={acc.id}>{acc.name}</option>
            ))}
          </select>
        </div>
        
        <div>
          <Label htmlFor="movements-from">From</Label>
          <Input
            id="movements-from"
            type="date"
            value={from ?? ''} 
            onChange={(e) => handleFilterChange('from', e.target.value)}
            className="w-[150px]"
          />
        </div>
        
        <div>
          <Label htmlFor="movements-to">To</Label>
          <Input
            id="movements-to"
            type="date"
            value={to ?? ''} 
            onChange={(e) => handleFilterChange('to', e.target.value)}
            className="w-[150px]"
          />
        </div>
      </div>

      {isLoading ? (
        <Loading />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Kind</TableHead>
              <TableHead>Amount</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Description</TableHead>
              <TableHead>Origin</TableHead>
              <TableHead>Account</TableHead>
              <TableHead>Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {movements?.map(m => <MovementRow key={m.id} movement={m} />)}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
