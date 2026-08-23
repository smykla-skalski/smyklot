import { beforeEach, describe, expect, it, vi } from 'vitest';

const redirect = vi.hoisted(() => vi.fn());
vi.mock('@sveltejs/kit', () => ({ redirect }));

import { load as openRuntime } from '../src/routes/root/runtime/+page.ts';
import { load as openLegacySettings } from '../src/routes/root/settings/+page.ts';

describe('Root Runtime redirects [Unit]', () => {
  beforeEach(() => redirect.mockReset());

  it('opens bare Runtime on settings and keeps the query string', () => {
    void openRuntime({
      url: new URL('https://panel.example/panel/root/runtime?from=rail'),
    } as never);

    expect(redirect).toHaveBeenCalledWith(308, '/panel/root/runtime/settings?from=rail');
  });

  it('sends the legacy Root settings address directly to Runtime settings', () => {
    void openLegacySettings({
      url: new URL('https://panel.example/panel/root/settings?from=bookmark'),
    } as never);

    expect(redirect).toHaveBeenCalledWith(308, '/panel/root/runtime/settings?from=bookmark');
  });
});
