import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const read=file=>fs.readFileSync(path.join(root,file),'utf8');
const html=read('relaisdesk/essai/conditions.html');
const contents=html.match(/<pre>([\s\S]*?)<\/pre>/)[1].replace(/&#(\d+);/g,(_,n)=>String.fromCharCode(Number(n))).replaceAll('&lt;','<').replaceAll('&gt;','>').replaceAll('&amp;','&');
assert.equal(contents.trim(),read('api/mailer/legal/ESSAI-RelaisDesk-2026-09-21-fleet-v2.txt').trim(),'published and emailed trial conditions differ');
for(const file of ['relaisdesk/essai/index.html','relaisdesk/essai/conditions.html']){
  const text=read(file);
  for(const match of text.matchAll(/(?:src|href)="([^"#?]+)(?:[?#][^"]*)?"/g)){
    if(/^(https?:|mailto:)/.test(match[1]))continue;
    // Site-absolute references resolve against the web root (relaisdesk/).
    const target=match[1].startsWith('/')?path.join(root,'relaisdesk',match[1]):path.resolve(path.dirname(path.join(root,file)),match[1]);
    assert(fs.existsSync(target),`missing local resource: ${file} -> ${match[1]}`);
  }
}
assert.match(read('api/.env.example'),/TRIALS_ENABLED=false/);
assert.match(read('relaisdesk/essai/app.js'),/expected_price_cents/);
assert.match(read('api/handlers/trials.go'),/ExpectedPriceCents/);
assert.match(read('api/handlers/trials.go'),/cfg.B2CSalesReady\(\)/);
assert.match(read('api/config/config.go'),/B2C_MEDIATION_PENDING_ACKNOWLEDGED/);
assert.match(read('api/main.go'),/public\/trials\/withdraw/);
assert.match(read('relaisdesk/essai/index.html'),/name="billing_cycle"/);
assert(!/<input[^>]+type="checkbox"[^>]+checked/.test(read('relaisdesk/essai/index.html')));
// Exercise the real trial summary with both the API catalog and its fallback.
for (const publishedCatalog of [true, false]) {
  const elements = new Map();
  const defaults = { plan: 'starter', technicians: '10', billing_cycle: 'monthly', customer_type: 'business' };
  const element = id => {
    if (!elements.has(id)) elements.set(id, {
      value: defaults[id] || '', textContent: '', listeners: {},
      addEventListener(name, callback) { this.listeners[name] = callback; },
    });
    return elements.get(id);
  };
  const catalog = {
    starter: {price_monthly: 24.9, price_annual: 239},
    pro: {price_monthly: 110, price_annual: 1100},
    ultra: {base_monthly: 199},
  };
  if (publishedCatalog) {
    catalog.starter.managed_devices = 500;
    catalog.pro.managed_devices = 1000;
    Object.assign(catalog.ultra, {managed_devices: 2000, managed_devices_per_extra_technician: 5, max_managed_devices: 4500, managed_devices_max_tier_bonus: 50, base_techs: 10, max_techs: 500});
  }
  vm.runInNewContext(read('relaisdesk/essai/app.js'), {
    URLSearchParams,
    location: {hostname: 'localhost', search: '', hash: ''},
    window: {addEventListener() {}}, document: {getElementById: element},
    fetch: async url => ({ok: true, json: async () => url.endsWith('/pricing') ? catalog : {enabled: true}}),
  });
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(element('trialForm').hidden, false, 'trial did not initialize');
  for (const [plan, technicians, quota] of [['starter',1,500], ['pro',5,1000], ['ultra',10,2000], ['ultra',20,2050], ['ultra',50,2200], ['ultra',100,2450], ['ultra',200,2950], ['ultra',499,4445], ['ultra',500,4500]]) {
    element('plan').value = plan;
    element('technicians').value = String(technicians);
    element('plan').listeners.change();
    assert(element('summary').textContent.replace(/\s/g, '').includes(`${quota}postesenregistrés`), `${plan}/${technicians}: incorrect trial quota`);
  }
}
console.log('OK: fleet trial conditions identical in website/email, explicit price consent, B2C operational gate, withdrawal route and mediation warning.');
