import type { QueueWorkload } from './types';

/**
 * What each background job is called, and what it does.
 *
 * The Schedules page names them in a sentence and the console's overview names
 * one in a row, so the words live here rather than in whichever page said them
 * first. A job whose title differs between two pages is a job an operator cannot
 * search for.
 *
 * Each description says what the job DOES, in the third person and with the job
 * as the subject - "Accepts what GitHub sends", never "Accept and deliver GitHub
 * events", which reads as an instruction to the reader.
 */
export const WORKLOAD_COPY: Record<QueueWorkload, { title: string; description: string }> = {
  webhook_delivery: {
    title: 'Webhook intake',
    description: 'Accepts what GitHub sends',
  },
  pending_ci: {
    title: 'CI re-checks',
    description: 'Rechecks merge requests waiting on CI',
  },
  pending_ci_gate: {
    title: 'Deferred CI gate',
    description: 'Wakes deferred checks after their quiet period',
  },
  catalog_refresh: {
    title: 'Catalog refresh',
    description: 'Discovers workspaces and their repositories',
  },
  reaction_scan: {
    title: 'Reaction discovery',
    description: 'Finds pull request approval reactions',
  },
  config_migration: {
    title: 'Configuration migration',
    description: 'Moves repositories to the current configuration',
  },
  sync_scan: {
    title: 'Workspace sync scan',
    description: 'Computes drift and prepares plans',
  },
  sync_apply: {
    title: 'Sync plan execution',
    description: 'Applies a plan an owner has approved',
  },
  path_refresh: {
    title: 'File indexing',
    description: "Refreshes each repository's file list",
  },
  delivery_cleanup: {
    title: 'Delivery retention',
    description: 'Archives deliveries past their retention window',
  },
  auth_cleanup: {
    title: 'Sign-in session cleanup',
    description: 'Ends sign-in sessions and credentials past their lifetime',
  },
  schedule_change: {
    title: 'Schedule change',
    description: 'Applies a timing change an operator has approved',
  },
};

export function workloadTitle(kind: QueueWorkload): string {
  return WORKLOAD_COPY[kind].title;
}

export function workloadDescription(kind: QueueWorkload): string {
  return WORKLOAD_COPY[kind].description;
}

const CADENCE_UNITS: ReadonlyArray<{ seconds: number; one: string; many: string }> = [
  { seconds: 86_400, one: 'day', many: 'days' },
  { seconds: 3_600, one: 'hour', many: 'hours' },
  { seconds: 60, one: 'minute', many: 'minutes' },
  { seconds: 1, one: 'second', many: 'seconds' },
];

/**
 * A cadence in words: "every 30 minutes", "every day".
 *
 * The wire carries nanoseconds, and the Schedules table says them in the short
 * form a column has room for - `30m`. A sentence has room for the words, and a
 * request an operator has to approve is read as a sentence.
 */
export function cadenceWords(nanoseconds: number): string {
  const seconds = Math.round(nanoseconds / 1_000_000_000);
  if (seconds <= 0) return 'immediately';

  const unit =
    CADENCE_UNITS.find((candidate) => seconds % candidate.seconds === 0) ?? CADENCE_UNITS[3];
  const amount = seconds / unit.seconds;

  return amount === 1 ? `every ${unit.one}` : `every ${amount} ${unit.many}`;
}
