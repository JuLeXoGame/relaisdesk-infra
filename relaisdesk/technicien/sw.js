const CACHE_NAME = 'relaisdesk-tech-v3';
const ASSETS_TO_CACHE = [
  './',
  './index.html',
  './styles.css',
  './app.js',
  './manifest.json',
  './logo.avif'
];
const CACHEABLE_URLS = new Set(ASSETS_TO_CACHE.map((asset) => new URL(asset, self.registration.scope).href));

// Installation : Mise en cache des assets statiques
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(ASSETS_TO_CACHE);
    }).then(() => self.skipWaiting())
  );
});

// Activation : Nettoyage des anciens caches
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
    }).then(() => self.clients.claim())
  );
});

// Stratégie Network-First avec fallback sur le cache pour l'application
self.addEventListener('fetch', (event) => {
	const requestURL = new URL(event.request.url);
	// Only the immutable application shell may enter the cache. API and
	// authenticated responses always stay on the network path.
	if (event.request.method !== 'GET' || requestURL.search || !CACHEABLE_URLS.has(requestURL.href)) {
    return;
  }

  event.respondWith(
    fetch(event.request)
      .then((response) => {
        if (response && response.status === 200) {
          const responseClone = response.clone();
          caches.open(CACHE_NAME).then((cache) => {
            cache.put(event.request, responseClone);
          });
        }
        return response;
      })
      .catch(() => caches.match(event.request))
  );
});
