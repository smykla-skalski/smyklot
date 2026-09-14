import { expect, it } from 'vitest';
import { parseLocalTimeResolution } from '../src/lib/schedule-local-time';
it('preserves the explicit absence of a skipped wall time', () => {
  const result = { timezone: 'Europe/Warsaw', local_time: '2026-03-29T02:30', options: [] };
  expect(parseLocalTimeResolution(result)).toEqual(result);
});
it.each([null, {}, { timezone: 'UTC', local_time: '2026-01-01T12:00', options: [{}] }])(
  'rejects malformed resolution %j',
  (value) => {
    expect(() => parseLocalTimeResolution(value)).toThrow();
  },
);
