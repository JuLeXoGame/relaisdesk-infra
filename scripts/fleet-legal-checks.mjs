import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = file => fs.readFileSync(path.join(root, file), 'utf8');
const plain = value => value.replace(/<[^>]*>/g, '').replace(/\s+/g, ' ').trim();
const terms = read('api/mailer/legal/CGV-RelaisDesk-2026-09-21.txt');
const webTerms = plain(read('relaisdesk/cgv.html'));
const dpa = plain(read('relaisdesk/sous-traitance-rgpd.html'));
const privacy = plain(read('relaisdesk/politique-confidentialite.html'));

// The clauses in the durable email attachment must match the visible web terms.
for (const start of ['La console de gestion', "Compatibilité de l'accès", "Le Client s'engage", 'Accès permanent sans présence', "Fin de l'autorisation", 'Les accès à la console']) {
  const paragraph = terms.split(/\r?\n\r?\n/).find(p => p.startsWith(start));
  assert(paragraph, 'missing clause: ' + start);
  assert(webTerms.includes(plain(paragraph)), 'website/email mismatch: ' + start);
}
for (const start of ['RelaisDesk traite les données', "Pour l'accès permanent", 'Parc : les fiches']) {
  const paragraph = terms.split(/\r?\n\r?\n/).find(p => p.startsWith(start));
  assert(paragraph, 'missing DPA clause: ' + start);
  assert(dpa.includes(plain(paragraph)), 'DPA/email mismatch: ' + start);
  if (start.startsWith('Parc :')) assert(privacy.includes(plain(paragraph)), 'privacy/DPA retention mismatch');
}
for (const term of ["nom d'hôte", 'clé publique', "dernière adresse IP", 'nonces', 'procédure manuelle', 'trente jours', 'ne vaut pas effacement automatique']) {
  assert(dpa.includes(term), 'DPA missing: ' + term);
}
for (const file of ['relaisdesk/cgv.html', 'relaisdesk/sous-traitance-rgpd.html', 'relaisdesk/essai/conditions.html']) {
  assert(!/2026-09-(09|10)|(?:9|10) septembre 2026/.test(read(file)), file + ': stale current cross-reference');
}
for (const file of ['relaisdesk/app.js', 'relaisdesk/client/app.js', 'relaisdesk/essai/app.js']) {
  assert(read(file).includes('2026-09-21'), file + ': wrong checkout version');
  assert(!read(file).includes('2026-09-11'), file + ': stale checkout version');
}
assert.match(read('database/trials.go'), /const TrialTermsVersion = "2026-09-21-fleet-v2"/);
assert.match(read('api/mailer/mailer.go'), /const CurrentTrialTermsVersion = "2026-09-21-fleet-v2"/);
for (const file of ['relaisdesk/index.html', 'relaisdesk/client/index.html', 'relaisdesk/essai/index.html']) {
  const cacheVersion = '20260921-legal-crypto-v1';
  assert(read(file).includes('app.js?v=' + cacheVersion), file + ': cache version missing');
  assert(read(file).includes('autorisation préalable documentée'), file + ': authorization notice missing');
  assert(read(file).includes('Linux ou macOS'), file + ': compatibility notice missing');
}
const i18n = {window: {}, document: {readyState: 'loading', addEventListener() {}}};
vm.runInNewContext(read('relaisdesk/i18n.js'), i18n);
for (const lang of ['fr', 'en', 'de', 'es', 'it', 'ru', 'pl']) {
  const translated = i18n.window.RdI18n.translations[lang];
  assert(translated.enroll_desc.includes('15'), lang + ': missing enrollment expiry');
  for (const platform of ['Windows', 'Linux', 'macOS']) {
    assert(translated.enroll_desc.includes(platform), lang + ': missing compatibility limitation');
    assert(translated.devices_subtitle.includes(platform), lang + ': missing fleet subtitle limitation');
  }
  assert(!translated.enroll_desc.includes('systemd'), lang + ': obsolete Linux availability');
}
const sources = read('relaisdesk/logiciel-libre.html');
assert(sources.includes('e34c8bab932791961b2406bac5d0256b744b40b1'));
assert(sources.includes('72d1d638e56d6f2dd4f357263000738144633660'), 'historical source link lost');
assert(sources.includes('pas à elle seule les sources des lanceurs'), 'engine confused with complete package source');
assert(sources.includes("distribution n'est pas conditionnée à SignPath"));
assert(sources.includes('restent à vérifier et à compléter'), 'sources incorrectly declared complete');
console.log('OK: permanent access clauses, DPA/privacy/email consistency, versions, seven-language notices and source scope.');
