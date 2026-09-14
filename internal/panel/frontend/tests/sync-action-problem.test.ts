import { describe, expect, it } from 'vitest';
import { syncActionProblem } from '../src/lib/sync-action-problem';
import type { SyncAction } from '../src/lib/types';

const action = (fields: Partial<SyncAction>): SyncAction => ({
  repository: 'api',
  kind: 'labels',
  operation: 'create',
  subject: 'bug',
  state: 'pending',
  ...fields,
});

describe('recorded action problems', () => {
  it('identifies failure even when details are absent or blank', () => {
    for (const error of [undefined, '', '   '])
      expect(syncActionProblem(action({ state: 'failed', error }))).toBe(
        'Failed: No failure details were recorded.',
      );
    expect(syncActionProblem(action({ state: 'failed', error: ' Permission denied ' }))).toBe(
      'Failed: Permission denied',
    );
  });
  it('distinguishes a failed dependency from a descriptive skip reason', () => {
    expect(syncActionProblem(action({ state: 'skipped', blocker: 'labels' }))).toBe(
      'Skipped because labels failed earlier in this repository.',
    );
    expect(
      syncActionProblem(
        action({ state: 'skipped', blocker: 'Sync is disabled for this category.' }),
      ),
    ).toBe('Skipped: Sync is disabled for this category.');
    expect(syncActionProblem(action({ state: 'skipped', blocker: 'toString' }))).toBe(
      'Skipped: toString',
    );
  });
  it('does not hide skipped work when its reason is missing', () => {
    expect(syncActionProblem(action({ state: 'skipped', blocker: ' ' }))).toBe(
      'Skipped: No reason was recorded.',
    );
    expect(syncActionProblem(action({ state: 'skipped', error: 'Dependency unavailable' }))).toBe(
      'Skipped: Dependency unavailable',
    );
  });
  it('does not invent a failure for pending or successful work', () => {
    expect(syncActionProblem(action({}))).toBeNull();
    expect(syncActionProblem(action({ state: 'applied' }))).toBeNull();
    expect(syncActionProblem(action({ error: 'Existing diagnostic' }))).toBe('Existing diagnostic');
  });
});
