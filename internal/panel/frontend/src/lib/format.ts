/**
 * Rendering timestamps and deadlines for a person reading the page.
 *
 * The panel stores timestamps in UTC and the browser knows the reader's zone, so
 * the conversion belongs here rather than in the API.
 */

const SECOND = 1_000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** Below this an age is the present rather than a duration worth counting. */
const PRESENT_MS = 45 * SECOND;

/**
 * Render an RFC 3339 timestamp in full.
 *
 * A value the browser cannot parse is shown verbatim: an unreadable timestamp is
 * still a fact about the account, and `Invalid Date` would hide it.
 */
export function formatTimestamp(value: string): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return value;
  }
  return new Date(parsed).toLocaleString();
}

/** How far in the past a timestamp is, in the coarsest unit that still says it. */
export type RelativeBucket =
  | { kind: 'just-now' }
  | { kind: 'minutes'; minutes: number }
  | { kind: 'hours'; hours: number }
  | { kind: 'date' };

/**
 * Place an age in a bucket.
 *
 * Elapsed time only, never calendar days: "yesterday" depends on the zone the
 * page is read in, so the same stored stamp would bucket differently for the
 * owner and for the person it belongs to.
 */
export function relativeBucket(fromMs: number, nowMs: number): RelativeBucket {
  // A stamp ahead of the reader's clock is skew, not the future. Counting up
  // from it would print a negative age.
  const elapsed = Math.max(0, nowMs - fromMs);
  if (elapsed < PRESENT_MS) {
    return { kind: 'just-now' };
  }
  if (elapsed < HOUR) {
    // The stretch between the present bucket and a whole minute floors to zero,
    // and "0 minutes ago" is not something anybody means to write. The hours
    // bucket needs no such guard: it starts at exactly one hour.
    return { kind: 'minutes', minutes: Math.max(1, Math.floor(elapsed / MINUTE)) };
  }
  if (elapsed < DAY) {
    return { kind: 'hours', hours: Math.floor(elapsed / HOUR) };
  }
  return { kind: 'date' };
}

/** Say how long ago something happened, or give the date once that stops helping. */
export function formatRelative(value: string, nowMs: number): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return value;
  }
  const bucket = relativeBucket(parsed, nowMs);
  switch (bucket.kind) {
    case 'just-now':
      return 'just now';
    case 'minutes':
      return `${bucket.minutes} ${plural(bucket.minutes, 'minute')} ago`;
    case 'hours':
      return `${bucket.hours} ${plural(bucket.hours, 'hour')} ago`;
    case 'date':
      return new Date(parsed).toLocaleDateString(undefined, {
        day: 'numeric',
        month: 'short',
        year: 'numeric',
      });
  }
}

/**
 * How long is left before a deadline, or `null` for one that cannot be read. A
 * passed deadline is zero rather than negative: every caller treats "no time
 * left" the same and none of them count up.
 */
export function remainingMs(expiresAt: string, nowMs: number): number | null {
  const parsed = Date.parse(expiresAt);
  if (Number.isNaN(parsed)) {
    return null;
  }
  return Math.max(0, parsed - nowMs);
}

/**
 * Render a countdown as `m:ss`, or `h:mm:ss` once there are hours left.
 *
 * Part seconds round up so the last second stays on screen: a countdown reading
 * `0:00` beside a link that still works would send someone to generate another.
 */
export function formatCountdown(ms: number): string {
  const total = Math.ceil(Math.max(0, ms) / SECOND);
  const seconds = total % 60;
  const minutes = Math.floor(total / 60) % 60;
  const hours = Math.floor(total / 3_600);
  if (hours > 0) {
    return `${hours}:${pad(minutes)}:${pad(seconds)}`;
  }
  return `${minutes}:${pad(seconds)}`;
}

/**
 * How much of a lifetime is left, as `0` to `1`, for the drain track under a
 * link. A window with no length, or one measured from a deadline that could not
 * be read, reports empty: either would put `Infinity` or `NaN` into a CSS width.
 */
export function remainingFraction(issuedMs: number, expiresMs: number, nowMs: number): number {
  const window = expiresMs - issuedMs;
  if (!Number.isFinite(window) || window <= 0) {
    return 0;
  }
  return Math.min(1, Math.max(0, (expiresMs - nowMs) / window));
}

function plural(count: number, unit: string): string {
  return count === 1 ? unit : `${unit}s`;
}

function pad(value: number): string {
  return value.toString().padStart(2, '0');
}
