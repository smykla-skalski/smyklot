import { describe, expect, it } from 'vitest';

import { aroundTheClock, hoursPhrase, windowsSentence } from '#lib/schedule-words.js';
import type { ScheduleProfile } from '#lib/types.js';

function profile(
  windows: ScheduleProfile['windows'],
  exceptions: ScheduleProfile['exceptions'] = [],
  name = 'Europe business hours',
): ScheduleProfile {
  return {
    id: 'profile-1',
    name,
    timezone: 'Europe/Warsaw',
    system: false,
    revision: 1,
    windows,
    exceptions,
  };
}

const WEEKDAYS = [1, 2, 3, 4, 5].map((weekday) => ({
  weekday,
  start_minute: 8 * 60,
  end_minute: 18 * 60,
}));

const EVERY_DAY = [0, 1, 2, 3, 4, 5, 6].map((weekday) => ({
  weekday,
  start_minute: 0,
  end_minute: 24 * 60,
}));

describe('schedule words [Unit]', () => {
  it('reads a working week as one run', () => {
    expect(windowsSentence(profile(WEEKDAYS))).toBe('Mon to Fri, 8:00 to 18:00');
  });

  it('breaks a run where the hours change', () => {
    const windows = [...WEEKDAYS, { weekday: 6, start_minute: 10 * 60, end_minute: 14 * 60 }];
    expect(windowsSentence(profile(windows))).toBe(
      'Mon to Fri, 8:00 to 18:00; Sat, 10:00 to 14:00',
    );
  });

  it('counts exceptions rather than listing them', () => {
    const one = profile(WEEKDAYS, [{ date: '2026-09-30', closed: true }]);
    const two = profile(WEEKDAYS, [
      { date: '2026-09-30', closed: true },
      { date: '2026-10-01', closed: true },
    ]);
    expect(windowsSentence(one)).toBe('Mon to Fri, 8:00 to 18:00, plus one exception');
    expect(windowsSentence(two)).toBe('Mon to Fri, 8:00 to 18:00, plus 2 exceptions');
  });

  it('says a profile that never closes in words, whatever it is called', () => {
    const open = profile(EVERY_DAY, [], 'Always Open');
    expect(aroundTheClock(open)).toBe(true);
    expect(windowsSentence(open)).toBe('never closed');
    expect(hoursPhrase(open, 'the default hours')).toBe('around the clock');
  });

  it('names the profile a job runs in, and falls back to the id it was given', () => {
    expect(hoursPhrase(profile(WEEKDAYS), 'x')).toBe('during Europe business hours');
    expect(hoursPhrase(undefined, 'always-open')).toBe('during always-open');
  });

  it('says a profile with no windows is never open', () => {
    expect(windowsSentence(profile([]))).toBe('never open');
    expect(aroundTheClock(profile([]))).toBe(false);
  });
});
