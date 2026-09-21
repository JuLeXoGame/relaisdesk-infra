import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = file => fs.readFileSync(path.join(root, file), 'utf8');
const elements = new Map();
function element(id) {
  if (!elements.has(id)) elements.set(id, {
    value: id === 'ultraTechSlider' ? '10' : '', textContent: '', style: {},
    classList: { add() {}, remove() {} }, reset() {}, addEventListener() {},
  });
  return elements.get(id);
}
let language = 'fr';
const context = vm.createContext({
  document: { addEventListener() {}, getElementById: element, querySelectorAll: () => [] },
  window: { RdI18n: { getLang: () => language } },
});
vm.runInContext(read('relaisdesk/app.js'), context);
const evaluate = code => vm.runInContext(code, context);
const adminContext = vm.createContext({
  document: { addEventListener() {}, getElementById: element }, navigator: {}, window: {},
});
vm.runInContext(read('relaisdesk/admin/app.js'), adminContext);
assert.equal(evaluate('PRO_MONTHLY_PRICE'), 110);
assert.equal(evaluate('PRO_ANNUAL_PRICE'), 1100);
assert.equal(evaluate('ULTRA_BASE_MONTHLY_PRICE'), 199);
assert.equal(evaluate('TERMS_VERSION'), '2026-09-21');
const displayAmount = value => Number(value.replace(/\s|€/gu, '').replace(',', '.'));
for (language of ['fr', 'en']) {
  for (const [cycle, multiplier] of [['monthly', 1], ['annual', 10]]) {
    evaluate(`currentBillingCycle = '${cycle}'; updatePricingDisplays()`);
    assert.equal(displayAmount(element('proPriceVal').textContent), 110 * multiplier);
    assert.equal(displayAmount(element('starterPriceVal').textContent), cycle === 'annual' ? 239 : 24.9);
    assert.equal(displayAmount(element('ultraPriceVal').textContent), 199 * multiplier);
    for (const [plan, expected] of [['starter', cycle === 'annual' ? 239 : 24.9], ['pro', 110 * multiplier], ['ultra', 199 * multiplier]]) {
      evaluate(`openOrderModal('${plan}')`);
      assert.equal(evaluate('currentOrder.price'), expected);
      assert.match(element('modalTotalPrice').innerHTML, /0,00 €/);
      const quota = plan === 'starter' ? 500 : plan === 'pro' ? 1000 : 2000;
      assert.equal(evaluate(`calculateManagedDeviceLimit('${plan}', 10)`), quota);
      assert(element('orderFleetQuota').textContent.includes(quota.toLocaleString('fr-FR')));
      evaluate('selectPaymentMethod("bank_transfer")');
      assert.equal(displayAmount(element('modalTotalPrice').textContent), expected);
    }
  }
}

// Compare equal or greater capacity, allowing both a mix and an unused Pro seat.
function cheapestBundle(capacity, cycle) {
  const starter = cycle === 'annual' ? 23900 : 2490;
  const pro = cycle === 'annual' ? 110000 : 11000;
  let best = { cents: capacity * starter, pro: 0, starter: capacity };
  for (let count = 1; count <= Math.ceil(capacity / 5); count++) {
    const singles = Math.max(0, capacity - count * 5);
    const cents = count * pro + singles * starter;
    if (cents < best.cents) best = { cents, pro: count, starter: singles };
  }
  return best;
}
const disadvantages = { monthly: [], annual: [] };
const rows = [];
for (let capacity = 10; capacity <= 500; capacity++) {
  // The approved base changes, not the marginal price at each tier.
  const expected = 199 + Math.min(capacity - 10, 40) * 14
    + Math.min(Math.max(capacity - 50, 0), 50) * 10 + Math.max(capacity - 100, 0) * 7;
  for (const cycle of ['monthly', 'annual']) {
    const price = evaluate(`calculateUltraPrice(${capacity}, '${cycle}')`);
    assert.equal(price, expected * (cycle === 'annual' ? 10 : 1));
    element('ultraTechSlider').value = String(capacity);
    evaluate(`currentBillingCycle = '${cycle}'; updatePricingDisplays(); openOrderModal('ultra')`);
    assert.equal(displayAmount(element('ultraPriceVal').textContent), price);
    const expectedDevices = capacity === 500 ? 4500 : 2000 + (capacity - 10) * 5;
    assert.equal(displayAmount(element('ultraDeviceCount').textContent), expectedDevices);
    assert(element('orderFleetQuota').textContent.includes(expectedDevices.toLocaleString('fr-FR')));
    assert.match(element('modalTotalPrice').innerHTML, /0,00 €/);
    evaluate('selectPaymentMethod("bank_transfer")');
    assert.equal(displayAmount(element('modalTotalPrice').textContent), price);
    const best = cheapestBundle(capacity, cycle);
    if (Math.round(price * 100) >= best.cents) disadvantages[cycle].push(capacity);
    if ([10, 11, 12, 20, 50, 100, 500].includes(capacity)) rows.push({
      capacity, cycle, personalised: price, bestBundle: best.cents / 100,
      saving: (best.cents - Math.round(price * 100)) / 100,
    });
  }
  element('createTechSlider').value = String(capacity);
  vm.runInContext('updateUltraPricing()', adminContext);
  assert(element('createUltraPriceDisplay').innerHTML.startsWith(`${expected} € `));
  assert(element('createUltraPriceDisplay').innerHTML.includes(`(${expected * 10} €/an)`));
}
assert.deepEqual(disadvantages, { monthly: [], annual: [] });

const html = read('relaisdesk/index.html');
assert.match(html, /id="proPriceVal"[^>]*>110,00</);
assert.match(html, /id="ultraPriceVal"[^>]*>199,00</);
assert.match(html, /id="modalTotalPrice"[^>]*>110,00 €/);
const structured = [...html.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)].map(match => JSON.parse(match[1]));
const offers = [];
function visit(value) {
  if (!value || typeof value !== 'object') return;
  if ('price' in value) offers.push(value);
  for (const child of Object.values(value)) visit(child);
}
structured.forEach(visit);
assert.equal(offers.length, 3);
assert.equal(Number(offers.find(offer => offer.name === 'Starter').price), 24.9);
assert.equal(Number(offers.find(offer => offer.name === 'Pro').price), 110);
assert.equal(Number(offers.find(offer => offer.name !== 'Pro' && offer.name !== 'Starter').price), 199);
assert(!/129(?:,00)? €|1290|1 290/.test(read('relaisdesk/admin/index.html')));
console.table(rows);
console.log('OK: prices, FR/EN display, checkout summary, SEO, and all 491 personalised capacities.');
console.log('All personalised capacities are cheaper than the best Starter/Pro combination, monthly and annual.');
