import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = file => fs.readFileSync(path.join(root, file), 'utf8');
class Element {
  constructor() { this.handlers = {}; this.children = []; this.disabled = false; this.open = false; }
  addEventListener(type, fn) { (this.handlers[type] ||= []).push(fn); }
  emit(type) { return Promise.all((this.handlers[type] || []).map(fn => fn({preventDefault() {}}))); }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  focus() { this.focused = true; }
  showModal() { this.open = true; }
  close() { this.open = false; this.emit('close'); }
  set innerHTML(_) { throw Error('Untrusted content must not be rendered as HTML'); }
  style = {};
}
const fields = new Map();
const dialog = new Element();
dialog.querySelector = name => {
  if (!fields.has(name)) fields.set(name, new Element());
  return fields.get(name);
};
const context = {window: {}};
vm.runInNewContext(read('relaisdesk/client/subscription-cancellation.js'), context);
const review = context.window.RdSubscriptionCancellation.create(dialog);
const details = {language: 'fr', contract: 'trial-test', account: 'test@example.invalid', offer: '<img src=x onerror=alert(1)>', price: '110 €/mois', end: '12 octobre 2026'};
let pending = review.confirm(details);
assert(dialog.open);
assert.equal(fields.get('[data-cancel="offer"]').textContent, details.offer);
assert.equal(fields.get('[data-cancel="submit"]').textContent, 'Confirmer la résiliation du contrat');
assert(fields.get('[data-cancel="title"]').focused);
assert.equal(fields.get('form').scrollTop, 0, 'open at the recap even on a small screen');
assert.equal(await review.confirm(details), false, 'must not replace an active review');
await fields.get('[data-cancel="back"]').emit('click');
assert.equal(await pending, false);
pending = review.confirm({...details, language: 'en'});
assert.equal(fields.get('[data-cancel="submit"]').textContent, 'Confirm contract cancellation');
await dialog.emit('cancel');
assert.equal(await pending, false, 'Escape must not confirm');
pending = review.confirm(details);
await dialog.emit('close');
assert(dialog.open, 'stale close event must not dismiss the new review');
await fields.get('form').emit('submit');
assert.equal(await pending, true);
assert(!dialog.open);
pending = review.confirm(details);
review.dismiss();
assert.equal(await pending, false, 'logout must dismiss without confirming');
dialog.showModal = () => { throw Error('unsupported'); };
assert.equal(await review.confirm(details), false);

// Exercise the real subscription handler, with no network and no real account.
const app = read('relaisdesk/client/app.js');
const renderSource = app.slice(app.indexOf('function renderSubscriptions('), app.indexOf('\nfunction badge(', app.indexOf('function renderSubscriptions(')));
const nodes = new Map(['subscriptionsList', 'subscriptionsCard', 'appMessage'].map(name => [name, new Element()]));
let decide, apiCalls = [];
const state = {token: 'test-session', data: {email: 'test@example.invalid'}};
const appContext = {
  state, window: {}, byId: id => nodes.get(id),
  document: {createElement: () => new Element()},
  cancellationReview: {confirm(data) { assert.equal(data.account, state.data.email); return new Promise(resolve => {decide = resolve;}); }},
  api: async (url, request) => { apiCalls.push([url, JSON.parse(request.body)]); return {status: 202, json: async () => ({message: 'Recorded'})}; },
  loadDashboard: async () => {}, setMessage: () => {},
  formatDate: () => '12 octobre 2026', money: value => value + ' €'
};
vm.runInNewContext(renderSource, appContext);
const subscription = {id: 'trial-test', plan: 'pro', technicians: 5, trial_end: 2000000000, paid_through: 0, price_cents: 11000, billing_cycle: 'monthly'};
function renderButton() {
  appContext.renderSubscriptions([subscription]);
  return nodes.get('subscriptionsList').children[0].children.find(child => child.handlers.click);
}
let button = renderButton();
let click = button.emit('click');
assert.equal(apiCalls.length, 0, 'opening review must not cancel');
await button.emit('click');
decide(false); await click;
assert.equal(apiCalls.length, 0);
assert(!button.disabled);
click = button.emit('click'); decide(true); await click;
assert.deepEqual(apiCalls, [['/api/v1/customer/subscriptions/cancel', {id: 'trial-test'}]]);
apiCalls = [];
button = renderButton(); click = button.emit('click');
state.token = 'different-session'; decide(true); await click;
assert.equal(apiCalls.length, 0, 'changed session must not submit old confirmation');

const translations = {window: {}, document: {readyState: 'loading', addEventListener() {}}};
vm.runInNewContext(read('relaisdesk/i18n.js'), translations);
const noMac = {fr: 'pas encore disponible', en: 'not yet publicly available', es: 'no está disponible', it: 'non è ancora disponibile', ru: 'пока недоступна', pl: 'nie jest jeszcze dostępny', de: 'noch nicht öffentlich verfügbar'};
for (const [lang, phrase] of Object.entries(noMac)) {
  const t = translations.window.RdI18n.translations[lang];
  for (const key of ['feat5_desc', 'faq_a5']) {
    assert(t[key].includes(phrase), lang + ':' + key);
    for (const os of ['Windows','Linux','macOS']) assert(t[key].includes(os));
  }
  assert(t.fleet_feat4_desc.includes('Ed25519'));
  assert(!/Chiffrement Ed25519|Ed25519 encryption|Cifrado Ed25519|Crittografia Ed25519|Шифрование Ed25519|Szyfrowanie Ed25519|Ed25519-Verschlüsselung/.test(t.fleet_feat4_desc));
  assert(t.feat2_desc.includes('Oracle Cloud Infrastructure'));
}
const index = read('relaisdesk/index.html');
for (const match of index.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)) {
  const data = JSON.parse(match[1]);
  const serialized = JSON.stringify(data);
  assert(!/"operatingSystem":"[^"]*macOS/.test(serialized));
}
const client = read('relaisdesk/client/index.html');
assert(client.includes('aria-labelledby="cancelSubscriptionTitle"'));
assert(client.indexOf('src="subscription-cancellation.js?') < client.indexOf('src="app.js?'));
assert(!renderSource.includes('window.confirm'));
assert(app.includes('cancellationReview.dismiss()'));
const privacy = read('relaisdesk/politique-confidentialite.html');
for (const text of ['stripe.com/fr/legal/dta','oracle.com/contracts/cloud-services/','ovhcloud.com/fr/personal-data-protection/','demander gratuitement']) assert(privacy.includes(text));
const sources = read('relaisdesk/logiciel-libre.html');
for (const value of ['10:22:39 UTC','0479b842','86a2132b','924efac1','4890b540','non publié']) assert(sources.includes(value));
console.log('OK : confirmation/retour/Escape/déconnexion/double clic, requête API inchangée, 7 langues, sources et garanties RGPD.');
