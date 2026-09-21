// Isolated browser checks: all API/Stripe traffic is intercepted. No email,
// payment, real account or production API request is ever sent.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const http = require('node:http');
const { chromium } = require('playwright');
const root = path.resolve(__dirname, '../relaisdesk');
const output = path.resolve(__dirname, '../output/tests/parcours-essai');
const prices = { starter: {price_monthly:24.9,price_annual:239},pro:{price_monthly:110,price_annual:1100},ultra:{base_monthly:199,base_annual:1990} };
const server = http.createServer(async (req,res) => {
  const requested = decodeURIComponent(new URL(req.url,'http://localhost').pathname);
  let file = path.resolve(root,'.'+requested);
  if (!file.startsWith(root+path.sep)) { res.writeHead(403).end(); return; }
  if (requested.endsWith('/')) file = path.join(file,'index.html');
  try { const body = await fs.readFile(file); res.setHeader('Content-Type',file.endsWith('.css')?'text/css':file.endsWith('.js')?'application/javascript':'text/html; charset=utf-8');res.end(body); }
  catch {res.writeHead(404).end();}
});
(async()=>{
  await fs.mkdir(output,{recursive:true});
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const browser=await chromium.launch({headless:true,channel:'msedge'});
  try {
    const page=await browser.newPage({viewport:{width:1100,height:900},locale:'fr-FR'});
    let enabled=true, consumer=false, submitted, cancelled=false, withdrawal;
    const errors=[];page.on('pageerror',err=>errors.push(err.message));
    await page.route('**/*',async route=>{
      const request=route.request(),url=new URL(request.url());
      if (url.pathname.startsWith('/api/')) {
        const headers={'Access-Control-Allow-Origin':origin,'Access-Control-Allow-Headers':'content-type','Access-Control-Allow-Methods':'GET,POST,OPTIONS'};
        if(request.method()==='OPTIONS'){await route.fulfill({status:204,headers});return;}
        let body;
        if(url.pathname.endsWith('/trials'))body={enabled,days:30,consumer_enabled:consumer,mediation_pending:consumer};
        else if(url.pathname.endsWith('/pricing'))body=prices;
        else if(url.pathname.endsWith('/trials/request')){submitted=request.postDataJSON();body={message:'Confirmation e-mail simulée.'};}
        else if(url.pathname.endsWith('/trials/verify'))body={checkout_url:'https://checkout.stripe.com/c/pay/mock'};
        else if(url.pathname.endsWith('/trials/withdraw')){withdrawal=request.postDataJSON();body={request_id:'RET-MOCK',requested_at:new Date().toISOString()};}
        else if(url.pathname.endsWith('/customer/dashboard'))body={email:'test@example.com',customer_id:'CUS_TEST',licenses:[],orders:[],invoices:[],interventions:[],renewal_reminders_enabled:false,subscriptions:[{id:'trial-fixture',plan:'Pro',technicians:5,price_cents:110000,billing_cycle:'annual',trial_end:Math.floor(Date.now()/1000)+30*86400,paid_through:0,subscription_status:'trialing',cancel_at_period_end:cancelled}]};
        else if(url.pathname.endsWith('/customer/viewer-codes'))body={codes:[]};
        else if(url.pathname.endsWith('/customer/subscriptions/cancel')){assert.equal(request.postDataJSON().id,'trial-fixture');cancelled=true;body={message:'Annulation simulée confirmée.'};}
        else throw Error('Unexpected API request '+url.pathname);
        await route.fulfill({status:200,headers,contentType:'application/json',body:JSON.stringify(body)});return;
      }
      if(url.origin===origin){await route.continue();return;}
      if(url.hostname==='checkout.stripe.com'){await route.fulfill({status:200,body:'Stripe mock — no payment'});return;}
      await route.abort();
    });
    await page.goto(origin+'/essai/');await page.locator('#trialForm').waitFor({state:'visible'});
    await page.selectOption('#plan','ultra');await page.fill('#technicians','500');await page.locator('#technicians').dispatchEvent('change');await page.selectOption('#billing_cycle','annual');
    assert.match(await page.locator('#summary').innerText(),/40\s?590,00/);
    const grid=await page.evaluate(()=>{
      const amounts=[];
      for(let count=10;count<=500;count++){
        const input=document.getElementById('technicians');input.value=String(count);input.dispatchEvent(new Event('change'));
        const part=document.getElementById('summary').textContent.split('puis ')[1].split('/an')[0];
        amounts.push(Number(part.replace(/[^\d,]/g,'').replace(',','.')));
      }
      return amounts;
    });
    let monthly=199;
    for(let count=10;count<=500;count++){
      if(count>10)monthly+=count<=50?14:count<=100?10:7;
      assert.equal(grid[count-10],monthly*10,`trial quote for ${count} technicians`);
    }
    assert.equal(await page.locator('#consumerOption').isDisabled(),true);
    await page.check('#recurring');await page.selectOption('#plan','pro');assert.equal(await page.isChecked('#recurring'),false);
    assert.match(await page.locator('#summary').innerText(),/1\s?100,00/);
    await page.screenshot({path:path.join(output,'essai-desktop.png'),fullPage:true});
    await page.setViewportSize({width:390,height:844});await page.screenshot({path:path.join(output,'essai-mobile.png'),fullPage:true});
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth),false,'mobile horizontal overflow');
    for(const [id,value] of Object.entries({email:'test@example.com',name:'Entreprise Test',address:'1 rue Test',postal_code:'75001',city:'Paris'}))await page.fill('#'+id,value);
    await page.check('#terms');await page.check('#recurring');await page.click('#submitButton');
    await page.getByText('Confirmation e-mail simulée.',{exact:true}).waitFor();
    assert.equal(submitted.expected_price_cents,110000);assert.equal(submitted.technicians,5);assert.equal(submitted.recurring_accepted,true);
    await page.goto(origin+'/essai/#token=demo_verification_token');await page.locator('#verifyPanel').waitFor({state:'visible'});
    assert.equal(new URL(page.url()).hash,'','token must leave address bar');
    await page.click('#verifyButton');await page.waitForURL('https://checkout.stripe.com/**');
    enabled=false;await page.goto(origin+'/essai/');await page.getByText('Les essais ne sont pas encore ouverts.',{exact:true}).waitFor();assert.equal(await page.locator('#trialForm').isVisible(),false);
    await page.addInitScript(()=>sessionStorage.setItem('rd_customer_token','browser-fixture-only'));
    await page.goto(origin+'/client/');await page.locator('#subscriptionsCard').waitFor({state:'visible'});
    await page.screenshot({path:path.join(output,'annulation-mobile.png'),fullPage:true});
    page.once('dialog',async dialog=>{assert.match(dialog.message(),/Pro/);assert.match(dialog.message(),/annuler|Annuler/);await dialog.accept();});
    await page.getByRole('button',{name:'Annuler le renouvellement automatique',exact:true}).click();
    await page.getByText('Annulation simulée confirmée.',{exact:true}).waitFor();assert.equal(cancelled,true);
    assert.equal(await page.getByRole('button',{name:'Annuler le renouvellement automatique',exact:true}).isDisabled(),true);
    await page.evaluate(()=>window.RdI18n.setLanguage('en'));
    assert.equal(await page.getByRole('button',{name:'Cancel automatic renewal',exact:true}).isDisabled(),true);
    enabled=true;consumer=true;
    await page.goto(origin+'/essai/');await page.locator('#trialForm').waitFor({state:'visible'});
    await page.selectOption('#customer_type','consumer');
    assert.equal(await page.locator('#consumerOption').isDisabled(),false);
    assert.equal(await page.locator('#mediationNotice').isVisible(),true);
    assert.equal(await page.locator('#immediate').evaluate(n=>n.required),true);
    await page.goto(origin+'/formulaire-retractation.html#trial=trial-FiXtUrE');
    assert.equal(await page.inputValue('#withdrawalKind'),'trial');
    assert.equal(new URL(page.url()).hash,'');
    await page.fill('#withdrawalEmail','test@example.com');await page.fill('#withdrawalName','Client Test');
    await page.getByRole('button',{name:'Se rétracter du contrat',exact:true}).click();
    assert.equal(withdrawal,undefined,'first step must not submit');
    await page.getByRole('button',{name:'Confirmer la rétractation',exact:true}).click();
    await page.locator('#withdrawalResult').waitFor({state:'visible'});
    assert.equal(withdrawal.id,'trial-FiXtUrE');
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth),false,'withdrawal mobile overflow');
    await page.screenshot({path:path.join(output,'retractation-mobile.png'),fullPage:true});
    assert.deepEqual(errors,[]);
    console.log('OK: isolated browser — pricing, all plans, consent reset, B2C guard, request, email verification, Stripe redirect, disabled rollout, mobile layout.');
  } finally {await browser.close();server.close();}
})().catch(error=>{console.error(error);server.close();process.exitCode=1;});
