import { describe, expect, it } from 'vitest';

import {
  formatCountdown,
  formatRelative,
  formatTimestamp,
  relativeBucket,
  remainingFraction,
  remainingMs,
} from '../src/lib/format';

const SECOND = 1_000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

describe('formatTimestamp', () => {
  it('shows a value the browser cannot parse verbatim', () => {
    expect(formatTimestamp('not a timestamp')).toBe('not a timestamp');
  });

  it('renders a parseable timestamp in the reader zone', () => {
    expect(formatTimestamp('2026-07-26T14:01:01Z')).not.toBe('2026-07-26T14:01:01Z');
  });
});

// The buckets are elapsed-time only, never calendar days, so the same input
// lands in the same bucket in every time zone the panel is read in.
describe('relativeBucket', () => {
  it('treats the last few seconds as the present', () => {
    expect(relativeBucket(0, 20 * SECOND)).toEqual({ kind: 'just-now' });
  });

  it('counts whole minutes below an hour', () => {
    expect(relativeBucket(0, 5 * MINUTE)).toEqual({ kind: 'minutes', minutes: 5 });
    expect(relativeBucket(0, 59 * MINUTE + 59 * SECOND)).toEqual({ kind: 'minutes', minutes: 59 });
  });

  // The gap between the present bucket and a whole minute would otherwise floor
  // to zero and read as "0 minutes ago".
  it('never reports a count of zero', () => {
    for (let elapsed = 0; elapsed <= DAY; elapsed += SECOND) {
      const bucket = relativeBucket(0, elapsed);
      if (bucket.kind === 'minutes') {
        expect(bucket.minutes).toBeGreaterThan(0);
      }
      if (bucket.kind === 'hours') {
        expect(bucket.hours).toBeGreaterThan(0);
      }
    }
  });

  it('counts whole hours below a day', () => {
    expect(relativeBucket(0, 3 * HOUR)).toEqual({ kind: 'hours', hours: 3 });
    expect(relativeBucket(0, 23 * HOUR + 59 * MINUTE)).toEqual({ kind: 'hours', hours: 23 });
  });

  it('falls back to a date a day out or more', () => {
    expect(relativeBucket(0, DAY)).toEqual({ kind: 'date' });
  });

  // A last-seen stamp ahead of the reader's clock means skew, not the future,
  // and counting up from it would print a negative age.
  it('treats a stamp ahead of now as the present', () => {
    expect(relativeBucket(10 * MINUTE, 0)).toEqual({ kind: 'just-now' });
  });
});

describe('formatRelative', () => {
  const now = Date.parse('2026-07-26T14:00:00Z');

  it('shows a value the browser cannot parse verbatim', () => {
    expect(formatRelative('not a timestamp', now)).toBe('not a timestamp');
  });

  it('names the present', () => {
    expect(formatRelative('2026-07-26T13:59:50Z', now)).toBe('just now');
  });

  it('singularises one unit', () => {
    expect(formatRelative('2026-07-26T13:59:00Z', now)).toBe('1 minute ago');
    expect(formatRelative('2026-07-26T13:00:00Z', now)).toBe('1 hour ago');
  });

  it('rounds the gap below a whole minute up to one', () => {
    expect(formatRelative('2026-07-26T13:59:10Z', now)).toBe('1 minute ago');
  });

  it('pluralises the rest', () => {
    expect(formatRelative('2026-07-26T13:45:00Z', now)).toBe('15 minutes ago');
    expect(formatRelative('2026-07-26T09:00:00Z', now)).toBe('5 hours ago');
  });

  // Derived from the same API the panel renders with rather than asserting a
  // literal: a runner under a non-Gregorian calendar or Arabic-Indic digits
  // renders this date without the substring "2026" in it anywhere.
  it('drops to a date beyond a day', () => {
    const stamp = '2026-07-01T09:00:00Z';
    const rendered = formatRelative(stamp, now);
    expect(rendered).not.toMatch(/ago$/);
    expect(rendered).toBe(
      new Date(Date.parse(stamp)).toLocaleDateString(undefined, {
        day: 'numeric',
        month: 'short',
        year: 'numeric',
      }),
    );
  });
});

describe('remainingMs', () => {
  const now = Date.parse('2026-07-26T14:00:00Z');

  it('reports null for a value it cannot parse', () => {
    expect(remainingMs('never', now)).toBeNull();
  });

  it('measures the gap to the deadline', () => {
    expect(remainingMs('2026-07-26T14:30:00Z', now)).toBe(30 * MINUTE);
  });

  it('clamps a passed deadline to zero rather than counting up', () => {
    expect(remainingMs('2026-07-26T13:30:00Z', now)).toBe(0);
  });
});

describe('formatCountdown', () => {
  it('shows a spent link as zero', () => {
    expect(formatCountdown(0)).toBe('0:00');
    expect(formatCountdown(-5 * MINUTE)).toBe('0:00');
  });

  // Rounding up keeps the last second on screen: a countdown that reads 0:00
  // while the link still works would send someone to generate another.
  it('rounds a part second up', () => {
    expect(formatCountdown(500)).toBe('0:01');
    expect(formatCountdown(74 * SECOND + 500)).toBe('1:15');
  });

  it('pads seconds to two digits', () => {
    expect(formatCountdown(29 * MINUTE + 14 * SECOND)).toBe('29:14');
    expect(formatCountdown(9 * SECOND)).toBe('0:09');
  });

  it('adds an hours field only when there are hours left', () => {
    expect(formatCountdown(59 * MINUTE + 59 * SECOND)).toBe('59:59');
    expect(formatCountdown(HOUR)).toBe('1:00:00');
    expect(formatCountdown(HOUR + 2 * MINUTE + 33 * SECOND)).toBe('1:02:33');
  });
});

describe('remainingFraction', () => {
  it('runs from full at issue to empty at expiry', () => {
    expect(remainingFraction(0, 100, 0)).toBe(1);
    expect(remainingFraction(0, 100, 25)).toBe(0.75);
    expect(remainingFraction(0, 100, 100)).toBe(0);
  });

  it('clamps outside the window', () => {
    expect(remainingFraction(50, 100, 10)).toBe(1);
    expect(remainingFraction(0, 100, 500)).toBe(0);
  });

  // A deadline at or before issue leaves nothing to drain, and dividing by that
  // window would put Infinity or NaN into a CSS width.
  it('reports empty for a window with no length', () => {
    expect(remainingFraction(100, 100, 100)).toBe(0);
    expect(remainingFraction(100, 50, 100)).toBe(0);
  });

  // The width this feeds is set from a deadline the panel did not parse, so a
  // deadline it could not read has to land on a real number here.
  it('reports empty for a deadline that is not a number', () => {
    expect(remainingFraction(0, Number.NaN, 50)).toBe(0);
    expect(remainingFraction(Number.NaN, 100, 50)).toBe(0);
  });
});
