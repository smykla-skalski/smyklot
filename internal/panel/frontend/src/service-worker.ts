/// <reference types="@sveltejs/kit" />

/**
 * SvelteKit native service worker.
 *
 * Replaces the hand-written sw.js. Uses $service-worker's `build`, `files`,
 * and `version` for precaching, with a cache-first strategy for hashed
 * assets and a network-first strategy for navigation requests.
 */

import { build, files, version } from '$service-worker';

const CACHE = `smyklot-panel-${version}`;
const ASSETS = [...build, ...files];

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE);
      await cache.addAll(ASSETS);
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key)));
      await self.clients.claim();
    })(),
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;

  const url = new URL(request.url);

  // Same-origin only.
  if (url.origin !== location.origin) return;

  // Hashed build assets: cache-first.
  if (build.some((asset) => url.pathname === asset)) {
    event.respondWith(
      (async () => {
        const cached = await caches.match(request);
        if (cached) return cached;
        const cache = await caches.open(CACHE);
        const fetched = await fetch(request);
        cache.put(request, fetched.clone());
        return fetched;
      })(),
    );
    return;
  }

  // Navigation requests: network-first, fall back to cached shell.
  if (request.mode === 'navigate') {
    event.respondWith(
      (async () => {
        try {
          return await fetch(request);
        } catch {
          const cached = await caches.match(request);
          if (cached) return cached;
          return (await caches.match('/')) ?? Response.error();
        }
      })(),
    );
    return;
  }

  // Static files: cache-first.
  if (files.some((file) => url.pathname === file)) {
    event.respondWith(
      (async () => {
        const cached = await caches.match(request);
        if (cached) return cached;
        const cache = await caches.open(CACHE);
        const fetched = await fetch(request);
        cache.put(request, fetched.clone());
        return fetched;
      })(),
    );
  }
});
