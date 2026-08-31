import type { QueueWorkload } from './types';

/**
 * What each background lane is called, and what it does.
 *
 * The Schedules page names them in a table and the console's overview names one
 * in a sentence, so the words live here rather than in whichever page said them
 * first. A lane whose title differs between two pages is a lane an operator
 * cannot search for.
 */
export const WORKLOAD_COPY: Record<QueueWorkload, { title: string; description: string }> = {
  webhook_delivery: {
    title: 'Webhook delivery',
    description: 'Accept and deliver GitHub events',
  },
  pending_ci: {
    title: 'Pending CI checks',
    description: 'Recheck merge requests waiting on CI',
  },
  pending_ci_gate: {
    title: 'Deferred CI gate',
    description: 'Wake deferred checks after their quiet period',
  },
  catalog_refresh: {
    title: 'Catalog refresh',
    description: 'Discover installations and repositories',
  },
  reaction_scan: {
    title: 'Reaction discovery',
    description: 'Find pull request approval reactions',
  },
  config_migration: {
    title: 'Configuration migration',
    description: 'Move repositories to the current configuration',
  },
  sync_scan: {
    title: 'Organization sync scan',
    description: 'Compute drift and prepare an approval plan',
  },
  sync_apply: {
    title: 'Sync plan execution',
    description: 'Apply a previously approved organization plan',
  },
  path_refresh: {
    title: 'Path indexing',
    description: 'Refresh repository configuration paths',
  },
  delivery_cleanup: {
    title: 'Delivery retention',
    description: 'Remove expired delivery history',
  },
  auth_cleanup: {
    title: 'Authentication cleanup',
    description: 'Remove expired sessions and credentials',
  },
  schedule_change: {
    title: 'Schedule change',
    description: 'Apply an approved recurring policy request',
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
  if (seconds <= 0) return 'continuously';

  const unit =
    CADENCE_UNITS.find((candidate) => seconds % candidate.seconds === 0) ?? CADENCE_UNITS[3];
  const amount = seconds / unit.seconds;

  return amount === 1 ? `every ${unit.one}` : `every ${amount} ${unit.many}`;
}
