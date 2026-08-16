/// <reference types="@sveltejs/kit" />

/**
 * SvelteKit native service worker.
 *
 * Replaces the hand-written sw.js. Uses $service-worker's `build`, `files`,
 * and `version` for precaching, with a cache-first strategy for hashed
 * assets and a network-first strategy for navigation requests.
 */

import { base, build, files, version } from '$service-worker';

const SCOPE_PATH = `${base}/`;
const CACHE_PREFIX = `smyklot-panel:${encodeURIComponent(SCOPE_PATH)}:`;
const CACHE = `${CACHE_PREFIX}${version}`;
const ASSETS = [...build, ...files];
const SHELL_REQUEST = new Request(SCOPE_PATH, { credentials: 'same-origin' });

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE);
      await cache.addAll([...ASSETS, SHELL_REQUEST]);
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(
        keys
          .filter((key) => key.startsWith(CACHE_PREFIX) && key !== CACHE)
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

  // Build assets and static files are precached and immutable for this version.
  if (ASSETS.includes(url.pathname)) {
    event.respondWith(
      (async () => {
        const cache = await caches.open(CACHE);
        const cached = await cache.match(url.pathname);
        if (cached) return cached;
        const fetched = await fetch(request);
        if (fetched.ok && fetched.type === 'basic') {
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
        const cache = await caches.open(CACHE);
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
