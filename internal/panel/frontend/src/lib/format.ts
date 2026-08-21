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

/** Render a compact local date without the time — decision-level precision.
 * Day-first ("2 Aug 2026"), as the approved dialogs set their dates. */
export function formatDate(value: string): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return value;
  }
  return new Date(parsed).toLocaleDateString('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  });
}

/** Render a compact local date and time without machine-oriented seconds. */
export function formatDateTime(value: string): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return value;
  }
  return new Date(parsed).toLocaleString(undefined, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

/** How far in the past a timestamp is, in the coarsest unit that still says it. */
export type RelativeBucket =
  | { kind: 'just-now' }
  | { kind: 'minutes'; minutes: number }
  | { kind: 'hours'; hours: number }
  | { kind: 'days'; days: number }
  | { kind: 'date' };

/**
 * Place an age in a bucket.
 *
 * Elapsed time only, never calendar days: "yesterday" depends on the zone the
 * page is read in, so the same stored stamp would bucket differently for the
 * owner and for the person it belongs to. "2 days ago" is elapsed time - 48
 * whole hours whatever the zone - which is why days get a bucket and
 * "yesterday" still does not. A week out, the date says more than a count.
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
  if (elapsed < 7 * DAY) {
    return { kind: 'days', days: Math.floor(elapsed / DAY) };
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
    case 'days':
      return `${bucket.days} ${plural(bucket.days, 'day')} ago`;
    case 'date':
      /* Day-first, like every other date in the product - `undefined` here
         handed an en-US reader "Aug 9, 2026" beside a "9 Aug 2026" written by
         formatDate two lines up the same dialog. */
      return formatDate(value);
  }
}

/**
 * Say how soon a deadline arrives, or give the date once "in N hours" stops
 * helping. The future mirror of {@link formatRelative}: a deadline already
 * behind the reader's clock reads "now" rather than counting up.
 */
export function formatUntil(value: string, nowMs: number): string {
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) {
    return value;
  }
  const left = parsed - nowMs;
  if (left < PRESENT_MS) {
    return 'now';
  }
  if (left < HOUR) {
    const minutes = Math.max(1, Math.floor(left / MINUTE));
    return `in ${minutes} ${plural(minutes, 'minute')}`;
  }
  if (left < DAY) {
    const hours = Math.floor(left / HOUR);
    return `in ${hours} ${plural(hours, 'hour')}`;
  }
  if (left < 14 * DAY) {
    const days = Math.floor(left / DAY);
    return `in ${days} ${plural(days, 'day')}`;
  }
  return formatDate(value);
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

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

/**
 * Render a byte count against a volume it has to stay inside.
 *
 * Decimal units, because that is what a hosting provider sells a volume in: a
 * database reported in GiB beside a 3 GB volume reads as more headroom than
 * there is. Three significant figures, so `1.05 GB` and `1.10 GB` are different
 * numbers rather than the same rounded one.
 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }
  let value = bytes;
  let unit = 0;
  while (value >= 1_000 && unit < BYTE_UNITS.length - 1) {
    value /= 1_000;
    unit += 1;
  }
  // Bytes are whole things; only a scaled unit has a fraction to show.
  const digits = unit === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(digits)} ${BYTE_UNITS[unit]}`;
}

/**
 * Render a database round trip. Sub-millisecond answers keep two decimals,
 * because the difference between `0.30 ms` and `0.90 ms` is the difference
 * between a socket next door and one across a region.
 */
export function formatLatency(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) {
    return '—';
  }
  if (ms >= 100) {
    return `${Math.round(ms)} ms`;
  }
  return `${ms.toFixed(ms >= 10 ? 1 : 2)} ms`;
}

/**
 * Render a total that has been accumulating since the service started, which
 * can be anything from a few milliseconds to hours. Unlike a round trip, the
 * useful precision here shrinks as the number grows.
 */
export function formatElapsed(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) {
    return '0 ms';
  }
  if (ms < 1_000) {
    return `${Math.round(ms)} ms`;
  }
  const seconds = ms / 1_000;
  if (seconds < 60) {
    return `${seconds.toFixed(seconds < 10 ? 1 : 0)} s`;
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ${Math.round(seconds - minutes * 60)}s`;
  }
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

function plural(count: number, unit: string): string {
  return count === 1 ? unit : `${unit}s`;
}

function pad(value: number): string {
  return value.toString().padStart(2, '0');
}
