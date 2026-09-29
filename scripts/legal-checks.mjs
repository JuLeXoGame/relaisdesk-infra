import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = p => fs.readFileSync(path.join(root, p), 'utf8');
const version = '2026-09-27';
for (const file of ['relaisdesk/cgv.html', 'relaisdesk/index.html', 'relaisdesk/app.js', 'relaisdesk/client/app.js', 'api/mailer/mailer.go']) {
  assert(read(file).includes(version), `${file}: version courante absente`);
}
for (const file of ['relaisdesk/index.html', 'relaisdesk/app.js', 'relaisdesk/client/app.js']) {
  assert(!read(file).includes('2026-08-25'), `${file}: nouvelle commande sur anciennes CGV`);
}
const old = fs.readFileSync(path.join(root, 'api/mailer/legal/CGV-RelaisDesk-2026-08-25.txt'));
assert.equal(createHash('sha256').update(old).digest('hex'), '8f8dfdda5f02f57089dfe38a4b9caa684b47d916646a8e74ac38d87b3dea1be2', 'archive contractuelle historique altérée');
const september7 = fs.readFileSync(path.join(root, 'api/mailer/legal/CGV-RelaisDesk-2026-09-07.txt'));
assert.equal(createHash('sha256').update(september7).digest('hex'), 'ba7085ac657bde98e5b41fa176360b5a7d2ccf2840259eeff61310da4b984df6', 'ancienne grille tarifaire contractuelle altérée');

const cgv = read('relaisdesk/cgv.html');
for (const term of ['24,90', '239,00', '110,00', '1 100,00', '199,00', '1 990,00', '365 jours', '5 techniciens', '500 techniciens', 'legal-guarantee', 'sous-traitance-rgpd.html']) assert(cgv.includes(term), `CGV : ${term}`);
assert(!/(?<![\d])(?:10|20),00 € pour 30 jours/.test(cgv), 'CGV : anciens prix 10/20 €');
for (const forbidden of ['ChaCha20-Poly1305 /', 'région souveraine française']) assert(!cgv.includes(forbidden), `CGV obsolètes : ${forbidden}`);
assert.match(read('relaisdesk/index.html'), /<option\b[^>]*value="consumer"/);
assert(cgv.includes("aucun médiateur de la consommation n'est encore désigné"));
const september8=fs.readFileSync(path.join(root,'api/mailer/legal/CGV-RelaisDesk-2026-09-08.txt'));
assert.equal(createHash('sha256').update(september8).digest('hex'),'64854a5d4e4ed412dd8d2e429b07e2dbf0d53cb7a924432d1386c42cac0825d7');
for (const [file, digest] of [
  ['CGV-RelaisDesk-2026-09-11.txt', 'f70200cc63916adc623879d6fd6900cdf4b37bff933ed6f173679aa817ea4237'],
  ['ESSAI-RelaisDesk-2026-09-11-fleet-v2.txt', '310e46af809a5b5f3b11bf2eeede5ae662dad5c966782681bcf97e12f2c1910d'],
  ['CGV-RelaisDesk-2026-09-10.txt', 'b9e3eab6d33e62dae9badebd9ec4a6bb71cdbd31363fa695b174b09250bb4c4d'],
  ['ESSAI-RelaisDesk-2026-09-10-fleet-v1.txt', '23cefa9fb8963308e499f90a63c72da0e57946b0e16c4cb80b6e3491a6563f99'],
  ['ESSAI-RelaisDesk-2026-09-09.txt', 'e9731183499a848a52f01bd678b08e86d554e3a62fb9db207f0c8c0c5524bb87'],
  ['CGV-RelaisDesk-2026-09-09.txt', '547a75a4fad49bf84cef3568387e3e2c80759475d9246b644258719c9801a6c8'],
  ['ESSAI-RelaisDesk-2026-09-09-v2.txt', '0db587169ae03d14a9af6ccf488728f901943b42e79159c525ea86a3d57b347c'],
]) assert.equal(createHash('sha256').update(fs.readFileSync(path.join(root, 'api/mailer/legal', file))).digest('hex'), digest, file + ': archive modifiée');
for (const term of ['500 postes enregistrables', '1 000', '2 000', '4 500', 'par licence', 'même hors ligne', 'quinze minutes']) {
  assert(cgv.includes(term), 'quota absent des CGV : ' + term);
  assert(read(`api/mailer/legal/CGV-RelaisDesk-${version}.txt`).includes(term), 'quota absent des CGV email : ' + term);
}
assert(!read('relaisdesk/logiciel-libre.html').includes('restent indisponibles tant que'));

const contact = 'contact@relaisdesk.fr';
let encoded;
for (const filename of ['cgv.html', 'mentions-legales.html', 'politique-confidentialite.html', 'contact.html', 'sous-traitance-rgpd.html', 'formulaire-retractation.html']) {
  const html = read('relaisdesk/' + filename);
  assert(!html.includes(contact), `${filename}: email non obfusqué dans le HTML`);
  const spans = [...html.matchAll(/<span data-legal-email="([a-f0-9]+)">([\s\S]*?)<\/span>/g)];
  assert(spans.length, `${filename}: contact absent`);
  for (const match of spans) {
    const decoded = match[2].replace(/&#(\d+);/g, (_, n) => String.fromCharCode(Number(n)));
    assert(decoded.includes(`href="mailto:${contact}"`), `${filename}: repli sans JS inutilisable`);
    assert(decoded.includes(`>${contact}</a>`), `${filename}: repli sans JS illisible`);
    encoded = match[1];
  }
  assert(html.includes('src="legal-email.js" defer'), `${filename}: script manquant`);
}
const validHost = {getAttribute: () => encoded, replaceChildren(link) { this.link = link; }};
const badHost = {getAttribute: () => 'not-a-hex-value', replaceChildren() { throw Error('invalid data rendered'); }};
vm.runInNewContext(read('relaisdesk/legal-email.js'), {
  document: {querySelectorAll: () => [validHost, badHost], createElement: () => ({})},
});
assert.equal(validHost.link.href, 'mailto:' + contact);
assert.equal(validHost.link.textContent, contact);

const pricing = vm.createContext({document: {addEventListener() {}}, window: {}});
vm.runInContext(read('relaisdesk/app.js'), pricing);
for (const [capacity, expected] of [[10,199],[50,759],[100,1259],[500,4059]]) {
  assert.equal(vm.runInContext(`calculateUltraPrice(${capacity}, 'monthly')`, pricing), expected);
  assert.equal(vm.runInContext(`calculateUltraPrice(${capacity}, 'annual')`, pricing), expected * 10);
}
const notice = read('installer/LEGAL_NOTICE.txt');
for (const file of ['installer/assets/license.txt', 'installer/configurator/license.txt', 'installer/build/license.txt']) assert.equal(read(file).trim(), notice.trim());
assert(!notice.includes('RelaisDesk SAS'));
assert(notice.includes('entrepreneur individuel'));
assert(notice.includes('AGPLv3'));
console.log('OK : versions CGV, archives historiques, prix, information B2C, email avec/sans JS, notice des installateurs.');
