/* Obfuscation anti-collecte simple, pas un secret : le contact reste public.
 * Le repli HTML encodé reste lisible et utilisable sans JavaScript. */
(() => {
  'use strict';
  for (const host of document.querySelectorAll('[data-legal-email]')) {
    const encoded = host.getAttribute('data-legal-email');
    if (!encoded || !/^(?:[a-f0-9]{2})+$/.test(encoded)) continue;
    const address = encoded.match(/../g).map(pair => String.fromCharCode(parseInt(pair, 16) ^ 43)).join('');
    if (!/^[a-z0-9._+-]+@[a-z0-9.-]+\.[a-z]{2,}$/i.test(address)) continue;
    const link = document.createElement('a');
    link.href = 'mailto:' + address;
    link.textContent = address;
    host.replaceChildren(link);
  }
})();
