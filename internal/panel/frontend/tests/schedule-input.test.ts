import { describe, expect, it } from 'vitest';
import { parseScheduleExceptions, scheduleMinute } from '../src/lib/schedule-input';

describe('schedule input preservation [Unit]', () => {
  it.each(['', '9:00', '09:60', '23:99', '24:01', '25:00', '09:00:30', '1e1:00', '-1:00'])(
    'does not normalize malformed time %s',
    (value) => {
      expect(scheduleMinute(value)).toBeNaN();
    },
  );
  it.each([
    ['00:00', 0],
    ['09:05', 545],
    ['23:59', 1439],
    ['24:00', 1440],
  ] as const)('preserves %s', (value, minutes) => {
    expect(scheduleMinute(value)).toBe(minutes);
  });
  it('preserves closed dates and full-day exception endpoints', () => {
    expect(parseScheduleExceptions('2026-12-25 closed\n\n2026-12-31 00:00-24:00')).toEqual([
      { date: '2026-12-25', closed: true },
      { date: '2026-12-31', closed: false, start_minute: 0, end_minute: 1440 },
    ]);
  });
  it.each([
    '2026-12-25',
    '2026-12-25 closed ignored',
    '2026-12-25 09:00-10:00-extra',
    '2026-12-25 09:60-11:00',
    '2026-02-30 closed',
    '2026-12-25 24:00-24:00',
  ])('refuses silently lossy exception %s', (line) => {
    expect(() => parseScheduleExceptions(`\n${line}`)).toThrow('line 2:');
  });
  it('reports the original line for conflicting rules without losing blank lines', () => {
    expect(() => parseScheduleExceptions('\n2026-12-25 closed\n\n2026-12-25 09:00-10:00')).toThrow(
      'line 2:',
    );
  });
});
