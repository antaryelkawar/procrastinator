import { useAccounts, useCreateAccount } from '../../lib/api/hooks';
import { formatMoney } from '../../lib/format/money';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useState } from 'react';

export function AccountsPage() {
  const { data: accounts, isLoading } = useAccounts();
  const createAccount = useCreateAccount();
  
  const [name, setName] = useState('');
  const [type, setType] = useState<string>('');
  const [currency, setCurrency] = useState('');
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name || !type || !currency) {
      setError('Name, Type, and Currency are required');
      return;
    }
    
    createAccount.mutate(
      { name, type: type as any, currency },
      {
        onSuccess: () => {
          setName('');
          setType('');
          setCurrency('');
          setError(null);
        },
        onError: (err) => {
          setError(err.message || 'Failed to create account');
        },
      }
    );
  };

  return (
    <div className="container mx-auto py-8 space-y-8">
      <h1 className="text-2xl font-bold">Accounts</h1>
      
      <Card>
        <CardHeader>
          <CardTitle>Create Account</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <Label htmlFor="name">Name</Label>
              <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div>
              <Label htmlFor="type">Type</Label>
              <Select value={type} onValueChange={setType}>
                <SelectTrigger id="type">
                  <SelectValue placeholder="Select type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bank">Bank</SelectItem>
                  <SelectItem value="wallet">Wallet</SelectItem>
                  <SelectItem value="cash">Cash</SelectItem>
                  <SelectItem value="credit_card">Credit Card</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="currency">Currency</Label>
              <Input id="currency" value={currency} onChange={(e) => setCurrency(e.target.value)} />
            </div>
            {error && <p className="text-red-500" data-testid="error-message">{error}</p>}
            <Button type="submit" disabled={createAccount.isPending}>
              {createAccount.isPending ? 'Creating...' : 'Create Account'}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Accounts</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p>Loading...</p>
          ) : (
            <ul className="space-y-2">
              {accounts?.map((acc) => (
                <li key={acc.id} className="flex justify-between p-2 border rounded">
                  <span>{acc.name} ({acc.type})</span>
                  <span>{formatMoney(acc.balance, acc.currency)}</span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
