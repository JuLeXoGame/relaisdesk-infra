import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Non-régression : les modales admin (ex. ajout de facture manuelle, au
// contenu plus grand que la fenêtre) doivent rester entièrement
// accessibles : hauteur plafonnée + défilement interne.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const css = fs.readFileSync(path.join(root, 'relaisdesk/admin/styles.css'), 'utf8');

const block = css.match(/\.modal-content\s*\{[^}]*\}/s);
assert(block, '.modal-content introuvable dans admin/styles.css');
assert(block[0].includes('max-height'), '.modal-content : max-height absente, la modale peut dépasser de l’écran');
assert(/overflow-y\s*:\s*(auto|scroll)/.test(block[0]), '.modal-content : défilement interne absent');

console.log('admin-modal-checks: OK');
