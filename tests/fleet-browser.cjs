// Offline browser review. Every request is fulfilled from local files or mock
// data; no fallback to the network, no production account, no native URI launch.
const { chromium } = require('playwright');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '../relaisdesk');
const outDir = path.resolve(__dirname, '../output/tests/fleet-fixed-2026-09-10');
fs.mkdirSync(outDir, {recursive:true});
const calls = [], blocked = [], errors = [];
const license = {license_id:'AUDIT-LICENSE',status:'active',max_connections:5,expires_at:'2027-01-01T00:00:00Z'};
let devices = [
  {device_id:'DEV-AUDT-0001',permanent_code:'PERM-AUDT-0001',rustdesk_id:'123456789',alias:'<img src=x onerror=alert(1)>',hostname:'MAC-AUDIT',os:'darwin',enrollment_state:'enrolled',status:'online',created_at:'2026-09-09T00:00:00Z',notes:''},
  {device_id:'DEV-AUDT-0002',permanent_code:'PERM-AUDT-0002',rustdesk_id:'',alias:'Poste en attente',hostname:'',os:'',enrollment_state:'pending',status:'offline',created_at:'2026-09-09T00:00:00Z',notes:''}
];
const dashboard = {customer_id:1,email:'audit@example.invalid',licenses:[license],invoices:[],orders:[],interventions:[],subscriptions:[],renewal_reminders_enabled:true};

(async () => {
  const browser = await chromium.launch({headless:true,channel:'msedge'});
  try {
    const context = await browser.newContext({viewport:{width:1440,height:1000},locale:'fr-FR',serviceWorkers:'block'});
    await context.route('**/*', async route => {
      const req = route.request(), u = new URL(req.url());
      if (u.hostname === 'localhost' && u.port === '8443' && u.pathname.startsWith('/api/v1/')) {
        calls.push({method:req.method(),path:u.pathname});
        if(req.method()==='OPTIONS') return route.fulfill({status:204,headers:{'Access-Control-Allow-Origin':'*','Access-Control-Allow-Headers':'*','Access-Control-Allow-Methods':'GET,POST,PUT,DELETE'}});
        let result, status = 200;
        if (u.pathname.endsWith('/dashboard')) result = dashboard;
        else if (u.pathname.endsWith('/viewer-codes')) result = {codes:[]};
        else if (u.pathname.endsWith('/devices/enrollment-code')) {
          const body = req.postDataJSON();
          const device = {...devices[1],device_id:'DEV-AUDT-0003',permanent_code:'PERM-AUDT-0003',alias:body.alias,notes:body.notes};
          devices.push(device); result={device}; status=201;
        } else if (u.pathname.endsWith('/devices')) result={devices};
        else if (u.pathname.includes('/devices/')) {
          const id=u.pathname.split('/').pop();
          if(req.method()==='DELETE') devices=devices.filter(d=>d.device_id!==id);
          else Object.assign(devices.find(d=>d.device_id===id),req.postDataJSON());
          result={success:true};
        } else { blocked.push(req.url()); return route.abort(); }
        return route.fulfill({status,contentType:'application/json',headers:{'Access-Control-Allow-Origin':'*'},body:JSON.stringify(result)});
      }
      if (u.origin === 'http://127.0.0.1:39753') {
        const relative = decodeURIComponent(u.pathname).replace(/^\//,'');
        let target = path.resolve(root,relative);
        if (!target.startsWith(root+path.sep)) return route.abort();
        if (target.endsWith(path.sep)) target += 'index.html';
        if (fs.existsSync(target) && fs.statSync(target).isDirectory()) target=path.join(target,'index.html');
        if (!fs.existsSync(target)) { blocked.push(u.pathname); return route.fulfill({status:404,body:'Not found'}); }
        const contentType = {'.html':'text/html','.css':'text/css','.js':'application/javascript','.svg':'image/svg+xml','.avif':'image/avif'}[path.extname(target)] || 'application/octet-stream';
        return route.fulfill({contentType,body:fs.readFileSync(target)});
      }
      blocked.push(req.url());
      return route.abort();
    });
    await context.addInitScript(() => sessionStorage.setItem('rd_customer_token','LOCAL-AUDIT-NOT-A-REAL-TOKEN'));
    const page=await context.newPage();
    page.on('pageerror', e=>errors.push(e.message));
    await page.goto('http://127.0.0.1:39753/client/#devices');
    await page.locator('#devicesBody tr').nth(1).waitFor();
    assert.equal(await page.locator('#devicesTotalCount').textContent(),'2');
    assert.equal(await page.locator('#devicesBody img').count(),0,'alias must remain text, not HTML');
    const macBadge=await page.locator('#devicesBody tr').first().locator('.os-badge').textContent();
    await page.locator('#deviceSearchInput').fill('MAC-AUDIT');
    assert.equal(await page.locator('#devicesBody tr').count(),1);
    await page.locator('#deviceSearchInput').fill('');
    await page.locator('#openEnrollDialogButton').click();
    await page.locator('#enrollAliasInput').fill('Poste audit');
    await page.locator('#enrollSubmitBtn').click();
    await page.waitForFunction(()=>document.getElementById('displayEnrollCode').textContent==='PERM-AUDT-0003');
    await page.locator('#closeEnrollDeviceDialog').click();
    await page.waitForFunction(()=>document.getElementById('devicesTotalCount').textContent==='3');
    page.once('dialog',d=>d.accept('Alias modifié'));
    await page.locator('#devicesBody tr').last().getByRole('button',{name:'Modifier',exact:false}).click();
    await page.waitForFunction(()=>document.getElementById('devicesBody').textContent.includes('Alias modifié'));
    page.once('dialog',d=>d.accept());
    await page.locator('#devicesBody tr').last().getByRole('button',{name:'Supprimer',exact:false}).click();
    await page.waitForFunction(()=>document.getElementById('devicesTotalCount').textContent==='2');
    await page.screenshot({path:path.join(outDir,'console-desktop.png'),fullPage:true});
    const languages={};
    for(const lang of ['fr','en','de','es','it','ru','pl']) {
      await page.evaluate(lang=>window.RdI18n.setLanguage(lang),lang);
      languages[lang]=await page.locator('#pageTitle').textContent();
    }
    await page.evaluate(()=>window.RdI18n.setLanguage('fr'));
    await page.setViewportSize({width:390,height:844});
    await page.screenshot({path:path.join(outDir,'console-mobile.png'),fullPage:true});
    const dimensions=await page.evaluate(()=>({viewport:innerWidth,pageWidth:document.documentElement.scrollWidth}));
    assert.match(macBadge,/macOS/);
    assert.ok(dimensions.pageWidth<=dimensions.viewport, `Mobile overflow: ${dimensions.pageWidth}`);
    const result={checks:['render','alias treated as text','filter','enrollment form (mock API)','rename','delete','seven languages'],macBadge,dimensions,languages,errors,blocked,apiCalls:calls.length,externalRequestsSent:0};
    console.log(JSON.stringify(result,null,2));
    fs.writeFileSync(path.join(outDir,'browser-results.json'),JSON.stringify(result,null,2));
    assert.equal(errors.length,0,'unexpected JavaScript runtime error');
  } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exitCode=1;});
