import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Le numéro de téléphone ne doit jamais apparaître en clair dans le HTML
// statique (ni affichage, ni lien tel:, ni JSON-LD) : les moissonneurs
// automatiques récoltent le source brut. Convention : attribut
// data-legal-phone (hex XOR 43, cf. legal-email.js) décodé par
// legal-phone.js, avec repli en entités HTML utilisable sans JavaScript.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const web = path.join(root, 'relaisdesk');

const FORBIDDEN = ['06 62 85 59 30', '0662855930', '+33662855930', 'tel:+33', 'tel:06'];

function htmlFiles(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'archives' || entry.name === 'downloads') continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...htmlFiles(full));
    else if (entry.name.endsWith('.html')) out.push(full);
  }
  return out;
}

const pages = htmlFiles(web);
assert(pages.length > 5, 'pages HTML introuvables');
for (const file of pages) {
  const body = fs.readFileSync(file, 'utf8');
  const rel = path.relative(root, file);
  for (const pattern of FORBIDDEN) {
    assert(!body.includes(pattern), `${rel}: numéro en clair (${pattern})`);
  }
  if (body.includes('data-legal-phone')) {
    assert(body.includes('legal-phone.js'), `${rel}: data-legal-phone sans legal-phone.js`);
  }
}

const decoder = fs.readFileSync(path.join(web, 'legal-phone.js'), 'utf8');
for (const pattern of FORBIDDEN) {
  assert(!decoder.includes(pattern), `legal-phone.js: numéro en clair (${pattern})`);
}
assert(decoder.includes('org-schema'), 'legal-phone.js: réinjection JSON-LD absente');
assert(fs.readFileSync(path.join(web, 'index.html'), 'utf8').includes('id="org-schema"'), 'index.html: id org-schema absent du JSON-LD');

console.log('phone-obfuscation-checks: OK');
