import { readFile } from 'node:fs/promises';
import { runInNewContext } from 'node:vm';

import { describe, expect, it, vi } from 'vitest';

import { registerPanelServiceWorker } from '../src/lib/service-worker';

describe('panel service worker registration', () => {
  it('registers the worker at the runtime base without using the HTTP cache', async () => {
    const registration = {} as ServiceWorkerRegistration;
    const register = vi.fn().mockResolvedValue(registration);

    await expect(registerPanelServiceWorker('/console', '1.23.0', { register })).resolves.toBe(
      registration,
    );
    expect(register).toHaveBeenCalledWith('/console/sw.js', {
      scope: '/console/',
      updateViaCache: 'none',
    });
  });

  it('does not register unsupported browsers or development builds', async () => {
    const register = vi.fn();

    await expect(registerPanelServiceWorker('/panel', null, { register })).resolves.toBeNull();
    await expect(registerPanelServiceWorker('/panel', '1.23.0', null)).resolves.toBeNull();
    expect(register).not.toHaveBeenCalled();
  });
});

describe('panel service worker cache boundary', () => {
  it('precaches the shell and build graph without private endpoints', async () => {
    const worker = await loadWorker();
    worker.fetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        'index.html': {
          file: 'assets/app.js',
          css: ['assets/app.css'],
          assets: ['assets/font.woff2', '../api/v1/session'],
        },
      }),
    });

    let lifetime: Promise<unknown> | undefined;
    requireListener(
      worker,
      'install',
    )({
      waitUntil: (promise: Promise<unknown>) => (lifetime = promise),
    });
    await lifetime;

    const requests = worker.addAll.mock.calls[0]?.[0] as Request[] | undefined;
    const urls = requests?.map((request) => request.url);
    expect(urls).toEqual([
      'https://example.test/panel/',
      'https://example.test/panel/smyklot-avatar.png?v=__smyklot_panel_version__',
      'https://example.test/panel/assets/app.js',
      'https://example.test/panel/assets/app.css',
      'https://example.test/panel/assets/font.woff2',
    ]);
    expect(worker.skipWaiting).toHaveBeenCalledOnce();
  });

  it('intercepts app routes and assets but never API, auth, or unknown routes', async () => {
    const worker = await loadWorker();
    worker.match.mockResolvedValue(new Response('cached'));
    worker.fetch.mockResolvedValue(new Response('network'));

    const route = fetchEvent('https://example.test/panel/root/history/audit', 'navigate');
    requireListener(worker, 'fetch')(route);
    expect(route.respondWith).toHaveBeenCalledOnce();

    for (const url of [
      'https://example.test/panel/root/access',
      'https://example.test/panel/root/history',
      'https://example.test/panel/i/acme/history/audit',
      'https://example.test/panel/i/acme/history/failures',
      'https://example.test/panel/root/installations/acme/history/audit',
      'https://example.test/panel/root/installations/acme/history/failures',
    ]) {
      const historyRoute = fetchEvent(url, 'navigate');
      requireListener(worker, 'fetch')(historyRoute);
      expect(historyRoute.respondWith).toHaveBeenCalledOnce();
    }

    const asset = fetchEvent('https://example.test/panel/assets/app.js', 'same-origin');
    requireListener(worker, 'fetch')(asset);
    expect(asset.respondWith).toHaveBeenCalledOnce();

    for (const url of [
      'https://example.test/panel/api/v1/session',
      'https://example.test/panel/auth/github/start',
      'https://example.test/panel/unknown',
      'https://example.test/panel/i/acme/history/unknown',
      'https://example.test/panel/root/installations/acme/history/unknown',
      'https://example.test/panel/root//history/audit',
      'https://cdn.example.test/panel/assets/app.js',
    ]) {
      const request = fetchEvent(url, 'navigate');
      requireListener(worker, 'fetch')(request);
      expect(request.respondWith).not.toHaveBeenCalled();
    }
  });
});

interface WorkerHarness {
  listeners: Record<string, (event: unknown) => void>;
  addAll: ReturnType<typeof vi.fn>;
  match: ReturnType<typeof vi.fn>;
  fetch: ReturnType<typeof vi.fn>;
  skipWaiting: ReturnType<typeof vi.fn>;
}

async function loadWorker(): Promise<WorkerHarness> {
  const source = await readFile(new URL('../public/sw.js', import.meta.url), 'utf8');
  const listeners: WorkerHarness['listeners'] = {};
  const addAll = vi.fn().mockResolvedValue(undefined);
  const match = vi.fn().mockResolvedValue(undefined);
  const fetch = vi.fn();
  const skipWaiting = vi.fn().mockResolvedValue(undefined);
  const cache = { addAll, match, put: vi.fn().mockResolvedValue(undefined) };

  runInNewContext(source, {
    Request,
    Response,
    URL,
    caches: {
      delete: vi.fn().mockResolvedValue(true),
      keys: vi.fn().mockResolvedValue([]),
      match,
      open: vi.fn().mockResolvedValue(cache),
    },
    fetch,
    self: {
      addEventListener: (type: string, listener: (event: unknown) => void) => {
        listeners[type] = listener;
      },
      clients: { claim: vi.fn().mockResolvedValue(undefined) },
      registration: { scope: 'https://example.test/panel/' },
      skipWaiting,
    },
  });

  return { listeners, addAll, match, fetch, skipWaiting };
}

function fetchEvent(url: string, mode: RequestMode) {
  return {
    request: { method: 'GET', mode, url },
    respondWith: vi.fn(),
    waitUntil: vi.fn(),
  };
}

function requireListener(worker: WorkerHarness, type: string): (event: unknown) => void {
  const listener = worker.listeners[type];
  if (listener === undefined) throw new Error(`service worker did not register ${type}`);
  return listener;
}
