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
const SHELL_REQUEST = new Request(SCOPE_PATH, { credentials: 'same-origin' });

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(await CACHE);
      await cache.addAll([...ASSETS, SHELL_REQUEST]);
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const current = await CACHE;
      const keys = await caches.keys();
      const scoped = keys.filter((key) => key.startsWith(CACHE_PREFIX));
      // CacheStorage.keys() is creation-ordered. Keep the immediately previous
      // build so a tab claimed during deployment can still lazy-load chunks
      // named by the document it already has open.
      const previous = scoped.filter((key) => key !== current).at(-1);
      await Promise.all(
        scoped
          .filter((key) => key !== current && key !== previous)
          .map((key) => caches.delete(key)),
      );
      await self.clients.claim();
    })(),
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;

  const url = new URL(request.url);

  // Same-origin only.
  if (url.origin !== location.origin || !url.pathname.startsWith(SCOPE_PATH)) return;

  // Immutable chunk names carry their content hash, so a cross-version lookup
  // cannot return the wrong bytes. Static files may keep the same name between
  // builds and must be read from the current cache only.
  if (url.pathname.startsWith(IMMUTABLE_PATH) || ASSETS.has(url.pathname)) {
    event.respondWith(
      (async () => {
        // A hashed chunk may be answered from any version's cache, and that is the
        // common case, so it is served without opening this version's - one fewer
        // CacheStorage round trip on the path every chunk takes.
        const cached = url.pathname.startsWith(IMMUTABLE_PATH)
          ? await caches.match(url.pathname)
          : await (await caches.open(await CACHE)).match(url.pathname);
        if (cached) return cached;

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
