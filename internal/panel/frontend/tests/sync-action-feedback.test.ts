import { describe, expect, it } from 'vitest';
import { syncActionFeedback } from '../src/lib/sync-action-feedback';
import type { SyncAction } from '../src/lib/types';

const action = (fields: Partial<SyncAction>): SyncAction => ({
  repository: 'api',
  kind: 'labels',
  operation: 'create',
  subject: 'bug',
  state: 'pending',
  ...fields,
});

describe('recorded action feedback', () => {
  it('identifies failure even when details are absent or blank', () => {
    for (const error of [undefined, '', '   '])
      expect(syncActionFeedback(action({ state: 'failed', error }))).toBe(
        'Failed: No failure details were recorded.',
      );
    expect(syncActionFeedback(action({ state: 'failed', error: ' Permission denied ' }))).toBe(
      'Failed: Permission denied',
    );
  });
  it('distinguishes a failed dependency from a descriptive skip reason', () => {
    expect(syncActionFeedback(action({ state: 'skipped', blocker: 'labels' }))).toBe(
      'Skipped because labels failed earlier in this repository.',
    );
    expect(
      syncActionFeedback(
        action({ state: 'skipped', blocker: 'Sync is disabled for this category.' }),
      ),
    ).toBe('Skipped: Sync is disabled for this category.');
    expect(syncActionFeedback(action({ state: 'skipped', blocker: 'toString' }))).toBe(
      'Skipped: toString',
    );
  });
  it('does not hide skipped work when its reason is missing', () => {
    expect(syncActionFeedback(action({ state: 'skipped', blocker: ' ' }))).toBe(
      'Skipped: No reason was recorded.',
    );
    expect(syncActionFeedback(action({ state: 'skipped', error: 'Dependency unavailable' }))).toBe(
      'Skipped: Dependency unavailable',
    );
  });
  it('does not imply that a completed file proposal was merged', () => {
    expect(
      syncActionFeedback(
        action({
          kind: 'files',
          state: 'applied',
          proposal_url: 'https://github.com/team/repo/pull/1',
        }),
      ),
    ).toBe('Proposed in a pull request');
    expect(syncActionFeedback(action({ kind: 'files', state: 'applied', proposal_url: ' ' }))).toBe(
      'Completed; no pull request link recorded',
    );
  });

  it('does not invent a failure for pending or successful work', () => {
    expect(syncActionFeedback(action({}))).toBe('Pending');
    expect(syncActionFeedback(action({ state: 'applied' }))).toBe('Succeeded');
    expect(syncActionFeedback(action({ error: 'Existing diagnostic' }))).toBe(
      'Existing diagnostic',
    );
  });
});
