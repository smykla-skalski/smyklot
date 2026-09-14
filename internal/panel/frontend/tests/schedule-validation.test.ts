import { describe, expect, it } from 'vitest';
import { scheduleHoursProblems, validScheduleDate } from '../src/lib/schedule-validation';

const window = { weekday: 1, start_minute: 540, end_minute: 1020 };
const profile = { name: 'Release hours', windows: [window], exceptions: [] };

describe('schedule calendar validation [Unit]', () => {
  it.each([
    '2026-02-29',
    '1900-02-29',
    '2026-04-31',
    '2026-00-01',
    '2026-13-01',
    '2026-01-00',
    '26-01-01',
    '2026-1-1',
    '2026-01-01T00:00:00Z',
  ])('rejects impossible or malformed date %s', (date) => {
    expect(validScheduleDate(date)).toBe(false);
  });
  it.each(['2024-02-29', '2000-02-29', '2026-12-31', '0000-01-01'])(
    'accepts calendar date %s',
    (date) => {
      expect(validScheduleDate(date)).toBe(true);
    },
  );
  it.each([
    [-1, 60],
    [1440, 1500],
    [0, 1441],
    [60, 60],
    [120, 60],
    [0.5, 60],
    [0, 60.5],
    [NaN, 60],
    [0, Infinity],
  ])('rejects invalid minute range %s to %s', (start_minute, end_minute) => {
    expect(
      scheduleHoursProblems({ ...profile, windows: [{ ...window, start_minute, end_minute }] }),
    ).toContainEqual(expect.objectContaining({ field: 'windows', index: 0 }));
  });
  it('accepts adjacent windows and a full day without mutating their order', () => {
    const windows = [
      { ...window, start_minute: 600, end_minute: 1440 },
      { ...window, start_minute: 0, end_minute: 600 },
    ];
    const before = structuredClone(windows);
    expect(scheduleHoursProblems({ ...profile, windows })).toEqual([]);
    expect(windows).toEqual(before);
  });
  it('addresses every overlapping row while keeping different weekdays independent', () => {
    const windows = [
      window,
      { ...window, start_minute: 600, end_minute: 700 },
      { ...window, weekday: 2 },
    ];
    expect(scheduleHoursProblems({ ...profile, windows }).map(({ index }) => index)).toEqual([
      0, 1,
    ]);
  });
  it('permits exception-only profiles and ignores unused closed-day minutes', () => {
    expect(
      scheduleHoursProblems({
        ...profile,
        windows: [],
        exceptions: [{ date: '2026-12-25', closed: true }],
      }),
    ).toEqual([]);
  });
  it('rejects a profile with no rules or name', () => {
    expect(
      scheduleHoursProblems({ name: ' ', windows: [], exceptions: [] }).map(({ field }) => field),
    ).toEqual(['name', 'windows']);
  });
  it('rejects closed/open mixtures and duplicate closed dates at both rows', () => {
    for (const closed of [true, false]) {
      const exceptions = [
        { date: '2026-12-25', closed: true },
        { date: '2026-12-25', closed, start_minute: 0, end_minute: 60 },
      ];
      expect(scheduleHoursProblems({ ...profile, exceptions }).map(({ index }) => index)).toEqual([
        0, 1,
      ]);
    }
  });
  it('accepts separate dates and adjacent exception ranges, but rejects overlaps', () => {
    const first = { date: '2026-12-25', closed: false, start_minute: 0, end_minute: 60 };
    expect(
      scheduleHoursProblems({
        ...profile,
        exceptions: [
          first,
          { ...first, start_minute: 60, end_minute: 120 },
          { ...first, date: '2026-12-26' },
        ],
      }),
    ).toEqual([]);
    expect(
      scheduleHoursProblems({ ...profile, exceptions: [first, { ...first, start_minute: 30 }] }),
    ).toHaveLength(2);
  });
  it('addresses invalid dates and open exception ranges', () => {
    expect(
      scheduleHoursProblems({ ...profile, exceptions: [{ date: '2026-02-30', closed: false }] }),
    ).toEqual([
      { field: 'exceptions', index: 0, message: 'Choose a valid calendar date' },
      {
        field: 'exceptions',
        index: 0,
        message: 'Closing time must be after opening time on the same day',
      },
    ]);
  });
});
