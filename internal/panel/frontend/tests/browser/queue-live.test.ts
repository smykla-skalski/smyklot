import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

let panel: Panel;
let viewer: Page;
let actor: Page;

beforeAll(async () => {
  panel = await startPanel();
  viewer = await panel.browser.newPage();
  actor = await panel.browser.newPage();
  await Promise.all([
    visit(viewer, `${panel.origin}/root/queue`, { ready: '.general-queue-table tbody .data-row' }),
    visit(actor, `${panel.origin}/root/queue`, { ready: '.general-queue-table tbody .data-row' }),
  ]);
}, 300_000);

afterAll(async () => {
  await viewer?.close();
  await actor?.close();
  await panel?.close();
});

describe('the general Queue live stream [Integration]', () => {
  it('refreshes another reader when an audited action changes an item', async () => {
    const title = 'Merge platform-infra#184 after CI';
    const viewerRow = viewer.locator('.general-queue-table tbody .data-row', { hasText: title });
    await viewerRow.locator('.priority-normal').waitFor({ state: 'visible' });

    const status = await actor.evaluate(async () => {
      const response = await fetch('/api/v1/root/queue/queue-pending-ci/actions', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          type: 'set_priority',
          priority: 'high',
          expected_revision: 1,
        }),
      });
      return response.status;
    });
    expect(status).toBe(200);
    await viewerRow.locator('.priority-high').waitFor({ state: 'visible', timeout: 5_000 });
  });

  it('rejects an action based on a stale item revision', async () => {
    const status = await actor.evaluate(async () => {
      const response = await fetch('/api/v1/root/queue/queue-pending-ci/actions', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          type: 'run_now',
          reason: 'incident response',
          expected_revision: 1,
        }),
      });
      return response.status;
    });
    expect(status).toBe(409);
  });
});
