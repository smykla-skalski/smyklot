import { describe, expect, it } from 'vitest';

import {
  formatBytes,
  formatCountdown,
  formatDate,
  formatDateTime,
  formatElapsed,
  formatLatency,
  formatRelative,
  formatTimestamp,
  formatUntil,
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

describe('formatDateTime', () => {
  it('shows a value the browser cannot parse verbatim', () => {
    expect(formatDateTime('not a timestamp')).toBe('not a timestamp');
  });

  it('renders a compact local date and time', () => {
    const stamp = '2026-07-26T14:01:01Z';
    expect(formatDateTime(stamp)).toBe(
      new Date(Date.parse(stamp)).toLocaleString(undefined, {
        day: 'numeric',
        month: 'short',
        year: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
      }),
    );
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
  // renders this date without the substring "2026" in it anywhere. It is
  // formatDate specifically, so the fallback is day-first like every other
  // date the panel prints rather than whatever the runner's locale prefers.
  it('drops to a date beyond a day', () => {
    const stamp = '2026-07-01T09:00:00Z';
    const rendered = formatRelative(stamp, now);
    expect(rendered).not.toMatch(/ago$/);
    expect(rendered).toBe(formatDate(stamp));
  });
});

describe('formatUntil', () => {
  const now = Date.parse('2026-07-26T14:00:00Z');

  it('shows a value the browser cannot parse verbatim', () => {
    expect(formatUntil('not a timestamp', now)).toBe('not a timestamp');
  });

  it('treats a passed or imminent deadline as now', () => {
    expect(formatUntil('2026-07-26T13:00:00Z', now)).toBe('now');
    expect(formatUntil('2026-07-26T14:00:30Z', now)).toBe('now');
  });

  it('counts minutes, hours, and days with singular forms', () => {
    expect(formatUntil('2026-07-26T14:01:30Z', now)).toBe('in 1 minute');
    expect(formatUntil('2026-07-26T14:45:00Z', now)).toBe('in 45 minutes');
    expect(formatUntil('2026-07-26T15:30:00Z', now)).toBe('in 1 hour');
    expect(formatUntil('2026-07-26T19:00:00Z', now)).toBe('in 5 hours');
    expect(formatUntil('2026-07-27T15:00:00Z', now)).toBe('in 1 day');
    expect(formatUntil('2026-08-01T14:00:00Z', now)).toBe('in 6 days');
  });

  // Same rationale as formatRelative's date fallback: assert via the API,
  // not a literal rendering.
  it('drops to a date two weeks out or more', () => {
    const stamp = '2026-08-20T14:00:00Z';
    const rendered = formatUntil(stamp, now);
    expect(rendered).not.toMatch(/^in /);
    expect(rendered).toBe(formatDate(stamp));
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

describe('formatBytes', () => {
  it('leaves a byte count whole', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(512)).toBe('512 B');
  });

  // Decimal units, because a hosting provider sells a volume in them. Reporting
  // a database in GiB beside a 3 GB volume reads as more headroom than exists.
  it('scales on thousands, not on 1024', () => {
    expect(formatBytes(1_000)).toBe('1.00 KB');
    expect(formatBytes(1_024)).toBe('1.02 KB');
    expect(formatBytes(84_711_103)).toBe('84.7 MB');
    expect(formatBytes(2_400_000_000)).toBe('2.40 GB');
  });

  // Three significant figures throughout, so growth inside a unit stays visible
  // rather than rounding to the same number for weeks.
  it('keeps three figures as the value grows within a unit', () => {
    expect(formatBytes(1_050_000_000)).toBe('1.05 GB');
    expect(formatBytes(12_300_000)).toBe('12.3 MB');
    expect(formatBytes(123_000_000)).toBe('123 MB');
  });

  it('reports nothing for a size the engine could not give', () => {
    expect(formatBytes(-1)).toBe('0 B');
    expect(formatBytes(Number.NaN)).toBe('0 B');
  });
});

describe('formatLatency', () => {
  // The difference between a socket next door and one across a region lives in
  // the hundredths, so a sub-millisecond round trip keeps them.
  it('keeps two decimals under ten milliseconds', () => {
    expect(formatLatency(0.3)).toBe('0.30 ms');
    expect(formatLatency(1.24)).toBe('1.24 ms');
  });

  it('drops precision as the number grows', () => {
    expect(formatLatency(12.34)).toBe('12.3 ms');
    expect(formatLatency(148.6)).toBe('149 ms');
  });

  it('reports nothing measurable for a latency it was not given', () => {
    expect(formatLatency(Number.NaN)).toBe('—');
    expect(formatLatency(-1)).toBe('—');
  });
});

describe('formatElapsed', () => {
  it('reports a total that has never accumulated', () => {
    expect(formatElapsed(0)).toBe('0 ms');
    expect(formatElapsed(Number.NaN)).toBe('0 ms');
  });

  // A total since start spans milliseconds to hours, and the useful precision
  // shrinks as it grows: nobody needs a wait of two hours to the millisecond.
  it('steps units as the total grows', () => {
    expect(formatElapsed(41)).toBe('41 ms');
    expect(formatElapsed(4_100)).toBe('4.1 s');
    expect(formatElapsed(41_000)).toBe('41 s');
    expect(formatElapsed(125_000)).toBe('2m 5s');
    expect(formatElapsed(7_320_000)).toBe('2h 2m');
  });
});
