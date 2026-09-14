import { expect, it } from 'vitest';
import { resolveQueueTimeFixture } from '../stories/support/queue-time';

it.each([
  ['UTC', '2026-08-24T02:30:00.000Z'],
  ['Europe/Warsaw', '2026-08-24T00:30:00.000Z'],
  ['Asia/Tokyo', '2026-08-23T17:30:00.000Z'],
])('keeps the catalogue wall time in %s', async (timezone, expected) => {
  const result = await resolveQueueTimeFixture(timezone, '2026-08-24T02:30');
  expect(result.options).toHaveLength(1);
  expect(result.options[0]?.at).toBe(expected);
  expect(Date.parse(result.options[0]!.local_time)).toBe(Date.parse(expected));
});

it.each([
  ['Europe/Warsaw', '2026-10-25T02:30'],
  ['Europe/Warsaw', '2026-08-32T02:30'],
  ['UTC', '2026-08-24T24:30'],
  ['Pacific/Auckland', '2026-08-24T02:30'],
])('refuses to invent catalogue results for %s %s', async (timezone, localTime) => {
  await expect(resolveQueueTimeFixture(timezone, localTime)).rejects.toThrow(
    'This example covers August 2026',
  );
});
