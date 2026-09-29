import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Les pages vitrines doivent reprendre fidèlement les conditions d'essai :
// 30 jours (relaisdesk/essai/conditions.html art. 2-3), carte vérifiée par
// Stripe sans débit pendant l'essai. Non-régression de la correction du
// 27/09/2026 (« 7 jours » / « sans carte bancaire » affichés par erreur).
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = p => fs.readFileSync(path.join(root, p), 'utf8');

const pages = ['relaisdesk/comparatif.html', 'relaisdesk/guide-demarrage.html'];
for (const file of pages) {
  const body = read(file);
  assert(body.includes('30 jours'), `${file}: mention des 30 jours d'essai absente`);
  assert(!body.includes('7 jours'), `${file}: ancienne durée d'essai « 7 jours »`);
  assert(!body.toLowerCase().includes('sans carte bancaire'), `${file}: mention « sans carte bancaire » contredite par les conditions d'essai (carte vérifiée requise)`);
}

const terms = read('relaisdesk/essai/conditions.html');
assert(terms.includes('30 jours'), 'conditions d’essai : durée de référence introuvable');
assert(terms.includes('vérification d\u2019une carte par Stripe') || terms.includes('vérification d\'une carte par Stripe'), 'conditions d’essai : vérification de carte introuvable');

console.log('showcase-copy-checks: OK');
