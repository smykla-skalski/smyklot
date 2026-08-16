import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const source = readFileSync(new URL('../src/service-worker.ts', import.meta.url), 'utf8');
const NativeRequest = Request;

describe('the panel service worker', () => {
  it('names caches within its panel scope', () => {
    expect(source).toContain('smyklot-panel:${encodeURIComponent(SCOPE_PATH)}:');
    expect(source).toContain('key.startsWith(CACHE_PREFIX)');
  });

  it('installs a canonical shell fallback and refreshes it after navigation', () => {
    expect(source).toContain("new Request(SCOPE_PATH, { credentials: 'same-origin' })");
    expect(source).toContain('cache.addAll([...ASSETS, SHELL_REQUEST])');
    expect(source).toContain('cache.put(SHELL_REQUEST, fetched.clone())');
    expect(source).toContain('cache.match(SHELL_REQUEST)');
  });

  it('never replaces the shell fallback with a navigated API response', () => {
    expect(source).toContain("contentType.startsWith('text/html')");
    expect(source).toMatch(
      /fetched\.ok && fetched\.type === 'basic' && contentType\.startsWith\('text\/html'\)/u,
    );
  });

  it('does not intercept same-origin traffic outside the panel mount', () => {
    expect(source).toContain('!url.pathname.startsWith(SCOPE_PATH)');
  });
});

describe('a panel service-worker upgrade', () => {
  const listeners = new Map<string, (event: ExtendableEvent | FetchEvent) => void>();
  const stored = new Map<string, Map<string, Response>>();
  const deleted: string[] = [];
  const skipWaiting = vi.fn(async () => undefined);
  const claim = vi.fn(async () => undefined);
  const network = vi.fn(async () => new Response('missing', { status: 404 }));

  beforeEach(async () => {
    vi.resetModules();
    vi.clearAllMocks();
    listeners.clear();
    stored.clear();
    deleted.length = 0;

    stored.set('unrelated', new Map());
    stored.set('smyklot-panel:%2Fpanel%2F:v0', new Map());
    stored.set(
      'smyklot-panel:%2Fpanel%2F:v1',
      new Map([
        [
          '/panel/_app/immutable/old.js',
          new Response('previous build', { headers: { 'Content-Type': 'text/javascript' } }),
        ],
        ['/panel/theme-boot.js', new Response('previous static file')],
      ]),
    );

    vi.stubGlobal('location', new URL('https://panel.example/panel/'));
    vi.stubGlobal(
      'Request',
      class TestRequest extends NativeRequest {
        constructor(input: RequestInfo | URL, init?: RequestInit) {
          super(typeof input === 'string' ? new URL(input, 'https://panel.example') : input, init);
        }
      },
    );
    vi.stubGlobal('fetch', network);
    vi.stubGlobal('caches', cacheStorage(stored, deleted));
    vi.stubGlobal('self', {
      addEventListener: (type: string, listener: (event: ExtendableEvent | FetchEvent) => void) =>
        listeners.set(type, listener),
      skipWaiting,
      clients: { claim },
    });

    await import('../src/service-worker');
  });

  afterEach(() => vi.unstubAllGlobals());

  it('keeps the previous cache and serves its lazy chunks after claiming old tabs', async () => {
    await dispatchExtended(listeners, 'install');
    await dispatchExtended(listeners, 'activate');

    expect([...stored.keys()]).toEqual([
      'unrelated',
      'smyklot-panel:%2Fpanel%2F:v1',
      'smyklot-panel:%2Fpanel%2F:v2',
    ]);
    expect(deleted).toEqual(['smyklot-panel:%2Fpanel%2F:v0']);
    expect(claim).toHaveBeenCalledOnce();

    const response = await dispatchFetch(
      listeners,
      new Request('https://panel.example/panel/_app/immutable/old.js'),
    );

    expect(await response.text()).toBe('previous build');
    expect(network).not.toHaveBeenCalled();

    const currentStatic = await dispatchFetch(
      listeners,
      new Request('https://panel.example/panel/theme-boot.js?v=v2'),
    );
    expect(await currentStatic.text()).toBe('cached /panel/theme-boot.js');
    expect(network).not.toHaveBeenCalled();
  });
});

vi.mock('$service-worker', () => ({
  base: '/panel',
  build: ['/panel/_app/immutable/current.js'],
  files: ['/panel/theme-boot.js'],
  version: 'v2',
}));

function cacheStorage(stored: Map<string, Map<string, Response>>, deleted: string[]): CacheStorage {
  return {
    keys: async () => [...stored.keys()],
    open: async (name: string) => {
      let entries = stored.get(name);
      if (entries === undefined) {
        entries = new Map();
        stored.set(name, entries);
      }
      return {
        addAll: async (requests: RequestInfo[]) => {
          for (const request of requests) {
            const path =
              request instanceof Request ? new URL(request.url).pathname : String(request);
            entries.set(path, new Response(`cached ${path}`));
          }
        },
        match: async (request: RequestInfo | URL) => entries.get(cachePath(request)),
        put: async (request: RequestInfo | URL, response: Response) => {
          entries.set(cachePath(request), response);
        },
      } as Cache;
    },
    delete: async (name: string) => {
      deleted.push(name);
      return stored.delete(name);
    },
    match: async (request: RequestInfo | URL) => {
      const path = cachePath(request);
      for (const entries of stored.values()) {
        const response = entries.get(path);
        if (response !== undefined) return response.clone();
      }
      return undefined;
    },
  } as CacheStorage;
}

function cachePath(request: RequestInfo | URL): string {
  if (request instanceof Request) return new URL(request.url).pathname;
  if (request instanceof URL) return request.pathname;
  return request;
}

async function dispatchExtended(
  listeners: Map<string, (event: ExtendableEvent | FetchEvent) => void>,
  type: 'install' | 'activate',
): Promise<void> {
  let completion: Promise<unknown> | undefined;
  listeners.get(type)?.({
    waitUntil: (promise: Promise<unknown>) => (completion = promise),
  } as unknown as ExtendableEvent);
  await completion;
}

async function dispatchFetch(
  listeners: Map<string, (event: ExtendableEvent | FetchEvent) => void>,
  request: Request,
): Promise<Response> {
  let response: Promise<Response> | undefined;
  listeners.get('fetch')?.({
    request,
    respondWith: (value: Response | PromiseLike<Response>) => (response = Promise.resolve(value)),
  } as unknown as FetchEvent);
  if (response === undefined) throw new Error('fetch was not intercepted');
  return response;
}
