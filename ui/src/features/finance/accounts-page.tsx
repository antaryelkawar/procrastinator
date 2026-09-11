import { useAccounts } from '@/features/docs/hooks';
import { formatMoney } from '@/lib/format/money';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useState } from 'react';
import { AddButton } from '@/features/docs/add-button';
import { AccountComposerFlow } from './account-composer-flow';

export function AccountsPage() {
  const { data: accounts, isLoading } = useAccounts();
  const [composerOpen, setComposerOpen] = useState(false);

  return (
    <div className="container mx-auto py-8 space-y-8">
      <h1 className="text-2xl font-bold">Accounts</h1>

      {/* Shared top-right [+] chrome opens the directive-first account-creation
          flow (task 14.5). The legacy multi-field "Create Account" form is gone
          from the add path (old-style entry is edit-time only). */}
      <AddButton label="Add account" onAdd={() => setComposerOpen(true)} />
      <AccountComposerFlow open={composerOpen} onOpenChange={setComposerOpen} />

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
                  <span>{acc.data.name} ({acc.data.account_type})</span>
                  <span>{formatMoney(acc.data.balance, acc.data.currency)}</span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
