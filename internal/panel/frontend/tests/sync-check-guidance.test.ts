import { describe, expect, it } from 'vitest';
import {
  syncCheckIntent,
  syncCheckBlocker,
  syncCheckGuidance,
} from '../src/lib/sync-check-guidance';
import type { SyncCheckCapability } from '../src/lib/types';
const available: SyncCheckCapability = {
  action: 'check',
  target_id: 'target',
  available: true,
  reason: 'available',
  effect: 'request_repository_check',
};
describe('check recovery intent [Unit]', () => {
  it('creates only a check intent for the current workspace', () => {
    expect(syncCheckIntent(available, 'target')).toEqual({ action: 'check' });
    expect(syncCheckGuidance(available, 'target')).toContain('Automatic sync settings still apply');
  });
  it.each([
    undefined,
    { ...available, target_id: 'other' },
    { ...available, available: false },
    { ...available, action: 'dispatch' },
    { ...available, effect: 'apply' },
  ])('refuses incomplete or inconsistent capability %j', (input) => {
    expect(syncCheckIntent(input as SyncCheckCapability | undefined, 'target')).toBeNull();
  });
  it('keeps blocker identity scoped to its reason and workspace', () => {
    const capability: SyncCheckCapability = {
      ...available,
      available: false,
      reason: 'changes_pending',
      blocking_plan_id: 'newer',
    };
    expect(syncCheckBlocker(capability, 'target')).toEqual({ kind: 'plan', id: 'newer' });
    expect(syncCheckBlocker(capability, 'other')).toBeNull();
    expect(
      syncCheckBlocker({ ...capability, reason: 'admin_or_owner_required' }, 'target'),
    ).toBeNull();
    expect(
      syncCheckBlocker(
        { ...capability, reason: 'check_running', running_check_id: 'scan' },
        'target',
      ),
    ).toEqual({ kind: 'check', id: 'scan' });
  });
});
