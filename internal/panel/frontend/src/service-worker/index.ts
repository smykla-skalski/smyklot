/// <reference lib="webworker" />

declare const self: ServiceWorkerGlobalScope;

/**
 * SvelteKit native service worker.
 *
 * Replaces the hand-written sw.js. Precaches the built bundle and the static
 * directory, with a cache-first strategy for hashed assets and a network-first
 * strategy for navigation requests.
 */

import { assets, immutable } from '$app/manifest';

import { panelUrl } from '#lib/base.js';
import { basePath } from '#lib/paths.js';

// The scope this worker is registered at. `basePath` carries no trailing separator -
// see `#lib/paths.js`, which is where SvelteKit 3's removal of the `base` export is
// answered, once, for the worker and the app alike.
const SCOPE_PATH = `${basePath}/`;
const CACHE_PREFIX = `smyklot-panel:${encodeURIComponent(SCOPE_PATH)}:`;
// `$app/manifest` reports the built bundle and the static directory relative to the
// base path; the `$service-worker` module it replaces reported them already prefixed.
const ASSETS = new Set([...immutable, ...assets].map((file) => panelUrl(basePath, file.path)));

/**
 * The cache this build owns, named after the build itself.
 *
 * Every entry in `immutable` carries a content hash, so the list changes exactly when
 * the app does - it is already the identity, and this only shortens it to fit a name.
 * The deployment version is deliberately not used: SvelteKit 3 does not give a worker
 * one (`$app/env` reads a payload nothing fills there), and the ways around that ran
 * through a build-time define whose emitted shape the server then had to be able to
 * rewrite. Naming the cache after the assets it holds needs none of that, and two
 * releases that ship identical assets now keep their cache instead of rotating for
 * nothing.
 *
 * A promise because the digest is: `crypto.subtle` is asynchronous, and every reader
 * here is already inside one.
 */
const CACHE = (async () => {
  const identity = [...immutable]
    .map((file) => file.path)
    .sort()
    .join('\n');
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(identity));

  return (
    CACHE_PREFIX +
    Array.from(new Uint8Array(digest, 0, 8), (byte) => byte.toString(16).padStart(2, '0')).join('')
  );
})();
const IMMUTABLE_PATH = panelUrl(basePath, '_app/immutable/');
/**
 * Where each cache records when it was last installed.
 *
 * `CacheStorage.keys()` is in creation order, which is not install order: a name is a
 * digest of the bundle, so it repeats when a release is rolled back, and reopening an
 * existing cache leaves it where it first appeared. Rotating on `keys()` therefore reads
 * the rolled-back build - the one the claimed tabs are running from - as the oldest, and
 * deletes it. The stamp is written on every install, so it says what `keys()` cannot.
 */
const STAMP_PATH = `${SCOPE_PATH}__installed__`;
const SHELL_REQUEST = new Request(SCOPE_PATH, { credentials: 'same-origin' });

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      // Opened, never dropped first. Dropping it would put `CacheStorage.keys()` back in
      // true install order, which `activate` below would rather have - a name is a digest
      // of the bundle now, so it repeats when a release is rolled back and reopening
      // leaves it where it was. It is not worth what it costs: this worker is not part of
      // the bundle it hashes, so changing this file alone installs a new worker with the
      // same name, and dropping the cache would empty the one the running worker is
      // serving from. `addAll` is one transaction, so a reader who is offline when that
      // happens would be left with no shell at all - which is the thing this exists for.
      const cache = await caches.open(await CACHE);
      await cache.addAll([...ASSETS, SHELL_REQUEST]);
      await cache.put(STAMP_PATH, new Response(String(Date.now())));
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const current = await CACHE;
      const keys = await caches.keys();
      const scoped = keys.filter((key) => key.startsWith(CACHE_PREFIX) && key !== current);
      // Keep the build installed most recently before this one, so a tab claimed during a
      // deployment can still lazy-load the chunks the document it already has names. Read
      // from each cache's own stamp rather than from `keys()` - see `STAMP_PATH`.
      const stamped = await Promise.all(
        scoped.map(async (key) => ({ key, at: await installedAt(key) })),
      );
      const previous = stamped.reduce<{ key: string; at: number } | null>(
        // `>=` so a tie falls back to creation order, which is what a cache written
        // before stamps existed has to be judged on.
        (newest, entry) => (newest === null || entry.at >= newest.at ? entry : newest),
        null,
      );
      await Promise.all(
        stamped
          .filter((entry) => entry.key !== previous?.key)
          .map((entry) => caches.delete(entry.key)),
      );
      await self.clients.claim();
    })(),
  );
});

