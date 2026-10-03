'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.resolve(__dirname, '..');
const source = fs.readFileSync(path.join(root, 'relaisdesk/client/services.js'), 'utf8');
const html = fs.readFileSync(path.join(root, 'relaisdesk/client/index.html'), 'utf8');

function fixture(api) {
  const nodes = new Map();
  class Element {
    constructor(tag) { this.tag = tag; this.children = []; this.events = {}; this.textContent = ''; this.value = ''; }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    addEventListener(name, handler) { this.events[name] = handler; }
    querySelector() { return this.button ||= new Element('button'); }
    reset() {}
  }
  const byId = id => {
    assert.ok(html.includes(`id="${id}"`), `Missing HTML element ${id}`);
    if (!nodes.has(id)) nodes.set(id, new Element('div'));
    return nodes.get(id);
  };
  const state = { token: 'fixture', data: { customer_role: 'owner', licenses: [{}] } };
  const scope = { state, api, byId, document: { createElement: tag => new Element(tag) }, window: {}, money: n => `${n} EUR`, setMessage: (n,s) => { n.textContent=s; }, URL, confirm: () => true, copyTextToClipboard() {}, location: { assign() { throw new Error('Unexpected navigation'); } } };
  vm.runInNewContext(source, scope);
  return { scope, nodes, byId, state, services: scope.window.rdServices };
}
function catalog(extra = {}) { return { available: true, merchant: {enabled:false}, rates:[], work:[], ...extra }; }

test('Terms require an unchecked explicit acceptance and matching document', async () => {
  const calls=[];
  const digest = require('node:crypto').createHash('sha256').update(fs.readFileSync(path.join(root,'api/servicelegal/conditions-2026-09-19.txt'))).digest('hex');
  const data=catalog({active_license:true,terms:{version:'2026-09-24-prestations-v2',sha256:digest,accepted_at:0}});
  const f=fixture(async(url,options)=>{calls.push({url,options});return{json:async()=>data};});
  await f.services.load();
  assert.equal(f.byId('serviceTermsAccept').checked,false);
  assert.equal(f.byId('serviceConnect').disabled,true);
  const form=f.byId('serviceTermsForm');
  const event={preventDefault(){},currentTarget:form};
  await form.events.submit(event);
  assert.equal(calls.length,1);
  f.byId('serviceTermsAccept').checked=true;
  data.terms.sha256='wrong';
  await form.events.submit(event);
  assert.equal(calls.length,1);
  data.terms.sha256=digest;
  await form.events.submit(event);
  assert.equal(calls[1].url,'/api/v1/customer/service-billing/terms/accept');
  assert.equal(JSON.parse(calls[1].options.body).accepted,true);
});

test('An enabled merchant can always disable even when the module is stopped', async () => {
  const f=fixture(async()=>({json:async()=>catalog({available:false,active_license:false,merchant:{enabled:true}})}));
  await f.services.load();
  assert.equal(f.byId('serviceToggle').disabled,false);
  assert.equal(f.byId('serviceConnect').disabled,true);
});

test('Owner-only navigation and rendering without HTML interpolation', async () => {
  const f = fixture(async () => ({ json: async () => catalog({ rates: [{ id:'rate', label:'<img src=x onerror=alert(1)>', mode:'hourly', cents:6000, active:true }] }) }));
  f.services.navigation(); assert.equal(f.byId('servicesNav').hidden, false);
  f.state.data.customer_role='member'; f.services.navigation(); assert.equal(f.byId('servicesNav').hidden, true);
  await f.services.load();
  assert.equal(f.byId('serviceRates').children[0].children[0].textContent, '<img src=x onerror=alert(1)>');
  assert.equal(f.byId('serviceRates').children[0].children[0].tag, 'h3');
});

test('A response arriving after logout does not restore commercial data', async () => {
  let resolve;
  const f = fixture(() => new Promise(done => { resolve=done; }));
  const pending = f.services.load();
  f.services.clear(); f.state.token='';
  resolve({ json: async () => catalog({ rates:[{id:'private',label:'Private'}] }) });
  await pending;
  assert.equal(f.byId('serviceRates').children.length,0);
});

test('Decimal rate is submitted as exact integer cents', async () => {
  const calls=[];
  const f=fixture(async (url, options) => { calls.push({url,options}); return {json:async()=>catalog()}; });
  f.byId('serviceRateAmount').value='60,05'; f.byId('serviceRateMode').value='hourly'; f.byId('serviceRateLabel').value='Assistance';
  const form=f.byId('serviceRateForm');
  await form.events.submit({preventDefault(){},currentTarget:form});
  const data=JSON.parse(calls[0].options.body);
  assert.equal(data.cents,6005); assert.equal(data.mode,'hourly');
  assert.equal(calls[0].options.method,'POST');
});

test('A foreign payment link is not offered to copy', async () => {
  const f=fixture(async()=>({json:async()=>catalog({work:[{id:'work',target_id:'pc',label:'Service',connected_ms:0,amount_cents:6000,state:'finished',paid:true,checkout_url:'https://evil.test/payment'}]})}));
  await f.services.load();
  const buttons=f.byId('serviceWork').children[0].children.filter(n=>n.tag==='button');
  assert.equal(buttons.length,0);
});
