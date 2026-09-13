import { describe, expect, it } from 'vitest';
import { createPanelApi } from '../src/lib/api';
import { recoveryExplanation, recoveryRequest } from '../src/lib/delivery-recovery';

describe('delivery recovery contract', () => {
  it.each([false, true])(
    'uses the authorized route and preserves request identity (root=%s)',
    async (root) => {
      const calls: { url: string; init?: RequestInit }[] = [];
      const api = createPanelApi('', async (url, init) => {
        calls.push({ url, init });
        return new Response(
          JSON.stringify({ available: true, current_run_id: 3, revision: 2, reason: 'available' }),
          { status: 200 },
        );
      });
      const preview = await api.previewDeliveryRecovery('target/one', 'source/one', root);
      const input = recoveryRequest(preview, 'same-key');
      await api.retryDelivery('target/one', 'source/one', root, input);
      await api.retryDelivery('target/one', 'source/one', root, input);
      expect(calls[0]?.url).toContain(
        `/api/v1/${root ? 'root/workspaces' : 'targets'}/target%2Fone/deliveries/source%2Fone/recovery`,
      );
      expect(calls[1]?.init?.body).toBe(calls[2]?.init?.body);
      expect(JSON.parse(String(calls[1]?.init?.body))).toEqual({
        expected_run_id: 3,
        expected_revision: 2,
        request_key: 'same-key',
      });
    },
  );
  it('requires a current eligible run before creating a request', () => {
    expect(() =>
      recoveryRequest({ available: false, reason: 'source_changed', revision: 1 }, 'key'),
    ).toThrow();
    expect(recoveryExplanation('source_changed')).toContain('current comment');
    expect(recoveryExplanation('future_reason')).not.toContain('future_reason');
  });
});
