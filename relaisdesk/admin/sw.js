// Service Worker pour RelaisDesk Admin (PWA)
const CACHE_NAME = 'relaisdesk-admin-v5';
const ASSETS_TO_CACHE = [
  './',
  './index.html',
  './styles.css',
  './app.js',
  './logo.avif',
  './manifest.json'
];
const CACHEABLE_URLS = new Set(ASSETS_TO_CACHE.map((asset) => new URL(asset, self.registration.scope).href));

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(ASSETS_TO_CACHE);
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(
        keys.map((key) => {
          if (key !== CACHE_NAME) {
            return caches.delete(key);
          }
        })
      );
    })
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
	const requestURL = new URL(event.request.url);
  if (event.request.method !== 'GET' || requestURL.search || !CACHEABLE_URLS.has(requestURL.href)) {
    return;
  }
  // Network First: Always try to get the latest version from the server first
  event.respondWith(
    fetch(event.request)
      .then((networkResponse) => {
		if (networkResponse && networkResponse.status === 200) {
          const clone = networkResponse.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(event.request, clone));
        }
        return networkResponse;
      })
      .catch(() => {
        return caches.match(event.request);
      })
  );
});
