/* Obfuscation anti-collecte simple, pas un secret : le contact reste public.
 * Même convention que legal-email.js (hex XOR 43). Le repli HTML encodé
 * reste lisible et utilisable sans JavaScript. */
(() => {
  'use strict';
  const containers = document.querySelectorAll('[data-legal-phone]');
  let international = '';
  for (const host of containers) {
    const encoded = host.getAttribute('data-legal-phone');
    if (!encoded || !/^(?:[a-f0-9]{2})+$/.test(encoded)) continue;
    const decoded = encoded.match(/../g).map(pair => String.fromCharCode(parseInt(pair, 16) ^ 43)).join('');
    if (!/^\+33\d{9}$/.test(decoded)) continue;
    international = decoded;
    const national = '0' + decoded.slice(3);
    const display = national.replace(/(\d{2})(?=\d)/g, '$1 ').trim();
    const link = document.createElement('a');
    link.href = 'tel:' + decoded;
    link.textContent = display;
    const style = host.getAttribute('data-legal-phone-style');
    if (style) link.setAttribute('style', style);
    host.replaceChildren(link);
  }
  // Réinjecte le téléphone dans le JSON-LD (retiré du HTML statique) pour
  // conserver le référencement : les moteurs exécutent le JavaScript.
  if (international) {
    const schema = document.getElementById('org-schema');
    if (schema) {
      try {
        const data = JSON.parse(schema.textContent);
        const nodes = Array.isArray(data['@graph']) ? data['@graph'] : [data];
        for (const node of nodes) {
          if (node && node['@type'] === 'Organization' && !node.telephone) {
            node.telephone = international;
          }
        }
        schema.textContent = JSON.stringify(data);
      } catch {
        /* schéma inchangé si illisible */
      }
    }
  }
})();
