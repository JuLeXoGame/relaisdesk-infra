// Isolated rendering: all external requests are intercepted. Never contacts
// production, sends mail, starts a checkout or downloads an executable.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const http = require('node:http');
const { chromium } = require('playwright');
const root = path.resolve(__dirname, '../relaisdesk');
const output = path.resolve(__dirname, '../output/tests/accueil-essai');
const before = process.argv.includes('--before');
const server = http.createServer(async (req, res) => {
  const requested = decodeURIComponent(new URL(req.url, 'http://localhost').pathname);
  let file = path.resolve(root, '.' + requested);
  if (file !== root && !file.startsWith(root + path.sep)) { res.writeHead(403).end(); return; }
  if (requested.endsWith('/')) file = path.join(file, 'index.html');
  try {
    const body = await fs.readFile(file);
    const types = { '.css': 'text/css', '.js': 'application/javascript', '.html': 'text/html; charset=utf-8', '.avif': 'image/avif', '.json': 'application/json' };
    res.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream');
    res.end(body);
  } catch { res.writeHead(404).end(); }
});

(async () => {
  await fs.mkdir(output, { recursive: true });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({ headless: true, channel: 'msedge' });
    const page = await browser.newPage({ locale: 'fr-FR' });
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    let enabled = true, unavailable = false, responded = false;
    await page.route('**/*', async route => {
      const req = route.request(), url = new URL(req.url());
      if (url.hostname === 'api.relaisdesk.fr' && url.pathname === '/api/v1/public/trials' && req.method() === 'GET') {
        await route.fulfill({ status: unavailable ? 503 : 200, headers: { 'Access-Control-Allow-Origin': origin },
          contentType: 'application/json', body: JSON.stringify({ enabled, days: 30 }) });
        responded = true;
      } else if (url.origin === origin) await route.continue();
      else await route.abort();
    });

    const results = [];
    const sizes = before ? [[1440, 16], [390, 16]] : [[1440, 16], [1024, 16], [768, 16], [390, 16], [320, 16], [960, 32], [390, 32]];
    for (const [width, fontSize] of sizes) {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(origin + '/', { waitUntil: 'networkidle' });
      await page.addStyleTag({ content: `html { scroll-behavior: auto; font-size: ${fontSize}px; }` });
      await page.locator('#trialOffer').waitFor({ state: 'visible' });
      await page.evaluate(async () => {
        await document.fonts.ready;
        window.scrollTo(0, document.querySelector('#pricing').offsetTop - document.querySelector('.header').offsetHeight - 12);
      });
      const geometry = await page.evaluate(() => {
        const offer = document.querySelector('#trialOffer');
        const cta = offer.querySelector('a');
        const description = offer.querySelector('p');
        const range = document.createRange();
        if (description) range.selectNodeContents(description);
        else range.selectNode(offer.lastChild);
        const detail = range.getBoundingClientRect();
        const banner = offer.getBoundingClientRect();
        const link = cta.getBoundingClientRect();
        const toggle = document.querySelector('.billing-toggle-container').getBoundingClientRect();
        const buttons = [...document.querySelectorAll('.billing-toggle-btn')].map(e => e.getBoundingClientRect());
        return { width: innerWidth, gap: toggle.top - detail.bottom, ctaGap: detail.top - link.bottom,
          bannerGap: toggle.top - banner.bottom, offerContained: banner.left >= 0 && banner.right <= innerWidth + 1,
          ctaContained: link.left >= banner.left && link.right <= banner.right + 1,
          descriptionContained: detail.left >= banner.left && detail.right <= banner.right + 1,
          controlsContained: buttons.every(r => r.left >= -1 && r.right <= innerWidth + 1) };
      });
      results.push({ width, fontSize, ...geometry });
      if ((width === 1440 || width === 390) && fontSize === 16) {
        await page.screenshot({ path: path.join(output, `${before ? 'avant' : 'apres'}-${width}.png`) });
      }
      if (!before) {
        assert.ok(geometry.gap >= 24, `Mention covered / gap too small: ${JSON.stringify(geometry)}`);
        assert.ok(geometry.ctaGap >= 8, 'Trial button and mention need separate space');
        assert.ok(geometry.bannerGap >= 24, 'Billing controls overlap the banner');
        for (const property of ['offerContained', 'ctaContained', 'descriptionContained', 'controlsContained']) assert.ok(geometry[property], `${property} at ${width}/${fontSize}`);
        await page.locator('#trialOffer a').click({ trial: true });
        assert.equal(await page.locator('#trialOffer a').getAttribute('href'), 'essai/');
        await page.locator('#cycleAnnualBtn').click();
        assert.match(await page.locator('#proPriceVal').innerText(), /1\s?100/);
        await page.locator('#cycleMonthlyBtn').click();
        assert.match(await page.locator('#proPriceVal').innerText(), /^110/);
      }
    }

    if (!before) {
      for (const state of ['disabled', 'unavailable']) {
        enabled = false; unavailable = state === 'unavailable'; responded = false;
        await page.goto(origin + '/', { waitUntil: 'networkidle' });
        assert.ok(responded, 'Trial availability stub not called');
        assert.equal(await page.locator('#trialOffer').isVisible(), false, state);
        assert.equal(await page.locator('#trialOffer').getAttribute('hidden'), '');
      }
      const noJS = await browser.newContext({ javaScriptEnabled: false });
      const staticPage = await noJS.newPage();
      await staticPage.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
      await staticPage.goto(origin + '/');
      assert.equal(await staticPage.locator('#trialOffer').isVisible(), false);
      await noJS.close();
      assert.deepEqual(errors, []);
    }
    await fs.writeFile(path.join(output, before ? 'avant.json' : 'apres.json'), JSON.stringify(results, null, 2));
    console.log(JSON.stringify(results));
    console.log(before ? 'Baseline captured without mutations.' : 'OK: no overlap desktop/mobile/large text; CTA clickable, billing controls work, disabled/offline/no-JS offer stays hidden.');
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
