import type { SyncRunNowInput, SyncRunNowIntent } from './types';
type IntentStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

/** Preserve one uncertain command per actor/workspace before sending any mutation. */
export class SyncRequestIntentStore {
  private readonly address: string;
  constructor(
    private readonly storage: IntentStorage,
    actorId: string,
    targetId: string,
  ) {
    if (!actorId || !targetId)
      throw new Error('Your account and workspace must be loaded before requesting sync.');
    // Keep the original key so an upgrade does not lose an uncertain check.
    this.address = `smyklot.sync-check.v1:${JSON.stringify([actorId, targetId])}`;
  }
  read(): SyncRunNowInput | null {
    const raw = this.storage.getItem(this.address);
    return raw === null ? null : parseIntent(JSON.parse(raw));
  }
  begin(input: SyncRunNowIntent): SyncRunNowInput {
    const pending = this.read();
    if (pending) return pending;
    const intent = parseIntent({
      ...input,
      request_key: crypto.randomUUID(),
      reason: input.reason.trim(),
    });
    this.storage.setItem(this.address, JSON.stringify(intent));
    return intent;
  }
  clear(requestKey: string): void {
    if (this.read()?.request_key === requestKey) this.storage.removeItem(this.address);
  }
}

function parseIntent(value: unknown): SyncRunNowInput {
  const invalid = () =>
    new Error(
      'The saved sync request could not be read. Keep this browser data so the request can be recovered.',
    );
  if (
    !value ||
    typeof value !== 'object' ||
    !('action' in value) ||
    !('request_key' in value) ||
    typeof value.request_key !== 'string' ||
    !value.request_key.trim() ||
    value.request_key.trim() !== value.request_key ||
    new TextEncoder().encode(value.request_key).length > 200 ||
    !('reason' in value) ||
    typeof value.reason !== 'string' ||
    !value.reason.trim()
  )
    throw invalid();
  const shared = { request_key: value.request_key, reason: value.reason };
  if (value.action === 'check' && !('plan_id' in value) && !('expected_revision' in value))
    return { action: 'check', ...shared };
  if (
    value.action !== 'dispatch' ||
    !('plan_id' in value) ||
    typeof value.plan_id !== 'string' ||
    !value.plan_id.trim() ||
    value.plan_id.trim() !== value.plan_id ||
    !('expected_revision' in value) ||
    typeof value.expected_revision !== 'number' ||
    !Number.isSafeInteger(value.expected_revision) ||
    value.expected_revision <= 0
  )
    throw invalid();
  return {
    action: 'dispatch',
    plan_id: value.plan_id,
    expected_revision: value.expected_revision,
    ...shared,
  };
}
