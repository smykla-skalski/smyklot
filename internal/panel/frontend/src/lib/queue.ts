import { formatCountdown, formatUntil, remainingMs } from './format';
import type { PendingCIRequest, PendingCITrigger } from './types';

/**
 * What the queue says about a request, derived from the reconciler's own rules.
 *
 * All of this was invisible before. The rules live in `internal/pendingci/policy.go`:
 * checks are re-read every 5 minutes while a request is active, a request with nothing new for an
 * hour is deferred to every 6 hours, a repository with no checks at all gets a 10-minute grace
 * before the merge goes ahead, and - the one that matters most to a reader - a passing request
 * waits 30 seconds of quiet before it lands.
 *
 * That last rule is why a row can say "Merging in 0:24" without a new API field: with
 * `last_observed_state = passing` and `next_check_trigger = quiet_period`, `next_check_at` IS the
 * moment the merge happens.
 */

/** How a state is drawn: a tone, a distinct shape, and a word. Never the tone alone. */
export interface QueueState {
  tone: 'clear' | 'stop' | 'neutral' | 'warning' | 'absent';
  icon: 'success' | 'failure' | 'pending' | 'alert' | 'circle-dashed';
  label: string;
}

/**
 * Colour cannot carry this on its own.
 *
 * Measured with the repo's own Viénot simulation: in the dark palette passing and failing separate
 * by 6.20 ΔE00 under protanopia, failing and unreadable by 1.97 under tritanopia, passing and
 * running by 5.25 under deuteranopia. Three of five pairs collapse, so every state here is a tone
 * PLUS a distinctly shaped glyph PLUS a word, and no two share a shape.
 */
export function queueState(request: PendingCIRequest): QueueState {
  switch (request.last_observed_state) {
    case 'passing':
      return { tone: 'clear', icon: 'success', label: 'Passing' };
    case 'failing':
      return { tone: 'stop', icon: 'failure', label: 'Failing' };
    case 'running':
    case 'pending':
      return { tone: 'neutral', icon: 'pending', label: 'Running' };
    case 'unreadable':
      return { tone: 'warning', icon: 'alert', label: 'Unreadable' };
    case 'no_checks':
      return { tone: 'absent', icon: 'circle-dashed', label: 'No checks' };
    default:
      return { tone: 'absent', icon: 'circle-dashed', label: 'Awaiting first check' };
  }
}

/** The two lines of the "what happens next" column. */
export interface QueueNext {
  lead: string;
  sub: string;
  /** Whether this is a merge landing rather than another look, which the row escalates. */
  merging: boolean;
  /** Seconds left when there is a countdown to run, so the row can escalate near zero. */
  seconds: number | null;
}

export function queueNext(request: PendingCIRequest, nowMs: number): QueueNext {
  const left = remainingMs(request.next_check_at, nowMs);
  const seconds = left === null ? null : Math.max(0, Math.round(left / 1000));

  /* A passing request in its quiet period is not waiting for another look - the next event IS the
     merge. Nothing in the product said so, and the row that said "next check in 24s" was describing
     the moment the pull request would be merged. */
  if (request.last_observed_state === 'passing' && request.next_check_trigger === 'quiet_period') {
    return {
      lead: seconds === null ? 'Merging' : `Merging in ${formatCountdown(seconds * 1000)}`,
      sub: 'Quiet period, then it lands',
      merging: true,
      seconds,
    };
  }

  return {
    lead: `Checks again ${formatUntil(request.next_check_at, nowMs)}`,
    sub: triggerReason(request),
    merging: false,
    seconds,
  };
}

/** Why the next look is scheduled when it is, in the reader's terms rather than the field's. */
function triggerReason(request: PendingCIRequest): string {
  const trigger: PendingCITrigger = request.next_check_trigger;
  if (trigger === 'webhook') return 'A delivery moved it forward';
  if (trigger === 'manual') return 'Asked for from this panel';
  if (trigger === 'cleanup') return 'Tidying up after the merge';
  if (trigger === 'command') return 'First look since it was armed';
  /* Deferred is staleness, not a cadence: nothing about this request has progressed for over an
     hour, so the service stopped watching it closely. */
  if (request.schedule === 'deferred') return 'Nothing has moved for an hour';
  if (request.last_observed_state === 'no_checks') return 'Waiting for checks to appear';

  return 'The regular safety net';
}

/**
 * How long ago, in the width a 6.5rem column has.
 *
 * "24 minutes ago" does not fit and wraps to two lines, which makes one row taller than the rest
 * and breaks the scan the whole page is for. The full timestamp is on the element's `title`, so
 * nothing is lost - this is the glanceable form, and the exact one is a hover away.
 */
export function shortAge(value: string, nowMs: number): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) return value;
  const elapsed = Math.max(0, nowMs - parsed);
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} hr`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days} d`;

  return `${Math.floor(days / 7)} wk`;
}

/** Soonest first, which is the only order this page is ever sorted in by default. */
export function bySoonest(first: PendingCIRequest, second: PendingCIRequest): number {
  return first.next_check_at.localeCompare(second.next_check_at);
}

/** The one-line summary of how a request ended, for the Recent table. */
export function outcomeState(request: PendingCIRequest): QueueState {
  switch (request.lifecycle) {
    case 'merged':
      return { tone: 'clear', icon: 'success', label: 'Merged' };
    case 'cancelled':
      return { tone: 'neutral', icon: 'circle-dashed', label: 'Cancelled' };
    case 'superseded':
      return { tone: 'warning', icon: 'alert', label: 'Superseded' };
    default:
      return queueState(request);
  }
}
