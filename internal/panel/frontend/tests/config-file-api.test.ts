import { describe, expect, it } from 'vitest';

import { createPanelApi, PanelApiError } from '../src/lib/api';
import { formatJson } from '../src/lib/merge';

describe('configuration file API', () => {
  for (const root of [false, true]) {
    for (const repository of [undefined, 'repo/雪']) {
      it(`keeps ${root ? 'Root' : 'workspace'} ${repository ? 'repository' : 'defaults'} reads separate from choices`, async () => {
        const calls: { path: string; init?: RequestInit }[] = [];
        const api = createPanelApi('/panel', (path, init) => {
          calls.push({ path, init });
          return Promise.resolve(
            new Response(
              path.endsWith('/resolution')
                ? '{"status":"pending"}'
                : '{"enabled":true,"available":true,"status":"pending"}',
              { status: path.endsWith('/resolution') ? 202 : 200 },
            ),
          );
        });
        const scope = root ? 'root/workspaces' : 'targets';
        const suffix = repository === undefined ? '' : '/repositories/repo%2F%E9%9B%AA';
        const base = `/panel/api/v1/${scope}/work%2Fspace${suffix}/config-file`;
        const status = root
          ? await api.fetchRootConfigFileStatus('work/space', repository)
          : await api.fetchConfigFileStatus('work/space', repository);
        expect(status.status).toBe('pending');
        expect(calls).toHaveLength(1);
        expect(calls[0]?.path).toBe(base);
        if (root) await api.previewRootConfigFile('work/space', repository);
        else await api.previewConfigFile('work/space', repository);
        expect(calls[1]?.path).toBe(`${base}/preview`);
        const choice = {
          review_token: 'a'.repeat(64),
          side: 'file' as const,
          document: { unexpected: true },
          actor_account_id: 'not-client-owned',
        };
        const receipt = root
          ? await api.resolveRootConfigFile('work/space', choice, repository)
          : await api.resolveConfigFile('work/space', choice, repository);
        expect(receipt).toEqual({ status: 'pending' });
        expect(calls[2]?.path).toBe(`${base}/resolution`);
        expect(calls[2]?.init).toMatchObject({ method: 'POST', credentials: 'same-origin' });
        expect(JSON.parse(String(calls[2]?.init?.body))).toEqual({
          review_token: choice.review_token,
          side: choice.side,
        });
        expect(calls).toHaveLength(3);
      });
    }
  }

  it('preserves exact numeric settings in both fresh choice documents', async () => {
    const body =
      '{"status":"blocked","checked_at":"2026-09-07T12:00:00Z","conflict_count":1,"review_token":"fresh","choices":[{"side":"panel","available":true,"document":{"config":{"number":9007199254740993,"small":1e-900,"signed_zero":-0}},"import_panel":false,"publish_file":true},{"side":"file","available":true,"document":{"config":{"number":9007199254740992}},"import_panel":true,"publish_file":false}]}';
    const api = createPanelApi('/panel', () => Promise.resolve(new Response(body)));
    for (const preview of [
      await api.previewConfigFile('workspace'),
      await api.previewRootConfigFile('workspace', 'repo'),
    ]) {
      expect(preview.conflict_count).toBe(1);
      expect(formatJson(preview.choices?.[0]?.document ?? null)).toContain('9007199254740993');
      expect(formatJson(preview.choices?.[0]?.document ?? null)).toContain('1e-900');
      expect(formatJson(preview.choices?.[0]?.document ?? null)).toContain('-0');
      expect(formatJson(preview.choices?.[1]?.document ?? null)).toContain('9007199254740992');
    }
  });

  it('returns a stale-review error without retrying the old decision', async () => {
    let calls = 0;
    const api = createPanelApi('/panel', () => {
      calls += 1;
      return Promise.resolve(
        new Response(
          '{"error":{"code":"config_file_changed","message":"Review the current values"}}',
          { status: 409 },
        ),
      );
    });
    const failure = await api
      .resolveConfigFile('workspace', {
        review_token: 'a'.repeat(64),
        side: 'panel',
      })
      .catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(PanelApiError);
    expect(failure).toMatchObject({ status: 409, code: 'config_file_changed' });
    expect(calls).toBe(1);
  });
});
