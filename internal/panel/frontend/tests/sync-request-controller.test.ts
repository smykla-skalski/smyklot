import { describe, expect, it, vi } from 'vitest';
import { PanelApiError } from '../src/lib/api';
import { SyncRequestController } from '../src/lib/sync-request-controller.svelte';
import type { SyncPlan, SyncRunNowResponse } from '../src/lib/types';

function fixture() {
  const saved = new Map<string, string>();
  let finish!: (response: SyncRunNowResponse) => void;
  let fail!: (cause: unknown) => void;
  const response = new Promise<SyncRunNowResponse>((resolve, reject) => {
    finish = resolve;
    fail = reject;
  });
  const send = vi.fn(() => response);
  const controller = new SyncRequestController('account', 'workspace', {
    runSyncNow: send,
    refresh: async () => {},
    storage: () => ({
      getItem: (key) => saved.get(key) ?? null,
      setItem: (key, value) => {
        saved.set(key, value);
      },
      removeItem: (key) => {
        saved.delete(key);
      },
    }),
  });
  controller.readRequestStorage();
  return { controller, saved, send, finish, fail };
}

const dispatch = {
  action: 'dispatch',
  plan_id: 'plan',
  expected_revision: 2,
  reason: 'Reviewed changes',
} as const;

function plan(id: string): SyncPlan {
  return {
    id,
    trigger: 'reconcile',
    state: 'computed',
    execution_stage: 'Waiting for approval',
    digest: 'digest',
    computed_at: '2026-09-14T10:00:00Z',
    expires_at: '2026-09-15T10:00:00Z',
    counts: { create: 1, update: 0, delete: 0 },
    actions: [],
  };
}

describe('sync request ownership [Unit]', () => {
  it('keeps a pending submission busy across remount reads without another POST', async () => {
    const { controller, finish, send, saved } = fixture();
    const submitted = controller.submit(dispatch);
    const original = controller.pendingRequest;
    controller.readRequestStorage();
    await controller.submit(dispatch);
    expect(controller.runningNow).toBe(true);
    expect(controller.requestUncertain).toBe(false);
    expect(controller.pendingRequest).toBe(original);
    expect(send).toHaveBeenCalledTimes(1);
    finish({ status: 'dispatch_accepted', plan_id: 'plan', queue_id: 'worker' });
    await submitted;
    expect(controller.localAcceptance).toEqual({ action: 'dispatch', key: original?.request_key });
    expect(controller.runningNow).toBe(false);
    expect(saved.size).toBe(0);
  });

  it.each(['approval_required', 'already_running'] as const)(
    'retains %s without inventing acceptance',
    async (status) => {
      const { controller, finish, saved } = fixture();
      const submitted = controller.submit(dispatch);
      controller.readRequestStorage();
      finish({ status, plan: plan('plan') });
      await submitted;
      expect(controller.relatedPlan).toEqual({
        id: 'plan',
        label: status === 'approval_required' ? 'Review changes' : 'View changes',
      });
      expect(controller.runNotice).not.toBe('');
      expect(controller.localAcceptance).toBeNull();
      expect(controller.pendingRequest).toBeNull();
      expect(controller.requestUncertain).toBe(false);
      expect(saved.size).toBe(0);
    },
  );

  it('retains the blocking plan when a check finds earlier changes', async () => {
    const { controller, finish } = fixture();
    const submitted = controller.submit({ action: 'check', reason: 'Check repositories' });
    finish({ status: 'changes_pending', plan: plan('earlier-plan') });
    await submitted;
    expect(controller.relatedPlan).toEqual({ id: 'earlier-plan', label: 'Review earlier changes' });
    expect(controller.localAcceptance).toBeNull();
  });

  it.each([undefined, '', 'other-plan'])(
    'preserves the original dispatch when its response identifies %s',
    async (id) => {
      const { controller, finish, saved } = fixture();
      const submitted = controller.submit(dispatch);
      const original = controller.pendingRequest;
      finish({ status: 'approval_required', ...(id === undefined ? {} : { plan: plan(id) }) });
      await submitted;
      expect(controller.pendingRequest).toBe(original);
      expect(controller.requestUncertain).toBe(true);
      expect(controller.relatedPlan).toBeNull();
      expect(saved.size).toBe(1);
    },
  );

  it.each([
    [409, 'stale_revision', false],
    [503, 'unavailable', true],
  ] as const)(
    'retains failure %i and its correct recovery state',
    async (status, code, uncertain) => {
      const { controller, fail, saved } = fixture();
      const submitted = controller.submit(dispatch);
      const original = controller.pendingRequest;
      controller.readRequestStorage();
      fail(new PanelApiError(status, code, 'Original failure'));
      await submitted;
      expect(controller.error).toBe('Original failure');
      expect(controller.localAcceptance).toBeNull();
      expect(controller.requestUncertain).toBe(uncertain);
      expect(controller.pendingRequest).toEqual(uncertain ? original : null);
      expect(saved.size).toBe(uncertain ? 1 : 0);
    },
  );
});