/** When a cache was last installed, or zero for one written before stamps existed. */
async function installedAt(name: string): Promise<number> {
  const stamp = await (await caches.open(name)).match(STAMP_PATH);
  if (stamp === undefined) return 0;

  return Number.parseInt(await stamp.text(), 10) || 0;
}

/**
 * Replaces a cached static file with what the network has, after answering from cache.
 *
 * The cache is named after the built bundle, so a release that changes only a file in
 * `static` - whose name carries no content hash - leaves the name alone and the cached
 * copy would otherwise stand forever. Answering from cache first keeps `theme-boot.js`
 * off the critical path, where it blocks the first paint; the refresh lands for the
 * next load. A failure here is not one worth reporting: the reader already has an answer.
 */
async function refreshStatic(request: Request, path: string): Promise<void> {
  const fetched = await fetch(request).catch(() => null);
  if (fetched === null || !fetched.ok || fetched.type !== 'basic') return;

  const cache = await caches.open(await CACHE);
  await cache.put(path, fetched);
}

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;

  const url = new URL(request.url);

  // Same-origin only.
  if (url.origin !== location.origin || !url.pathname.startsWith(SCOPE_PATH)) return;

  // Immutable chunk names carry their content hash, so a cross-version lookup cannot
  // return the wrong bytes. A file in `static` keeps its name between builds, so a
  // cached copy of one can be stale - see `refreshStatic`.
  const immutable = url.pathname.startsWith(IMMUTABLE_PATH);
  if (immutable || ASSETS.has(url.pathname)) {
    event.respondWith(
      (async () => {
        // A hashed chunk may be answered from any version's cache, and that is the
        // common case, so it is served without opening this version's - one fewer
        // CacheStorage round trip on the path every chunk takes.
        const cached = immutable
          ? await caches.match(url.pathname)
          : await (await caches.open(await CACHE)).match(url.pathname);
        if (cached) {
          if (!immutable) event.waitUntil(refreshStatic(request, url.pathname));

          return cached;
        }

        const fetched = await fetch(request);
        if (fetched.ok && fetched.type === 'basic') {
          const cache = await caches.open(await CACHE);
          await cache.put(url.pathname, fetched.clone());
        }
        return fetched;
      })(),
    );
    return;
  }

  // Navigation requests all resolve to the SPA shell. Refresh that canonical
  // response from the network, then use the installed copy when offline.
  if (request.mode === 'navigate') {
    event.respondWith(
      (async () => {
        const cache = await caches.open(await CACHE);
        try {
          const fetched = await fetch(request);
          const contentType = fetched.headers.get('content-type')?.toLowerCase() ?? '';
          if (fetched.ok && fetched.type === 'basic' && contentType.startsWith('text/html')) {
            await cache.put(SHELL_REQUEST, fetched.clone());
          }
          return fetched;
        } catch {
          const cached = await cache.match(SHELL_REQUEST);
          if (cached) return cached;
          return new Response('Smyklot is offline and its application shell is unavailable.', {
            status: 503,
            headers: { 'Content-Type': 'text/plain; charset=utf-8' },
          });
        }
      })(),
    );
  }
});
