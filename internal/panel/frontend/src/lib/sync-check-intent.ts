export interface SyncCheckIntent {
  action: 'check';
  request_key: string;
  reason: string;
}
type IntentStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

/** A pending command outlives its view and survives reload in the same browser tab. */
export class SyncCheckIntentStore {
  private readonly address: string;
  constructor(
    private readonly storage: IntentStorage,
    actorId: string,
    targetId: string,
  ) {
    if (!actorId || !targetId)
      throw new Error('Your account and workspace must be loaded before requesting a check.');
    this.address = `smyklot.sync-check.v1:${JSON.stringify([actorId, targetId])}`;
  }
  read(): SyncCheckIntent | null {
    const raw = this.storage.getItem(this.address);
    if (raw === null) return null;
    const value: unknown = JSON.parse(raw);
    if (
      !value ||
      typeof value !== 'object' ||
      !('action' in value) ||
      value.action !== 'check' ||
      !('request_key' in value) ||
      typeof value.request_key !== 'string' ||
      !value.request_key.trim() ||
      value.request_key.trim() !== value.request_key ||
      new TextEncoder().encode(value.request_key).length > 200 ||
      !('reason' in value) ||
      typeof value.reason !== 'string' ||
      !value.reason.trim()
    ) {
      throw new Error(
        'The saved check request could not be read. Keep this browser data so the request can be recovered.',
      );
    }
    return { action: 'check', request_key: value.request_key, reason: value.reason };
  }
  begin(reason: string): SyncCheckIntent {
    const pending = this.read();
    if (pending) return pending;
    const intent: SyncCheckIntent = {
      action: 'check',
      request_key: crypto.randomUUID(),
      reason: reason.trim(),
    };
    if (!intent.reason) throw new Error('A reason is required for the check.');
    this.storage.setItem(this.address, JSON.stringify(intent));
    return intent;
  }
  clear(requestKey: string): void {
    if (this.read()?.request_key === requestKey) this.storage.removeItem(this.address);
  }
}
