import { expect, it } from 'vitest';
import { queueActionLabel, queueLine } from '../src/lib/queue-words';
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

it('names retry and recurring actions by the occurrence they change', () => {
  const item = queueSeeds(() => '2026-09-14T12:00:00Z')[3]!;
  expect(queueActionLabel('run_now', item)).toBe('Retry now');
  expect(queueActionLabel('run_now', { ...item, state: 'scheduled' })).toBe(
    'Run next occurrence now',
  );
  expect(queueActionLabel('cancel', item)).toBe('Cancel this occurrence');
  expect(queueActionLabel('run_now', { ...item, state: 'ready', source_kind: undefined })).toBe(
    'Run now',
  );
});

it('does not promise an execution time while dependencies block work', () => {
  const item = queueSeeds(() => '2026-09-14T12:00:00Z')[1]!;
  const line = queueLine(item, Date.parse(item.eligible_at));
  expect(line.lead).toContain('Waiting on required checks');
  expect(line.lead).toContain('start time not confirmed');
  expect(line.when).toBeUndefined();
});
it.each(['scheduled', 'ready', 'retrying'] as const)(
  'qualifies %s eligibility with worker availability',
  (state) => {
    const item = queueSeeds(() => '2026-09-14T12:00:00Z')[3]!;
    const line = queueLine({ ...item, state }, Date.parse(item.eligible_at) - 120000);
    expect(line.lead).toContain('can start');
    expect(line.when?.relative).toBe('in 2 minutes');
    expect(line.tail).toContain('when a worker is available');
  },
);
