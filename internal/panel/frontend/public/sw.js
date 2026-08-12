'use strict';

const BUILD_VERSION = '__smyklot_panel_version__';
const CACHE_MANIFEST = 'cache-manifest.json';
const PANEL_ICON = 'smyklot-avatar.png';
const scopeUrl = new URL(self.registration.scope);
const scopePath = scopeUrl.pathname;
const cachePrefix = `smyklot-panel:${encodeURIComponent(scopePath)}:`;
const currentCacheName = `${cachePrefix}${BUILD_VERSION}`;
const shellRequest = new Request(scopeUrl.href, { credentials: 'same-origin' });

self.addEventListener('install', (event) => {
  event.waitUntil(
    precacheCurrentBuild().then(() => {
      return self.skipWaiting();
    }),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    trimOldCaches().then(() => {
      return self.clients.claim();
    }),
  );
});

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.method !== 'GET') return;

  const url = new URL(request.url);
  if (url.origin !== scopeUrl.origin || !url.pathname.startsWith(scopePath)) return;

  const relativePath = url.pathname.slice(scopePath.length);
  if (isServerRequest(relativePath)) return;

  if (request.mode === 'navigate' && isPanelNavigation(relativePath)) {
    const refresh = refreshShell(event);
    event.waitUntil(refresh.then(() => undefined).catch(() => undefined));
    event.respondWith(cachedShellOr(refresh));
    return;
  }

  if (relativePath.startsWith('assets/') || relativePath === PANEL_ICON) {
    event.respondWith(cacheFirst(request));
  }
});

async function precacheCurrentBuild() {
  const manifestUrl = new URL(CACHE_MANIFEST, scopeUrl);
  const response = await fetch(manifestUrl, {
    cache: 'no-store',
    credentials: 'same-origin',
  });
  if (!response.ok) {
    throw new Error(`cache manifest returned ${response.status}`);
  }

  const manifest = await response.json();
  const files = new Set();
  for (const entry of Object.values(manifest)) {
    if (entry === null || typeof entry !== 'object') continue;
    addManifestFile(files, entry.file);
    for (const file of entry.css ?? []) addManifestFile(files, file);
    for (const file of entry.assets ?? []) addManifestFile(files, file);
  }

  const requests = [
    shellRequest,
    new Request(new URL(`${PANEL_ICON}?v=${encodeURIComponent(BUILD_VERSION)}`, scopeUrl), {
      credentials: 'same-origin',
    }),
    ...[...files].map(
      (file) => new Request(new URL(file, scopeUrl), { credentials: 'same-origin' }),
    ),
  ];
  const cache = await caches.open(currentCacheName);
  await cache.addAll(requests);
}

function addManifestFile(files, file) {
  if (typeof file !== 'string') return;
  const url = new URL(file, scopeUrl);
  if (url.origin === scopeUrl.origin && url.pathname.startsWith(`${scopePath}assets/`)) {
    files.add(file);
  }
}

async function trimOldCaches() {
  const names = (await caches.keys()).filter((name) => name.startsWith(cachePrefix));
  const previous = names.filter((name) => name !== currentCacheName);
  await Promise.all(previous.slice(0, -1).map((name) => caches.delete(name)));
}

function isServerRequest(relativePath) {
  return (
    relativePath === 'api' ||
    relativePath.startsWith('api/') ||
    relativePath === 'auth' ||
    relativePath.startsWith('auth/')
  );
}

function isPanelNavigation(relativePath) {
  const trimmed = relativePath.replace(/^\/+|\/+$/gu, '');
  const parts = trimmed === '' ? [] : trimmed.split('/');
  if (parts.length === 0 || (parts.length === 1 && parts[0] === 'index.html')) return true;
  if (parts.length === 2 && parts[0] === 'invite') {
    return /^[A-Za-z0-9_-]{43}$/u.test(parts[1]);
  }
  if (parts.length === 4 && parts[0] === 'i' && parts[1] !== '') {
    return parts[2] === 'history' && isHistorySection(parts[3]);
  }
  if (parts.length === 3 && parts[0] === 'i' && parts[1] !== '') {
    return ['settings', 'repositories', 'users', 'invitations', 'history'].includes(parts[2]);
  }
  if (parts[0] !== 'root') return false;
  if (parts.length === 1) return true;
  if (parts.length === 2)
    return ['installations', 'access', 'history', 'settings'].includes(parts[1]);
  if (parts.length === 3 && parts[1] === 'access') {
    return ['users', 'invitations'].includes(parts[2]);
  }
  if (parts.length === 3 && parts[1] === 'history') {
    return ['audit', 'failures'].includes(parts[2]);
  }
  if (
    parts.length === 5 &&
    parts[1] === 'installations' &&
    parts[2] !== '' &&
    parts[3] === 'history'
  ) {
    return isHistorySection(parts[4]);
  }
  return (
    parts.length === 4 &&
    parts[1] === 'installations' &&
    parts[2] !== '' &&
    ['settings', 'repositories', 'users', 'invitations', 'history'].includes(parts[3])
  );
}

function isHistorySection(value) {
  return value === 'audit' || value === 'failures';
}

async function refreshShell(event) {
  const response = await fetch(event.request);
  if (response.ok && response.type === 'basic') {
    const cache = await caches.open(currentCacheName);
    await cache.put(shellRequest, response.clone());
  }
  return response;
}

async function cachedShellOr(networkResponse) {
  const cache = await caches.open(currentCacheName);
  const cached = await cache.match(shellRequest);
  if (cached !== undefined) return cached;
  try {
    return await networkResponse;
  } catch {
    return new Response('Smyklot is offline and its application shell is not cached yet.', {
      status: 503,
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
    });
  }
}

async function cacheFirst(request) {
  const cached = await caches.match(request);
  if (cached !== undefined) return cached;

  const response = await fetch(request);
  if (response.ok && response.type === 'basic') {
    const cache = await caches.open(currentCacheName);
    await cache.put(request, response.clone());
  }
  return response;
}
