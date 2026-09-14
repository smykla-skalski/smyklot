import { createPanelApi } from '../src/lib/api';
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

it('forwards cancellation and rejects a response for another wall time', async () => {
  const signal = new AbortController().signal;
  let wall = '2026-03-29T02:30';
  const api = createPanelApi('/panel', async (url, init) => {
    expect(new URL(url, 'http://localhost').searchParams.get('local_time')).toBe(
      '2026-03-29T02:30',
    );
    expect(init?.signal).toBe(signal);
    return new Response(
      JSON.stringify({ timezone: 'Europe/Warsaw', local_time: wall, options: [] }),
    );
  });
  expect((await api.resolveScheduleLocalTime('Europe/Warsaw', wall, signal)).options).toEqual([]);
  wall = '2026-03-30T02:30';
  await expect(
    api.resolveScheduleLocalTime('Europe/Warsaw', '2026-03-29T02:30', signal),
  ).rejects.toThrow('does not match');
});
