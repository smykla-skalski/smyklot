/**
 * What a piece of queued work says about itself.
 *
 * One sentence: what state it is in, why, and what happens next. The queue table reads
 * it into a row and the console's overview reads it into a card, and they used to say
 * different things about the same item - the overview named the state the service files
 * it under ("Retrying") where the table said the wait a reader can act on.
 *
 * The clock is passed in rather than read here, because a caller that redraws on a tick
 * owns the tick.
 */
import type { QueueItem } from '#lib/types.js';

/** A row's sentence, in the three pieces a time has to be an element to sit between. */
export interface QueueLine {
  lead: string;
  when?: { relative: string; exact: string; iso: string };
  tail?: string;
}

/** A wire word as a person writes it: `awaiting_approval` is "Awaiting approval". */
export function words(value: string): string {
  return value.replaceAll('_', ' ').replace(/^./, (letter) => letter.toUpperCase());
}

/** One of a thing is one, not one of them: "1 hours ago" is nobody's sentence. */
function count(value: number, unit: string): string {
  return `${value} ${unit}${value === 1 ? '' : 's'}`;
}

export function absolute(value: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    timeZoneName: 'short',
    ...(timeZone === undefined ? {} : { timeZone }),
  }).format(new Date(value));
}

function countdown(value: string, now: number): string {
  const seconds = Math.round((new Date(value).getTime() - now) / 1000);
  if (seconds <= 0) return 'now';
  if (seconds < 60) return `in ${count(seconds, 'second')}`;
  if (seconds < 3600) return `in ${count(Math.ceil(seconds / 60), 'minute')}`;
  if (seconds < 86_400) return `in ${count(Math.ceil(seconds / 3600), 'hour')}`;

  return `in ${count(Math.ceil(seconds / 86_400), 'day')}`;
}

function ago(value: string, now: number): string {
  const seconds = Math.round((now - new Date(value).getTime()) / 1000);
  if (seconds < 60) return 'just now';
  if (seconds < 3600) return `${count(Math.floor(seconds / 60), 'minute')} ago`;
  if (seconds < 86_400) return `${count(Math.floor(seconds / 3600), 'hour')} ago`;

  return `${count(Math.floor(seconds / 86_400), 'day')} ago`;
}

/** The exact instant a relative time is being relative about, said both ways. */
function instant(value: string, item: QueueItem): { exact: string; iso: string } {
  return { exact: absolute(value, item.profile_timezone), iso: value };
}

/**
 * What the row says about itself: what state it is in, why, and what happens next.
 *
 * One relative time per row, and the exact stamp rides that time's own tooltip - a
 * queue read at a glance is read in "in about four minutes", and a queue reasoned
 * about is read in a timestamp. Both, in two places, is what makes a row unreadable.
 *
 * A wait is said as a wait rather than as the state the service files it under: the
 * reason is what a reader can act on, and "Blocked" is a word about the queue.
 */
export function queueLine(item: QueueItem, now: number): QueueLine {
  const detail = item.summary ?? words(item.kind);
  const next = { relative: countdown(item.eligible_at, now), ...instant(item.eligible_at, item) };
  switch (item.state) {
    case 'awaiting_approval':
      return { lead: `${detail} · waiting for somebody to decide` };
    case 'running':
      return {
        lead:
          item.progress_total > 0
            ? `Running · ${item.progress_current} of ${item.progress_total} changes written`
            : 'Running',
      };
    case 'blocked':
      return { lead: `${item.blocked_reason ?? 'Waiting on something else'} · runs`, when: next };
    case 'retrying':
      return {
        lead: `${item.blocked_reason ?? `Attempt ${item.attempt} did not finish`} · tries again`,
        when: next,
        tail: ', on its own',
      };
    case 'succeeded':
    case 'failed':
    case 'cancelled':
    case 'superseded': {
      const finished = item.finished_at ?? item.updated_at;
      return {
        lead: `${detail} ·`,
        when: { relative: ago(finished, now), ...instant(finished, item) },
      };
    }
    default:
      return { lead: `${detail} · runs`, when: next };
  }
}
