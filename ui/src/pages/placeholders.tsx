/**
 * Placeholder screens for the asset-ui route table (task 3.2).
 *
 * Every route in `router.tsx` renders one of these until its real screen
 * lands (tasks 4.1–4.10 replace these components one by one; the route table
 * and the shell stay unchanged). A placeholder renders the screen's heading
 * plus an <EmptyState>, so the shell, navigation, deep linking, and shared
 * feedback are fully exercisable now.
 */
import type { ReactNode } from 'react';
import { useParams } from 'react-router-dom';
import { EmptyState } from '@/components/feedback/empty-state';

interface PlaceholderScreenProps {
  /** Screen heading (the h1 of the route). */
  readonly title: string;
  /** The route path, shown for traceability. */
  readonly route: string;
  /** The task that builds the real screen. */
  readonly task: string;
  /** Optional extra content between the heading and the empty state. */
  readonly extra?: ReactNode;
}

function PlaceholderScreen({ title, route, task, extra }: PlaceholderScreenProps) {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      {extra}
      <EmptyState
        title={`${title} is on its way`}
        description={`Route ${route} is wired and the surrounding app shell is final; the screen itself lands in task ${task}.`}
      />
    </div>
  );
}

function ParamValue({ label, value }: { readonly label: string; readonly value?: string }) {
  if (value === undefined) {
    return null;
  }
  return (
    <p className="text-sm text-muted-foreground">
      {label}: <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{value}</code>
    </p>
  );
}

/** `/upload` — document upload (task 4.1). */
export function UploadPlaceholder() {
  return <PlaceholderScreen title="Upload" route="/upload" task="4.1" />;
}

/** `/finance/movements` — money movements (tasks 4.5–4.8). */
export function MovementsPlaceholder() {
  return <PlaceholderScreen title="Money movements" route="/finance/movements" task="4.5–4.8" />;
}

/** `/finance/import` — statement import (task 4.9). */
export function ImportPlaceholder() {
  return <PlaceholderScreen title="Statement import" route="/finance/import" task="4.9" />;
}

/** `/finance/import/history` — import history (task 4.10). */
export function ImportHistoryPlaceholder() {
  return <PlaceholderScreen title="Import history" route="/finance/import/history" task="4.10" />;
}

/** `/finance/import/:batchId` — batch detail (task 4.10). */
export function BatchDetailPlaceholder() {
  const { batchId } = useParams<{ batchId?: string }>();
  return (
    <PlaceholderScreen
      title="Batch detail"
      route="/finance/import/:batchId"
      task="4.10"
      extra={<ParamValue label="Deep-linked batch id" value={batchId} />}
    />
  );
}
