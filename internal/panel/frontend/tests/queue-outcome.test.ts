import { expect, it } from 'vitest';
import { queueLine } from '../src/lib/queue-words';
import { queueSeeds } from '../dev/fixtures';

it.each(['succeeded', 'failed', 'cancelled', 'superseded'] as const)(
  'describes the %s occurrence using its finish time',
  (state) => {
    const item = queueSeeds(() => '2026-09-14T12:00:00Z')[4]!;
    const result = queueLine(
      { ...item, state, updated_at: '2026-09-15T12:00:00Z' },
      Date.parse('2026-09-14T13:00:00Z'),
    );
    expect(result.lead.toLowerCase()).toContain(state);
    expect(result.when?.iso).toBe(item.finished_at);
    expect(result.when?.relative).toBe('1 hour ago');
  },
);

it('does not present an update time as a completed occurrence finish', () => {
  const item = queueSeeds(() => '2026-09-14T12:00:00Z')[4]!;
  const result = queueLine({ ...item, finished_at: undefined }, Date.now());
  expect(result.lead).toContain('finish time unavailable');
  expect(result.when).toBeUndefined();
});
