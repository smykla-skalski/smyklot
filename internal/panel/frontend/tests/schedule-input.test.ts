import { describe, expect, it } from 'vitest';
import { scheduleMinute, scheduleMinuteText } from '../src/lib/schedule-input';

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
    expect(scheduleMinuteText(minutes)).toBe(value);
  });
});
