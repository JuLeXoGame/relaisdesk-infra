import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Les messages de l'espace client (ex. erreurs Stripe contenant une URL du
// dashboard) doivent afficher les liens http(s) comme des ancres cliquables
// (curseur main), sans innerHTML : construction DOM uniquement, target _blank
// + rel noopener, pour que le HTML injecté reste impossible.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const appJs = fs.readFileSync(path.join(root, 'relaisdesk', 'client', 'app.js'), 'utf8');
const css = fs.readFileSync(path.join(root, 'relaisdesk', 'client', 'styles.css'), 'utf8');

const start = appJs.indexOf('function setMessage(');
assert(start !== -1, 'setMessage introuvable');
const end = appJs.indexOf('\n}\n', start);
assert(end !== -1, 'fin de setMessage introuvable');
const body = appJs.slice(start, end);
assert(body.includes("document.createElement('a')"), 'setMessage ne crée pas d’ancre');
assert(body.includes("'_blank'"), 'lien sans target _blank');
assert(body.includes('noopener'), 'lien sans rel noopener');
assert(!body.includes('innerHTML'), 'setMessage utilise innerHTML (XSS)');
assert(body.includes('split(/(https?'), 'setMessage ne détecte pas les URL http(s)');

assert(css.includes('.message a'), 'style .message a manquant');
assert(/\.message a\{[^}]*cursor:pointer/.test(css), 'curseur main manquant sur .message a');

console.log('message-link-checks: OK');
