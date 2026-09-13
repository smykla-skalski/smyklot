import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startPanel, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('development check acceptance HTTP contract', () => {
  it('returns identity-only receipts and structured missing-plan errors', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    try {
      const endpoint = `${panel.origin}/api/v1/targets/1001/sync/run-now`;
      const data = {
        action: 'check',
        request_key: 'http-check-receipt',
        reason: 'Verify original request',
      };
      for (const request_key of [undefined, null, '', ' ', 'ą'.repeat(101)]) {
        const rejected = await page.request.post(endpoint, { data: { ...data, request_key } });
        expect(rejected.status()).toBe(400);
        expect(await rejected.json()).toMatchObject({ error: { code: 'invalid_request' } });
      }
      const first = await page.request.post(endpoint, { data });
      expect(first.status()).toBe(202);
      const accepted = await first.json();
      expect(accepted).toEqual({ status: 'check_accepted', check_id: expect.any(String) });
      const second = await page.request.post(endpoint, { data });
      expect(second.status()).toBe(200);
      expect(await second.json()).toEqual({ ...accepted, repeated: true });
      const changed = await page.request.post(endpoint, {
        data: { ...data, reason: 'Changed input' },
      });
      expect(changed.status()).toBe(409);
      expect(await changed.json()).toMatchObject({ error: { code: 'conflict' } });
      const missing = await page.request.post(endpoint, {
        data: {
          action: 'dispatch',
          request_key: 'dispatch-request',
          plan_id: 'missing-plan',
          expected_revision: 1,
          reason: 'Dispatch original',
        },
      });
      expect(missing.status()).toBe(404);
      expect(await missing.json()).toMatchObject({ error: { code: 'not_found' } });
    } finally {
      await page.close();
    }
  });
});
